package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

// ── Logging middleware ─────────────────────────────────────────────────────────

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

func enableCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS, PUT, DELETE")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Connect-Protocol-Version")
}

// ── Server ─────────────────────────────────────────────────────────────────────

type Server struct {
	evm           *adapter.EVMClient
	labels        *labels.Registry
	riskEvaluator *risk.Evaluator
	tracingEngine *tracing.Engine
	caseService   *cases.Service
	wsHub         *Hub
	pubSub        PubSubEngine
}

func NewServer(evm *adapter.EVMClient, lr *labels.Registry, re *risk.Evaluator, te *tracing.Engine, cs *cases.Service, hub *Hub) *Server {
	return &Server{
		evm:           evm,
		labels:        lr,
		riskEvaluator: re,
		tracingEngine: te,
		caseService:   cs,
		wsHub:         hub,
		pubSub:        NewMemoryPubSub(),
	}
}


func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// All business logic is served via ConnectRPC
	s.RegisterConnectRPC(mux)

	// Operational endpoint — not a business feature, no proto needed
	mux.HandleFunc("/api/v1/health", withLogging(s.handleHealth))

	// WebSocket hub for real-time graph updates
	mux.HandleFunc("/ws", s.wsHub.HandleWS)
}

// Handler returns the full HTTP handler with CORS applied to every route.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return withCORS(mux)
}

// withCORS sets CORS headers on every response and answers OPTIONS preflights.
// ConnectRPC POSTs send custom headers, so browsers preflight each call.
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ── Health ─────────────────────────────────────────────────────────────────────

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status":  "healthy",
		"service": "openchain-api",
		"network": "ETHEREUM_SEPOLIA",
	})
}

// shortAddress truncates an Ethereum address to a display-friendly form.
func shortAddress(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
