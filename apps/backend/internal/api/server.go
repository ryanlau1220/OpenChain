package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

type responseLogger struct {
	http.ResponseWriter
	statusCode int
}

func (writer *responseLogger) WriteHeader(statusCode int) {
	writer.statusCode = statusCode
	writer.ResponseWriter.WriteHeader(statusCode)
}

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &responseLogger{ResponseWriter: w, statusCode: http.StatusOK}
		next(writer, r)
		slog.Info("http_request", "method", r.Method, "path", r.URL.Path, "status", writer.statusCode, "duration", time.Since(started))
	}
}

type Server struct {
	evm            *adapter.EVMClient
	labels         *labels.Service
	tracingEngine  *tracing.Engine
	tracingQueue   *tracing.Queue
	webOrigin      string
	requestLimiter *RequestLimiter
	trustProxy     bool
}

func NewServer(evm *adapter.EVMClient, registry *labels.Service, engine *tracing.Engine, queue *tracing.Queue, webOrigin string, publicRequestsPerMinute int, trustProxy bool) *Server {
	return &Server{evm: evm, labels: registry, tracingEngine: engine, tracingQueue: queue, webOrigin: strings.TrimRight(webOrigin, "/"), requestLimiter: NewRequestLimiter(publicRequestsPerMinute), trustProxy: trustProxy}
}

func (s *Server) traceGraph(ctx context.Context, address string, direction tracing.Direction, limit uint32, cursor string, retry bool) (*tracing.GraphResult, error) {
	if s.tracingQueue != nil {
		return s.tracingQueue.TraceGraph(ctx, address, direction, limit, cursor, retry)
	}
	return s.tracingEngine.ResolveGraph(ctx, address, direction, limit, cursor)
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	s.RegisterConnectRPC(mux)
	mux.HandleFunc("/api/v1/health", withLogging(s.handleHealth))
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.RegisterRoutes(mux)
	return s.withCORS(withClientKey(mux, s.trustProxy))
}

func (s *Server) withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && origin == s.webOrigin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Connect-Protocol-Version")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	queue, err := s.tracingQueue.Stats(r.Context())
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		slog.Error("health queue stats", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	status := "healthy"
	if err != nil {
		status = "unhealthy"
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status  string               `json:"status"`
		Service string               `json:"service"`
		Network string               `json:"network"`
		Source  adapter.SourceStatus `json:"source"`
		Queue   tracing.Stats        `json:"queue"`
	}{Status: status, Service: "openchain-api", Network: "ETHEREUM_MAINNET", Source: s.tracingEngine.SourceStatus(r.Context()), Queue: queue})
}

func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
