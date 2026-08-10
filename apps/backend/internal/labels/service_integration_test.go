package labels

import (
	"context"
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
	if err := database.InitSchema(ctx); err != nil {
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
	search, err := service.SearchLabels(ctx, "ethereum-mainnet", "Uniswap", "DeFi", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(search) != 1 || search[0].ID != items[0].ID {
		t.Fatalf("search result = %#v", search)
	}
	var count int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM curated_labels WHERE id = $1`, items[0].ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("seed import count = %d", count)
	}
}
