package adapter

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ProviderError is safe to log: its text never includes an endpoint or API key.
// The wrapped error remains available for retry classification.
type ProviderError struct {
	Provider   string
	StatusCode int
	RetryAfter time.Duration
	err        error
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("%s returned HTTP %d", e.Provider, e.StatusCode)
	}
	return e.Provider + " request failed"
}

func (e *ProviderError) Unwrap() error { return e.err }

func NewProviderHTTPError(provider string, response *http.Response) error {
	return &ProviderError{Provider: provider, StatusCode: response.StatusCode, RetryAfter: retryAfter(response.Header.Get("Retry-After"))}
}

func NewProviderTransportError(provider string, err error) error {
	return &ProviderError{Provider: provider, err: err}
}

func NewProviderRateLimitError(provider string) error {
	return &ProviderError{Provider: provider, StatusCode: http.StatusTooManyRequests}
}

func retryAfter(value string) time.Duration {
	if seconds, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && seconds > 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(value); err == nil {
		return max(time.Until(when), 0)
	}
	return 0
}

// RetryDelay returns a bounded, provider-aware delay for a transient failure.
func RetryDelay(err error, attempt int) (time.Duration, bool) {
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) {
		return 0, false
	}
	if providerErr.StatusCode != 0 && providerErr.StatusCode != http.StatusRequestTimeout && providerErr.StatusCode != http.StatusTooManyRequests && providerErr.StatusCode < 500 {
		return 0, false
	}
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Second << min(attempt-1, 2)
	if providerErr.RetryAfter > delay {
		delay = providerErr.RetryAfter
	}
	return min(delay, 10*time.Second), true
}

// ProviderHealth is a read-only snapshot suitable for the public health endpoint.
type ProviderHealth struct {
	Provider          string     `json:"provider"`
	MaxConcurrent     int        `json:"max_concurrent"`
	RequestsPerSecond int        `json:"requests_per_second"`
	Requests          uint64     `json:"requests"`
	Failures          uint64     `json:"failures"`
	Throttled         uint64     `json:"throttled"`
	LastSuccessAt     *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt     *time.Time `json:"last_failure_at,omitempty"`
}

type providerMetrics struct {
	mu     sync.Mutex
	health ProviderHealth
}

func newProviderMetrics(provider string, requestsPerSecond int) *providerMetrics {
	return &providerMetrics{health: ProviderHealth{Provider: provider, MaxConcurrent: 1, RequestsPerSecond: requestsPerSecond}}
}

func (m *providerMetrics) request(delay time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.health.Requests++
	if delay > 0 {
		m.health.Throttled++
	}
	m.mu.Unlock()
}

func (m *providerMetrics) success() {
	if m == nil {
		return
	}
	m.mu.Lock()
	now := time.Now().UTC()
	m.health.LastSuccessAt = &now
	m.mu.Unlock()
}

func (m *providerMetrics) failure() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.health.Failures++
	now := time.Now().UTC()
	m.health.LastFailureAt = &now
	m.mu.Unlock()
}

func (m *providerMetrics) snapshot() ProviderHealth {
	if m == nil {
		return ProviderHealth{}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.health
}
