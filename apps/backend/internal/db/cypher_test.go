package db

import (
	"context"
	"testing"
)

func TestConnectAgeFallback(t *testing.T) {
	d := &DB{
		DSN:       "postgres://openchain:openchain_secret@localhost:5432/openchain?sslmode=disable",
		GraphName: "",
	}

	ag, err := d.ConnectAge()
	if err != nil {
		t.Fatalf("expected ConnectAge not to fail, got %v", err)
	}
	if ag == nil {
		t.Fatal("expected non-nil Age instance")
	}
}

func TestQueryHopGraphEmpty(t *testing.T) {
	cfg := DefaultConfig("postgres://openchain:openchain_secret@localhost:5432/openchain?sslmode=disable")
	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("NewDB error: %v", err)
	}
	defer func() { _ = db.Close() }()

	ctx := context.Background()
	res, err := db.QueryHopGraph(ctx, "0x0000000000000000000000000000000000000000", 1)
	if err != nil {
		t.Fatalf("QueryHopGraph returned unexpected error: %v", err)
	}
	if len(res.Nodes) == 0 {
		t.Errorf("expected at least root node in empty graph result")
	}
}
