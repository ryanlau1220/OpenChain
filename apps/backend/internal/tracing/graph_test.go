package tracing

import (
	"context"
	"math/big"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
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

func TestTraceJobRetriesTemporaryProviderFailure(t *testing.T) {
	chain := &retryChain{}
	queue := NewQueue(NewEngine(chain, nil, labels.NewService(nil)), nil, 1)
	result, err := queue.resolveWithRetry(context.Background(), &db.TraceJob{Query: db.TraceJobQuery{Network: chain.Network(), Address: testTraceAddress, Direction: string(DirectionBoth), Limit: 1}})
	if err != nil || result == nil || chain.attempts != 2 {
		t.Fatalf("result = %#v, attempts = %d, err = %v", result, chain.attempts, err)
	}
}

const testTraceAddress = "0x7a250d5630b4cf539739df2c5dacb4c659f2488d"

type retryChain struct{ attempts int }

func (c *retryChain) Network() string                                       { return "ethereum-mainnet" }
func (c *retryChain) NormalizeAddress(value string) (string, error)         { return value, nil }
func (c *retryChain) NormalizeTransactionHash(value string) (string, error) { return value, nil }
func (c *retryChain) NativeAsset() adapter.Asset {
	return adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}
func (c *retryChain) ActivityLabel() string                                { return "" }
func (c *retryChain) GetBalance(context.Context, string) (*big.Int, error) { return big.NewInt(0), nil }
func (c *retryChain) GetTxCount(context.Context, string) (uint64, error)   { return 0, nil }
func (c *retryChain) IsContract(context.Context, string) (bool, error)     { return false, nil }
func (c *retryChain) ListTransfers(context.Context, string, uint32, string) (*adapter.TransferPage, error) {
	c.attempts++
	if c.attempts == 1 {
		return nil, adapter.NewProviderRateLimitError("test")
	}
	return &adapter.TransferPage{SourceStatus: adapter.SourceStatus{Source: "test", RetrievedAt: time.Now().UTC(), IsComplete: true}}, nil
}
func (c *retryChain) LookupTransaction(context.Context, string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	return nil, adapter.SourceStatus{}, nil
}
func (c *retryChain) GetContractMetadata(context.Context, string) (*adapter.ContractMetadata, error) {
	return nil, nil
}
func (c *retryChain) SourceStatus() adapter.SourceStatus { return adapter.SourceStatus{Source: "test"} }
