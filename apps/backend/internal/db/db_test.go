package db

import (
	"context"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig("")
	if cfg.GraphName != "openchain" {
		t.Errorf("expected graph name 'openchain', got '%s'", cfg.GraphName)
	}
	if cfg.MaxOpenConns != 25 {
		t.Errorf("expected MaxOpenConns 25, got %d", cfg.MaxOpenConns)
	}
}

func TestGraphModels(t *testing.T) {
	node := Node{
		Address: "0x1234567890abcdef1234567890abcdef12345678",
		Label:   VertexWallet,
	}
	if node.Label != "Wallet" {
		t.Errorf("expected vertex label Wallet, got %s", node.Label)
	}

	edge := Edge{
		Hash:        "0xabcdef",
		FromAddress: "0x111",
		ToAddress:   "0x222",
		Label:       EdgeTransfer,
	}
	if edge.Label != "TRANSFER" {
		t.Errorf("expected edge label TRANSFER, got %s", edge.Label)
	}
}

func TestNewDBInitialization(t *testing.T) {
	cfg := DefaultConfig("postgres://invalid:invalid@localhost:5432/invalid?sslmode=disable")
	db, err := NewDB(cfg)
	if err != nil {
		t.Fatalf("expected NewDB not to fail on constructor, got %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // instantly canceled context
	err = db.InitSchema(ctx)
	if err == nil {
		t.Errorf("expected error on canceled context, got nil")
	}
}

