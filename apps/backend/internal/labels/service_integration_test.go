package labels

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

func TestSeedImportIsIdempotentAndSearchable(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test curated label persistence")
	}
	database, err := db.NewDB(db.DefaultConfig(os.Getenv("DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	service := NewService(database)
	if err := service.ImportSeed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := service.ImportSeed(ctx); err != nil {
		t.Fatal(err)
	}
	items, err := service.GetLabels(ctx, "ethereum-mainnet", "0x7A250D5630B4CF539739DF2C5DACB4C659F2488D")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Uniswap V2 Router" || items[0].SourceVersion == "" {
		t.Fatalf("imported labels = %#v", items)
	}
	search, err := service.SearchLabels(ctx, "ethereum-mainnet", "Uniswap", string(CategoryDeFiService), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 || search[0].ID != items[0].ID {
		t.Fatalf("search result = %#v", search)
	}
	var count int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM label_assertions WHERE id = $1`, items[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("seed import count = %d", count)
	}
	var evidenceSnapshot string
	if err := database.SQL.QueryRowContext(ctx, `SELECT encode(evidence.content, 'escape') FROM label_assertions assertion JOIN label_evidence evidence ON evidence.sha256 = assertion.evidence_sha256 WHERE assertion.id = $1`, items[0].ID).Scan(&evidenceSnapshot); err != nil || evidenceSnapshot == "" {
		t.Fatalf("stored label evidence = %q err=%v", evidenceSnapshot, err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE label_assertions SET label = 'mutated' WHERE id = $1`, items[0].ID); err == nil {
		t.Fatal("immutable label assertion accepted mutation")
	}
	assertionKey := fmt.Sprintf("versioned-label-%d", time.Now().UnixNano())
	address := fmt.Sprintf("0x%040x", time.Now().UnixNano())
	first := db.CuratedLabel{ID: assertionKey + "@v1", AssertionKey: assertionKey, Network: "ethereum-mainnet", Address: address, Category: string(CategoryDeFiService), Label: "Version one", Confidence: 1, EvidenceURL: "https://example.test/v1", EvidenceSnapshot: `{"source":"v1"}`, Source: "integration test", SourceVersion: "v1", ReviewState: "approved", Visibility: "public", TrustTier: 1, ValidFrom: time.Now().Add(-time.Minute), CreatedBy: "test", CreatedAt: time.Now().Add(-time.Minute)}
	second := first
	second.ID = assertionKey + "@v2"
	second.Label = "Version two"
	second.EvidenceURL = "https://example.test/v2"
	second.EvidenceSnapshot = `{"source":"v2"}`
	second.SourceVersion = "v2"
	second.SupersedesID = first.ID
	if err := database.InsertLabelAssertions(ctx, []db.CuratedLabel{first, second}); err != nil {
		t.Fatal(err)
	}
	current, err := database.GetCuratedLabels(ctx, "ethereum-mainnet", address)
	if err != nil || len(current) != 1 || current[0].ID != second.ID || current[0].SupersedesID != first.ID {
		t.Fatalf("current label assertion = %#v err=%v", current, err)
	}
	conflict := second
	conflict.EvidenceSnapshot = `{"source":"altered"}`
	if err := database.InsertLabelAssertions(ctx, []db.CuratedLabel{conflict}); err == nil {
		t.Fatal("label assertion reused a version with different evidence")
	}
}
