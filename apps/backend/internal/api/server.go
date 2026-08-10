package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
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
	networks       map[pb.Network]NetworkRuntime
	labels         *labels.Service
	webOrigin      string
	requestLimiter *RequestLimiter
	trustProxy     bool
}

var errUnsupportedNetwork = errors.New("unsupported network")

type NetworkRuntime struct {
	EVM    *adapter.EVMClient
	Engine *tracing.Engine
	Queue  *tracing.Queue
}

func NewServer(networks map[pb.Network]NetworkRuntime, registry *labels.Service, webOrigin string, publicRequestsPerMinute int, trustProxy bool) *Server {
	return &Server{networks: networks, labels: registry, webOrigin: strings.TrimRight(webOrigin, "/"), requestLimiter: NewRequestLimiter(publicRequestsPerMinute), trustProxy: trustProxy}
}

func (s *Server) network(network pb.Network) (NetworkRuntime, error) {
	runtime, ok := s.networks[network]
	if !ok || runtime.Engine == nil {
		return NetworkRuntime{}, errUnsupportedNetwork
	}
	return runtime, nil
}

func (s *Server) traceGraph(ctx context.Context, network pb.Network, address string, direction tracing.Direction, limit uint32, cursor string, retry bool) (*tracing.GraphResult, error) {
	runtime, err := s.network(network)
	if err != nil {
		return nil, err
	}
	if runtime.Queue != nil {
		return runtime.Queue.TraceGraph(ctx, address, direction, limit, cursor, retry)
	}
	return runtime.Engine.ResolveGraph(ctx, address, direction, limit, cursor)
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
	queue := tracing.Stats{}
	networks := make([]string, 0, len(s.networks))
	var err error
	for network, runtime := range s.networks {
		networks = append(networks, network.String())
		stats, statsErr := runtime.Queue.Stats(r.Context())
		if statsErr != nil && err == nil {
			err = statsErr
		}
		queue.Enabled = queue.Enabled || stats.Enabled
		queue.Queued += stats.Queued
		queue.Running += stats.Running
		queue.Failed += stats.Failed
	}
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
		Status   string        `json:"status"`
		Service  string        `json:"service"`
		Networks []string      `json:"networks"`
		Queue    tracing.Stats `json:"queue"`
	}{Status: status, Service: "openchain-api", Networks: networks, Queue: queue})
}

func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
