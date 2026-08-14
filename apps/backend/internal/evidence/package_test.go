package evidence

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
)

func TestBuildCreatesManifestForFrozenPayload(t *testing.T) {
	now := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	packageFile, err := Build([]byte(`{"version":1,"title":"Case"}`), &db.EvidenceExport{
		Transfers:      []db.Transfer{{ID: "transfer-1", Network: "ethereum-mainnet", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, BlockTimestamp: now, RetrievedAt: now}},
		Snapshots:      []db.AcquisitionSnapshot{{ID: 7, Network: "ethereum-mainnet", Provider: "test", RequestIdentity: "GET /", Hash: "hash", Response: []byte(`{"ok":true}`), RetrievedAt: now}},
		Scopes:         []db.RecordedAcquisitionScope{{ID: 4, Network: "ethereum-mainnet", Address: "0xaddress", Cursor: "page-1", RetrievedAt: now}},
		ScopeTransfers: []db.AcquisitionScopeTransfer{{ScopeID: 4, TransferID: "transfer-1"}},
		ScopeSnapshots: []db.AcquisitionScopeSnapshot{{ScopeID: 4, AcquisitionID: 7}},
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(packageFile.Payload)
	if err != nil || packageFile.Format != Format || packageFile.Version != Version || packageFile.Manifest.Algorithm != "SHA-256" || packageFile.Manifest.PayloadSHA256 == "" {
		t.Fatalf("package = %#v err = %v", packageFile, err)
	}
	if string(payload) == "" || packageFile.Payload.Snapshots[0].ResponseBodyBase64 != "eyJvayI6dHJ1ZX0=" {
		t.Fatalf("payload = %s", payload)
	}
	hash := sha256.Sum256(payload)
	if packageFile.Manifest.PayloadSHA256 != fmt.Sprintf("%x", hash[:]) {
		t.Fatalf("manifest = %#v", packageFile.Manifest)
	}
}
