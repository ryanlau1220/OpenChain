package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
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
	Chain  adapter.ChainAdapter
	Engine *tracing.Engine
	Queue  *tracing.Queue
}

type healthNetwork struct {
	Network   string                   `json:"network"`
	Queue     tracing.Stats            `json:"queue"`
	Providers []adapter.ProviderHealth `json:"providers"`
}

// HealthAlert is a stable, machine-readable operational signal. A degraded
// service remains reachable so monitoring can collect the cause and operators
// can drain or retry durable jobs.
type HealthAlert struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Network  string `json:"network,omitempty"`
	Provider string `json:"provider,omitempty"`
	Message  string `json:"message"`
}

func NewServer(networks map[pb.Network]NetworkRuntime, registry *labels.Service, webOrigin string, publicRequestsPerMinute int, trustProxy bool) *Server {
	return &Server{networks: networks, labels: registry, webOrigin: strings.TrimRight(webOrigin, "/"), requestLimiter: NewRequestLimiter(publicRequestsPerMinute), trustProxy: trustProxy}
}

func (s *Server) network(network pb.Network) (NetworkRuntime, error) {
	runtime, ok := s.networks[network]
	if !ok || runtime.Chain == nil || runtime.Engine == nil {
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

func (s *Server) traceStatus(ctx context.Context, network pb.Network, address string, direction tracing.Direction, limit uint32, cursor string) (*tracing.GraphResult, error) {
	runtime, err := s.network(network)
	if err != nil {
		return nil, err
	}
	if runtime.Queue != nil {
		return runtime.Queue.TraceStatus(ctx, address, direction, limit, cursor)
	}
	return nil, tracing.ErrTraceNotFound
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
	type providerHealthReporter interface {
		ProviderHealth() []adapter.ProviderHealth
	}
	networks := make([]healthNetwork, 0, len(s.networks))
	var err error
	for _, runtime := range s.networks {
		stats := tracing.Stats{}
		if runtime.Queue != nil {
			var statsErr error
			stats, statsErr = runtime.Queue.Stats(r.Context())
			if statsErr != nil && err == nil {
				err = statsErr
			}
		}
		queue.Enabled = queue.Enabled || stats.Enabled
		queue.Queued += stats.Queued
		queue.Running += stats.Running
		queue.Failed += stats.Failed
		item := healthNetwork{Network: runtime.Engine.Network(), Queue: stats}
		if reporter, ok := runtime.Chain.(providerHealthReporter); ok {
			item.Providers = reporter.ProviderHealth()
		}
		networks = append(networks, item)
	}
	sort.Slice(networks, func(left, right int) bool { return networks[left].Network < networks[right].Network })
	alerts := healthAlerts(networks, s.networks)
	w.Header().Set("Content-Type", "application/json")
	if err != nil {
		slog.Error("health queue stats", "error", err)
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	status := "healthy"
	if err != nil {
		status = "unhealthy"
	} else if len(alerts) > 0 {
		status = "degraded"
	}
	_ = json.NewEncoder(w).Encode(struct {
		Status   string          `json:"status"`
		Service  string          `json:"service"`
		Networks []healthNetwork `json:"networks"`
		Queue    tracing.Stats   `json:"queue"`
		Alerts   []HealthAlert   `json:"alerts"`
	}{Status: status, Service: "openchain-api", Networks: networks, Queue: queue, Alerts: alerts})
}

func healthAlerts(networks []healthNetwork, runtimes map[pb.Network]NetworkRuntime) []HealthAlert {
	alerts := make([]HealthAlert, 0)
	for _, network := range networks {
		for _, runtime := range runtimes {
			if runtime.Engine == nil || runtime.Engine.Network() != network.Network {
				continue
			}
			capacity := runtime.Queue.Capacity()
			if capacity > 0 && network.Queue.Queued >= int64(capacity) {
				alerts = append(alerts, HealthAlert{Code: "trace_queue_full", Severity: "critical", Network: network.Network, Message: "Trace queue is at capacity; new investigations are rejected."})
			} else if capacity > 0 && network.Queue.Queued*100 >= int64(capacity*80) {
				alerts = append(alerts, HealthAlert{Code: "trace_queue_near_capacity", Severity: "warning", Network: network.Network, Message: "Trace queue is at least 80% full."})
			}
			break
		}
		if network.Queue.Failed > 0 {
			alerts = append(alerts, HealthAlert{Code: "trace_jobs_failed", Severity: "warning", Network: network.Network, Message: "One or more trace jobs require a user retry."})
		}
		for _, provider := range network.Providers {
			if provider.LastFailureAt != nil && (provider.LastSuccessAt == nil || provider.LastFailureAt.After(*provider.LastSuccessAt)) {
				alerts = append(alerts, HealthAlert{Code: "provider_unhealthy", Severity: "warning", Network: network.Network, Provider: provider.Provider, Message: "The most recent provider request failed."})
			}
		}
	}
	return alerts
}

func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
