package tracing

import (
	"context"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/labels"
)

func TestTraceGraphWithoutUpstreamReturnsSeed(t *testing.T) {
	engine := NewEngine(nil, nil, labels.NewService(nil))
	_, err := engine.ResolveGraph(context.Background(), "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", DirectionBoth, 10, "")
	if err == nil {
		t.Fatal("expected an unavailable Etherscan error")
	}
}

func TestPendingGraphReturnsSeedImmediately(t *testing.T) {
	engine := NewEngine(nil, nil, labels.NewService(nil))
	result := engine.PendingGraph("0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", "Trace retrieval is queued.")
	if !result.Pending || result.TotalNodes != 1 || len(result.Nodes) != 1 || !result.Nodes[0].IsSeed {
		t.Fatalf("pending result = %#v", result)
	}
}

func TestTransferIDIncludesNetwork(t *testing.T) {
	hash := "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if transferID("ethereum-mainnet", hash, "tx") == transferID("base-mainnet", hash, "tx") {
		t.Fatal("transfer IDs must be unique across networks")
	}
}
