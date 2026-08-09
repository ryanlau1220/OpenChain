package tracing

import (
	"context"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/labels"
)

func TestTraceGraphWithoutUpstreamReturnsSeed(t *testing.T) {
	engine := NewEngine(nil, labels.NewRegistry())
	result, err := engine.TraceGraph(context.Background(), "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D")
	if err != nil { t.Fatal(err) }
	if result.SeedAddress != "0x7a250d5630b4cf539739df2c5dacb4c659f2488d" { t.Fatalf("seed = %s", result.SeedAddress) }
	if len(result.Nodes) != 1 || !result.Nodes[0].IsSeed { t.Fatalf("nodes = %#v", result.Nodes) }
}
