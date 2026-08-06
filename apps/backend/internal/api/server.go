package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

type Server struct {
	evm           *adapter.EVMClient
	labels        *labels.Registry
	riskEvaluator *risk.Evaluator
	tracingEngine *tracing.Engine
	caseService   *cases.Service
	wsHub         *Hub
}

func NewServer(evm *adapter.EVMClient, lr *labels.Registry, re *risk.Evaluator, te *tracing.Engine, cs *cases.Service, hub *Hub) *Server {
	return &Server{
		evm:           evm,
		labels:        lr,
		riskEvaluator: re,
		tracingEngine: te,
		caseService:   cs,
		wsHub:         hub,
	}
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/health", s.handleHealth)
	mux.HandleFunc("/api/v1/lookup/address", s.handleLookupAddress)
	mux.HandleFunc("/api/v1/tracing/graph", s.handleTraceGraph)
	mux.HandleFunc("/api/v1/labels", s.handleLabels)
	mux.HandleFunc("/api/v1/risk/evaluate", s.handleEvaluateRisk)
	mux.HandleFunc("/api/v1/cases", s.handleCases)
	mux.HandleFunc("/api/v1/cases/export", s.handleExportCase)
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

func (s *Server) handleTraceGraph(w http.ResponseWriter, r *http.Request) {
	enableCORS(w)
	if r.Method == "OPTIONS" {
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

	res, err := s.tracingEngine.TraceMultiHopGraph(r.Context(), address, "ETHEREUM_SEPOLIA", uint32(maxHops), "BOTH")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(res)
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
		http.Error(w, `{"error":"address is required"}`, http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	txCount, _ := s.evm.GetTxCount(ctx, address)
	isContract, _ := s.evm.IsContract(ctx, address)
	riskEval := s.riskEvaluator.EvaluateAddress(ctx, address, "ETHEREUM_SEPOLIA", isContract, txCount)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(riskEval)
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
		cItem, err := s.caseService.CreateCase(r.Context(), req.Title, req.Description, req.Tags)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(cItem)
		return
	}

	cs := s.caseService.ListCases(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cs)
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

	filename, content, mimeType, err := s.caseService.ExportReport(r.Context(), caseID, format)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(content)
}

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
}

func shortAddress(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
