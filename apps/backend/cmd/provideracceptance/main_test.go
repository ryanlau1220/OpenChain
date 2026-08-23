package main

import (
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
)

func TestValidatePageRejectsIncompleteEvidenceAndPagination(t *testing.T) {
	valid := &adapter.TransferPage{Transfers: []adapter.TransferItem{{Hash: "tx", EventID: "event", From: "from", To: "to", AmountBaseUnits: "1", Asset: adapter.Asset{Kind: "NATIVE"}, Timestamp: time.Now()}}, SourceStatus: adapter.SourceStatus{Source: "provider", RetrievedAt: time.Now()}}
	if err := validatePage(valid, "provider"); err != nil {
		t.Fatalf("valid page rejected: %v", err)
	}
	valid.HasMore = true
	if err := validatePage(valid, "provider"); err == nil {
		t.Fatal("missing cursor accepted")
	}
}
