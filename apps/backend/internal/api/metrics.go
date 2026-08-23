package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type metrics struct {
	registry            *prometheus.Registry
	requests            *prometheus.CounterVec
	requestDuration     *prometheus.HistogramVec
	queueJobs           *prometheus.GaugeVec
	oldestQueuedSeconds *prometheus.GaugeVec
	providerRequests    *prometheus.GaugeVec
	providerFailures    *prometheus.GaugeVec
	providerThrottled   *prometheus.GaugeVec
	providerLastSuccess *prometheus.GaugeVec
	providerLastFailure *prometheus.GaugeVec
	collectionSuccess   prometheus.Gauge
}

func newMetrics() *metrics {
	m := &metrics{registry: prometheus.NewRegistry()}
	m.requests = prometheus.NewCounterVec(prometheus.CounterOpts{Name: "openchain_http_requests_total", Help: "HTTP requests handled by OpenChain."}, []string{"method", "route", "status"})
	m.requestDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "openchain_http_request_duration_seconds", Help: "HTTP request latency in seconds.", Buckets: prometheus.DefBuckets}, []string{"method", "route", "status"})
	m.queueJobs = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_trace_queue_jobs", Help: "Durable trace jobs by status."}, []string{"network", "status"})
	m.oldestQueuedSeconds = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_trace_queue_oldest_queued_seconds", Help: "Age of the oldest queued trace job."}, []string{"network"})
	m.providerRequests = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_provider_requests", Help: "Provider requests since process start."}, []string{"network", "provider"})
	m.providerFailures = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_provider_failures", Help: "Provider failures since process start."}, []string{"network", "provider"})
	m.providerThrottled = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_provider_throttled", Help: "Provider throttles since process start."}, []string{"network", "provider"})
	m.providerLastSuccess = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_provider_last_success_timestamp_seconds", Help: "Unix timestamp of the last successful provider request."}, []string{"network", "provider"})
	m.providerLastFailure = prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "openchain_provider_last_failure_timestamp_seconds", Help: "Unix timestamp of the last failed provider request."}, []string{"network", "provider"})
	m.collectionSuccess = prometheus.NewGauge(prometheus.GaugeOpts{Name: "openchain_metrics_collection_success", Help: "Whether durable operational metrics were collected successfully."})
	m.registry.MustRegister(m.requests, m.requestDuration, m.queueJobs, m.oldestQueuedSeconds, m.providerRequests, m.providerFailures, m.providerThrottled, m.providerLastSuccess, m.providerLastFailure, m.collectionSuccess)
	return m
}

func (m *metrics) observeRequest(method, route string, status int, duration time.Duration) {
	labels := prometheus.Labels{"method": method, "route": route, "status": strconv.Itoa(status)}
	m.requests.With(labels).Inc()
	m.requestDuration.With(labels).Observe(duration.Seconds())
}

func (m *metrics) handler(server *Server) http.Handler {
	handler := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.collect(server, r)
		handler.ServeHTTP(w, r)
	})
}

func (m *metrics) collect(server *Server, request *http.Request) {
	success := true
	for _, runtime := range server.networks {
		if runtime.Engine == nil {
			continue
		}
		network := runtime.Engine.Network()
		stats, err := runtime.Queue.Stats(request.Context())
		if err != nil {
			success = false
		} else {
			m.queueJobs.WithLabelValues(network, "queued").Set(float64(stats.Queued))
			m.queueJobs.WithLabelValues(network, "running").Set(float64(stats.Running))
			m.queueJobs.WithLabelValues(network, "failed").Set(float64(stats.Failed))
			m.oldestQueuedSeconds.WithLabelValues(network).Set(stats.OldestQueuedSeconds)
		}
		if reporter, ok := runtime.Chain.(interface {
			ProviderHealth() []adapter.ProviderHealth
		}); ok {
			for _, provider := range reporter.ProviderHealth() {
				m.collectProvider(network, provider)
			}
		}
	}
	if success {
		m.collectionSuccess.Set(1)
	} else {
		m.collectionSuccess.Set(0)
	}
}

func (m *metrics) collectProvider(network string, provider adapter.ProviderHealth) {
	labels := []string{network, provider.Provider}
	m.providerRequests.WithLabelValues(labels...).Set(float64(provider.Requests))
	m.providerFailures.WithLabelValues(labels...).Set(float64(provider.Failures))
	m.providerThrottled.WithLabelValues(labels...).Set(float64(provider.Throttled))
	m.providerLastSuccess.WithLabelValues(labels...).Set(timestamp(provider.LastSuccessAt))
	m.providerLastFailure.WithLabelValues(labels...).Set(timestamp(provider.LastFailureAt))
}

func timestamp(value *time.Time) float64 {
	if value == nil {
		return 0
	}
	return float64(value.Unix())
}

func metricRoute(path string) string {
	switch path {
	case "/api/v1/health", "/metrics":
		return path
	default:
		if strings.HasPrefix(path, "/openchain.v1.") {
			return "rpc"
		}
		return "other"
	}
}
