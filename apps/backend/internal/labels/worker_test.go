package labels

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTier1WorkerIngestion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`[
			{
				"address": "0x1111111111111111111111111111111111111111",
				"name": "Test Sanctioned Wallet",
				"category": "Sanctions",
				"source": "OFAC_SDN"
			}
		]`))
	}))
	defer server.Close()

	registry := NewRegistry()
	worker := NewTier1Worker(nil, registry, server.URL)
	ctx := context.Background()

	err := worker.IngestTier1Datasets(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	labels := registry.GetLabels(ctx, "0x1111111111111111111111111111111111111111")
	if len(labels) == 0 {
		t.Errorf("expected ingested label, got 0")
	} else if labels[0].Label != "Test Sanctioned Wallet" {
		t.Errorf("expected label name 'Test Sanctioned Wallet', got '%s'", labels[0].Label)
	}
}

func TestTier1WorkerFallback(t *testing.T) {
	registry := NewRegistry()
	// Unreachable server URL
	worker := NewTier1Worker(nil, registry, "http://localhost:59999")
	ctx := context.Background()

	err := worker.IngestTier1Datasets(ctx)
	if err != nil {
		t.Fatalf("expected fallback seed without error, got %v", err)
	}

	labels := registry.GetLabels(ctx, "0x098B716B8Aaf21512996dC57EB0615e2383E2f96")
	if len(labels) == 0 {
		t.Errorf("expected fallback Tornado Cash label, got 0")
	}
}
