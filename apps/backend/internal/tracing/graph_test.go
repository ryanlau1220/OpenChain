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
	riskEvaluator := risk.NewEvaluator(labelRegistry)
	engine := NewEngine(evmClient, labelRegistry, riskEvaluator)

	ctx := context.Background()
	seed := "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"

	result, err := engine.TraceMultiHopGraph(ctx, seed, "ETHEREUM_SEPOLIA", 2, "BOTH")
	if err != nil {
		t.Fatalf("unexpected error during tracing: %v", err)
	}

	if result.SeedAddress != "0x7a250d5630b4cf539739df2c5dacb4c659f2488d" {
		t.Errorf("expected normalized seed address, got %s", result.SeedAddress)
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
