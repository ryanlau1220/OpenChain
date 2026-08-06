package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

type responseLogger struct {
	http.ResponseWriter
	statusCode int
}

func (rl *responseLogger) WriteHeader(code int) {
	rl.statusCode = code
	rl.ResponseWriter.WriteHeader(code)
}

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rl := &responseLogger{ResponseWriter: w, statusCode: http.StatusOK}
		next(rl, r)
		slog.Info("http_request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", rl.statusCode),
			slog.Duration("duration", time.Since(start)),
		)
	}
}

type Server struct {
	evm           *adapter.EVMClient
	labels        *labels.Registry
	riskEvaluator *risk.Evaluator
	tracingEngine *tracing.Engine
	caseService   *cases.Service
	wsHub         *Hub
	shareStoreMu  sync.RWMutex
	shareStore    map[string]CanvasShareItem
}

type CanvasShareItem struct {
	ShareID   string               `json:"share_id"`
	GraphData *tracing.GraphResult `json:"graph_data"`
	ExpiresAt time.Time            `json:"expires_at"`
}

func NewServer(evm *adapter.EVMClient, lr *labels.Registry, re *risk.Evaluator, te *tracing.Engine, cs *cases.Service, hub *Hub) *Server {
	return &Server{
		evm:           evm,
		labels:        lr,
		riskEvaluator: re,
		tracingEngine: te,
		caseService:   cs,
		wsHub:         hub,
		shareStore:    make(map[string]CanvasShareItem),
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", withLogging(s.handleHealth))
	mux.HandleFunc("/api/v1/lookup/address", withLogging(s.handleLookupAddress))
	mux.HandleFunc("/api/v1/tracing/graph", withLogging(s.handleTraceGraph))
	mux.HandleFunc("/api/v1/canvas/share", withLogging(s.handleCanvasShare))
	mux.HandleFunc("/api/v1/labels", withLogging(s.handleLabels))
	mux.HandleFunc("/api/v1/risk/evaluate", withLogging(s.handleEvaluateRisk))
	mux.HandleFunc("/api/v1/cases", withLogging(s.handleCases))
	mux.HandleFunc("/api/v1/cases/export", withLogging(s.handleExportCase))
	mux.HandleFunc("/ws", s.wsHub.HandleWS)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "openchain-api",
		"network": "ETHEREUM_SEPOLIA",
	})
}

func (s *Server) handleLookupAddress(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, `{"error":"address parameter is required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	bal, _ := s.evm.GetBalance(ctx, address)
	txCount, _ := s.evm.GetTxCount(ctx, address)
	isContract, _ := s.evm.IsContract(ctx, address)
	riskEval := s.riskEvaluator.EvaluateAddress(ctx, address, "ETHEREUM_SEPOLIA", isContract, txCount)
	lbls := s.labels.GetLabels(ctx, address)

	entityType := "EOA"
	if isContract {
		entityType = "CONTRACT"
	}

	labelStr := shortAddress(address)
	if len(lbls) > 0 {
		labelStr = lbls[0].Label
	}

	resp := map[string]interface{}{
		"summary": map[string]interface{}{
			"address":           address,
			"network":           "ETHEREUM_SEPOLIA",
			"entity_type":       entityType,
			"label":             labelStr,
			"balance_wei":       bal.String(),
			"balance_formatted": adapter.FormatWeiToETH(bal),
			"tx_count":          txCount,
			"risk_score":        riskEval.TotalScore,
			"risk_level":        riskEval.RiskLevel,
		},
		"labels": lbls,
		"risk":   riskEval,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

type TraceGraphRequest struct {
	Addresses []string `json:"addresses"`
	Tokens    []string `json:"tokens"`
	Direction string   `json:"direction"`
	MaxHops   uint32   `json:"max_hops"`
}

func (s *Server) handleTraceGraph(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method == "POST" {
		var req TraceGraphRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if len(req.Addresses) == 0 {
			http.Error(w, `{"error":"at least one address is required"}`, http.StatusBadRequest)
			return
		}

		maxHops := req.MaxHops
		if maxHops == 0 {
			maxHops = 2
		}

		dir := req.Direction
		if dir == "" {
			dir = "BOTH"
		}

		res, err := s.tracingEngine.TraceMultiAddressGraph(r.Context(), req.Addresses, "ETHEREUM_SEPOLIA", maxHops, dir, req.Tokens)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(res)
		return
	}

	address := r.URL.Query().Get("seed_address")
	if address == "" {
		http.Error(w, `{"error":"seed_address parameter is required"}`, http.StatusBadRequest)
		return
	}

	maxHopsStr := r.URL.Query().Get("max_hops")
	maxHops := 2
	if h, err := strconv.Atoi(maxHopsStr); err == nil && h > 0 {
		maxHops = h
	}

	dir := r.URL.Query().Get("direction")
	if dir == "" {
		dir = "BOTH"
	}

	res, err := s.tracingEngine.TraceMultiAddressGraph(r.Context(), []string{address}, "ETHEREUM_SEPOLIA", uint32(maxHops), dir, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
}

func (s *Server) handleCanvasShare(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method == "POST" {
		var req tracing.GraphResult
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		shareID := fmt.Sprintf("share-%s", uuid.New().String()[:8])
		expiresAt := time.Now().Add(7 * 24 * time.Hour) // 7 day expiration

		item := CanvasShareItem{
			ShareID:   shareID,
			GraphData: &req,
			ExpiresAt: expiresAt,
		}

		s.shareStoreMu.Lock()
		s.shareStore[shareID] = item
		s.shareStoreMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(item)
		return
	}

	shareID := r.URL.Query().Get("share_id")
	if shareID == "" {
		http.Error(w, `{"error":"share_id is required"}`, http.StatusBadRequest)
		return
	}

	s.shareStoreMu.RLock()
	item, ok := s.shareStore[shareID]
	s.shareStoreMu.RUnlock()

	if !ok || time.Now().After(item.ExpiresAt) {
		http.Error(w, `{"error":"share link invalid or expired"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(item)
}

func (s *Server) handleLabels(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method == "POST" {
		var req labels.LabelItem
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		item, err := s.labels.AddLabel(r.Context(), req)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(item)
		return
	}

	address := r.URL.Query().Get("address")
	lbls := s.labels.GetLabels(r.Context(), address)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(lbls)
}

func (s *Server) handleEvaluateRisk(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	address := r.URL.Query().Get("address")
	if address == "" {
		http.Error(w, `{"error":"address parameter is required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	isContract, _ := s.evm.IsContract(ctx, address)
	txCount, _ := s.evm.GetTxCount(ctx, address)
	eval := s.riskEvaluator.EvaluateAddress(ctx, address, "ETHEREUM_SEPOLIA", isContract, txCount)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(eval)
}

func (s *Server) handleCases(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	if r.Method == "POST" {
		var req struct {
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Tags        []string `json:"tags"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c, err := s.caseService.CreateCase(r.Context(), req.Title, req.Description, req.Tags)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c)
		return
	}

	id := r.URL.Query().Get("id")
	if id != "" {
		c, err := s.caseService.GetCase(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c)
		return
	}

	casesList := s.caseService.ListCases(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(casesList)
}

func (s *Server) handleExportCase(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
		return
	}

	caseID := r.URL.Query().Get("case_id")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "JSON"
	}

	filename, content, contentType, err := s.caseService.ExportReport(r.Context(), caseID, format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	w.Header().Set("Content-Type", contentType)
	_, _ = w.Write(content)
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func shortAddress(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
