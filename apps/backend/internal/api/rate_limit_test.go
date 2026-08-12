package api

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestRequestLimiterExpiresClientWindow(t *testing.T) {
	limiter := NewRequestLimiter(1)
	now := time.Now()
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("198.51.100.1") || limiter.Allow("198.51.100.1") {
		t.Fatal("request budget was not enforced")
	}
	now = now.Add(time.Minute)
	if !limiter.Allow("198.51.100.1") {
		t.Fatal("request budget did not reset")
	}
}

func TestRequestLimiterKeepsConcurrentClientBudgetsSeparate(t *testing.T) {
	limiter := NewRequestLimiter(2)
	clients := []string{"198.51.100.1", "198.51.100.2", "198.51.100.3"}
	var wait sync.WaitGroup
	for _, client := range clients {
		wait.Add(1)
		go func(client string) {
			defer wait.Done()
			if !limiter.Allow(client) || !limiter.Allow(client) || limiter.Allow(client) {
				t.Errorf("budget was not isolated for %s", client)
			}
		}(client)
	}
	wait.Wait()
}

func TestClientKeyTrustsForwardedAddressOnlyWhenConfigured(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(clientKey(r.Context()))) })
	request := httptest.NewRequest(http.MethodPost, "/", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	request.Header.Set("X-Forwarded-For", "198.51.100.4")
	response := httptest.NewRecorder()
	withClientKey(handler, false).ServeHTTP(response, request)
	if response.Body.String() != "192.0.2.10" {
		t.Fatalf("untrusted client key = %q", response.Body.String())
	}
	response = httptest.NewRecorder()
	withClientKey(handler, true).ServeHTTP(response, request)
	if response.Body.String() != "198.51.100.4" {
		t.Fatalf("trusted client key = %q", response.Body.String())
	}
}
