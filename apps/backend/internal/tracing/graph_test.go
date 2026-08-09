package tracing

import (
	"context"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/labels"
)

func TestTraceGraphWithoutUpstreamReturnsSeed(t *testing.T) {
	engine := NewEngine(nil, nil, nil, labels.NewRegistry())
	_, err := engine.TraceGraph(context.Background(), "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", DirectionBoth, 10, "")
	if err == nil {
		t.Fatal("expected an unavailable TrueBlocks error")
	}
}
