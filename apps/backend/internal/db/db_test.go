package db

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("")
	if config.DSN == "" {
		t.Fatalf("config = %#v", config)
	}
}

func TestSaveGraphIntegration(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test Apache AGE persistence")
	}
	database, err := NewDB(DefaultConfig(os.Getenv("DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	from := "0x1000000000000000000000000000000000000001"
	to := "0x2000000000000000000000000000000000000002"
	transfer := Transfer{ID: "ethereum-mainnet:test-graph:0", Network: "ethereum-mainnet", TransactionHash: "0xtest-graph", FromAddress: from, ToAddress: to, AssetSymbol: "ETH", AmountBaseUnits: "42", BlockNumber: 1, BlockTimestamp: time.Unix(1, 0), Source: "test", RetrievedAt: time.Now().UTC()}
	defer func() {
		cleanupGraph(t, database, transfer.ID, from, to)
		_, _ = database.SQL.Exec(`DELETE FROM transfers WHERE id = $1`, transfer.ID)
	}()
	if err := database.SaveGraph(ctx, []Address{{Address: from, Label: "From", EntityType: "EOA"}, {Address: to, Label: "To", EntityType: "EOA"}}, []Transfer{transfer}); err != nil {
		t.Fatal(err)
	}
	var relationalCount int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM transfers WHERE id = $1`, transfer.ID).Scan(&relationalCount); err != nil {
		t.Fatal(err)
	}
	if relationalCount != 1 || graphFundFlowCount(t, database, transfer.ID) != 1 {
		t.Fatalf("transfer was not persisted to both stores")
	}
}

func graphFundFlowCount(t *testing.T, database *DB, id string) int {
	t.Helper()
	ctx := context.Background()
	tx, err := database.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	var count string
	err = tx.QueryRowContext(ctx, `SELECT * FROM cypher('openchain', $$ MATCH ()-[flow:FundFlow {id: $id}]->() RETURN count(flow) $$, $1) AS (count agtype)`, string(encoded)).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	value, err := strconv.Atoi(count)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func cleanupGraph(t *testing.T, database *DB, flowID, from, to string) {
	t.Helper()
	ctx := context.Background()
	tx, err := database.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		query  string
		params map[string]string
	}{
		{`SELECT * FROM cypher('openchain', $$ MATCH ()-[flow:FundFlow {id: $id}]->() DELETE flow RETURN count(flow) $$, $1) AS (result agtype)`, map[string]string{"id": flowID}},
		{`SELECT * FROM cypher('openchain', $$ MATCH (address:Address {address: $address}) DETACH DELETE address RETURN count(address) $$, $1) AS (result agtype)`, map[string]string{"address": from}},
		{`SELECT * FROM cypher('openchain', $$ MATCH (address:Address {address: $address}) DETACH DELETE address RETURN count(address) $$, $1) AS (result agtype)`, map[string]string{"address": to}},
	} {
		if err := execCypher(ctx, tx, query.query, query.params); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
