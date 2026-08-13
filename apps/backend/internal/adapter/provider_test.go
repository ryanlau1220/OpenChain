package adapter

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
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

func TestAcquisitionIdentityRedactsCredentials(t *testing.T) {
	ctx, recorder := WithAcquisitionRecorder(context.Background())
	request, err := http.NewRequest(http.MethodPost, (&url.URL{Scheme: "https", Host: "provider.test", Path: "/history", RawQuery: "address=test&api-key=secret&token=private"}).String(), bytes.NewReader([]byte(`{"address":"test"}`)))
	if err != nil {
		t.Fatal(err)
	}
	recordAcquisition(ctx, "test", request, []byte(`{"result":[]}`))
	items := recorder.Items()
	if len(items) != 1 || strings.Contains(items[0].RequestIdentity, "secret") || strings.Contains(items[0].RequestIdentity, "private") || !strings.Contains(items[0].RequestIdentity, "redacted") || !strings.Contains(items[0].RequestIdentity, "body-sha256=") {
		t.Fatalf("acquisition identity = %#v", items)
	}
}

func TestAcquisitionIdentityRedactsAlchemyPathCredential(t *testing.T) {
	ctx, recorder := WithAcquisitionRecorder(context.Background())
	request, err := http.NewRequest(http.MethodPost, "https://eth-mainnet.g.alchemy.com/v2/secret-api-key", bytes.NewReader([]byte(`{}`)))
	if err != nil {
		t.Fatal(err)
	}
	recordAcquisition(ctx, AlchemySource, request, []byte(`{"result":[]}`))
	items := recorder.Items()
	if len(items) != 1 || strings.Contains(items[0].RequestIdentity, "secret-api-key") || !strings.Contains(items[0].RequestIdentity, "/v2/redacted") {
		t.Fatalf("acquisition identity = %#v", items)
	}
}

func TestProviderErrorRedactsProviderEndpointDetails(t *testing.T) {
	err := NewProviderTransportError("etherscan-v2", errors.New("https://api.example/?apikey=secret"))
	if err.Error() != "etherscan-v2 request failed" {
		t.Fatalf("provider error = %q", err)
	}
}
