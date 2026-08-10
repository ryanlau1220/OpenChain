package api

import (
	"context"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const maxTrackedClients = 10_000

type clientKeyContext struct{}

type requestWindow struct {
	started  time.Time
	requests int
}

// RequestLimiter applies a fixed per-client budget to public RPC calls.
type RequestLimiter struct {
	mu      sync.Mutex
	limit   int
	windows map[string]requestWindow
	now     func() time.Time
}

func NewRequestLimiter(limit int) *RequestLimiter {
	return &RequestLimiter{limit: limit, windows: make(map[string]requestWindow), now: time.Now}
}

func (l *RequestLimiter) Allow(client string) bool {
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	window, exists := l.windows[client]
	if !exists && len(l.windows) >= maxTrackedClients {
		for key, candidate := range l.windows {
			if now.Sub(candidate.started) >= time.Minute {
				delete(l.windows, key)
			}
		}
		if len(l.windows) >= maxTrackedClients {
			return false
		}
	}
	// ponytail: fixed windows can burst at their boundary; use a token bucket only if this budget proves too coarse.
	if !exists || now.Sub(window.started) >= time.Minute {
		l.windows[client] = requestWindow{started: now, requests: 1}
		return true
	}
	if window.requests >= l.limit {
		return false
	}
	window.requests++
	l.windows[client] = window
	return true
}

func withClientKey(next http.Handler, trustProxy bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		client := r.RemoteAddr
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			client = host
		}
		if trustProxy {
			if forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); net.ParseIP(forwarded) != nil {
				client = forwarded
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), clientKeyContext{}, client)))
	})
}

func clientKey(ctx context.Context) string {
	if key, ok := ctx.Value(clientKeyContext{}).(string); ok && key != "" {
		return key
	}
	return "unknown"
}
