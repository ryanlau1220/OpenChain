package tracing

import (
	"context"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
)

func TestTracingEngine(t *testing.T) {
	evmClient := adapter.NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	labelRegistry := labels.NewRegistry()
	riskEvaluator := risk.NewEvaluator(labelRegistry, nil)
	engine := NewEngine(evmClient, nil, nil, labelRegistry, riskEvaluator)

	ctx := context.Background()
	seed := "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"

	result, err := engine.TraceMultiHopGraph(ctx, seed, "ETHEREUM_SEPOLIA", 2, "BOTH")
	if err != nil {
		t.Fatalf("unexpected error during tracing: %v", err)
	}

	if len(result.SeedAddresses) == 0 || result.SeedAddresses[0] != "0x7a250d5630b4cf539739df2c5dacb4c659f2488d" {
		t.Errorf("expected normalized seed address, got %v", result.SeedAddresses)
	}

	if len(result.Nodes) == 0 {
		t.Errorf("expected at least seed node in graph")
	}

	// Verify seed node properties
	seedNode := result.Nodes[0]
	if !seedNode.IsSeed {
		t.Errorf("expected IsSeed to be true for first node")
	}
}

func TestMultiAddressTracing(t *testing.T) {
	evmClient := adapter.NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	labelRegistry := labels.NewRegistry()
	riskEvaluator := risk.NewEvaluator(labelRegistry, nil)
	engine := NewEngine(evmClient, nil, nil, labelRegistry, riskEvaluator)

	ctx := context.Background()
	seeds := []string{
		"0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
		"0x1111111111111111111111111111111111111111",
	}

	result, err := engine.TraceMultiAddressGraph(ctx, seeds, "ETHEREUM_SEPOLIA", 2, "BOTH", []string{"ETH", "USDT"})
	if err != nil {
		t.Fatalf("unexpected error during multi-address tracing: %v", err)
	}

	if len(result.SeedAddresses) != 2 {
		t.Errorf("expected 2 seed addresses, got %d", len(result.SeedAddresses))
	}
}
