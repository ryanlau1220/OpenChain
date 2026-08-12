package adapter

import (
	"errors"
	"net/http"
	"testing"
	"time"
)

func TestRetryDelayOnlyRetriesTemporaryProviderFailures(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		retry bool
	}{
		{name: "rate limited", err: NewProviderRateLimitError("test"), retry: true},
		{name: "server failure", err: &ProviderError{Provider: "test", StatusCode: http.StatusBadGateway}, retry: true},
		{name: "transport failure", err: NewProviderTransportError("test", errors.New("network unavailable")), retry: true},
		{name: "invalid request", err: &ProviderError{Provider: "test", StatusCode: http.StatusBadRequest}, retry: false},
		{name: "invalid data", err: errors.New("invalid data"), retry: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			delay, retry := RetryDelay(test.err, 1)
			if retry != test.retry {
				t.Fatalf("retry = %v", retry)
			}
			if retry && delay < time.Second {
				t.Fatalf("delay = %s", delay)
			}
		})
	}
}

func TestProviderErrorRedactsProviderEndpointDetails(t *testing.T) {
	err := NewProviderTransportError("etherscan-v2", errors.New("https://api.example/?apikey=secret"))
	if err.Error() != "etherscan-v2 request failed" {
		t.Fatalf("provider error = %q", err)
	}
}
