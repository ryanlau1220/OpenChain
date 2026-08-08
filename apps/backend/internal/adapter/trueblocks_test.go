package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTrueBlocksGetSyncStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{
			"data": [
				{
					"clientBlock": 20000000,
					"scrapeBlock": 19500000,
					"indexedBlock": 19500000
				}
			]
		}`))
	}))
	defer server.Close()

	tb := NewTrueBlocksAdapter(server.URL, "")
	ctx := context.Background()

	status, err := tb.GetSyncStatus(ctx)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if status.IndexedUpToBlock != 19500000 {
		t.Errorf("expected IndexedUpToBlock 19500000, got %d", status.IndexedUpToBlock)
	}

	if status.IsSynced {
		t.Errorf("expected IsSynced false when index is behind client block")
	}

	if status.WarningMessage == "" {
		t.Errorf("expected non-empty warning message when index is behind")
	}
}

func TestTrueBlocksOfflineFallback(t *testing.T) {
	// Connect to invalid port
	tb := NewTrueBlocksAdapter("http://localhost:59999", "")
	ctx := context.Background()

	status, err := tb.GetSyncStatus(ctx)
	if err != nil {
		t.Fatalf("expected no error on offline fallback, got %v", err)
	}

	if status.ScrapeStatus != "OFFLINE" {
		t.Errorf("expected OFFLINE status, got %s", status.ScrapeStatus)
	}
}
