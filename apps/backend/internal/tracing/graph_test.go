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
	_, err := engine.ResolveGraph(context.Background(), "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D", DirectionBoth, 10, "", DefaultCounterpartyLimit, RankingMostRecent)
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

func TestConfirmationBackedObservationsRequireCurrentChainHeight(t *testing.T) {
	source := adapter.SourceStatus{LatestChainBlock: 1_000}
	if isProvisional("ethereum-mainnet", 936, source) {
		t.Fatal("64 Ethereum confirmations should be confirmation-backed")
	}
	if !isProvisional("ethereum-mainnet", 937, source) {
		t.Fatal("63 Ethereum confirmations must remain provisional")
	}
	if !isProvisional("ethereum-mainnet", 900, adapter.SourceStatus{}) {
		t.Fatal("missing current chain height must remain provisional")
	}
	if !isProvisional("solana-mainnet", 900, source) {
		t.Fatal("unsupported confirmation source must remain provisional")
	}
}

func TestConfirmationStatusWarnsWhenAChainHeadIsUnavailable(t *testing.T) {
	status := confirmationSourceStatus("solana-mainnet", adapter.SourceStatus{Source: "test"})
	if status.Warning == "" {
		t.Fatal("unsupported confirmation status must be visible to investigators")
	}
	if confirmationSourceStatus("ethereum-mainnet", adapter.SourceStatus{LatestChainBlock: 1}).Warning != "" {
		t.Fatal("available EVM confirmation height must not add a warning")
	}
}

func TestSelectCounterpartyTransfersUsesDeterministicRequestedRanking(t *testing.T) {
	seed := testTraceAddress
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	transfers := []adapter.TransferItem{
		{From: seed, To: "recent", AmountBaseUnits: "1", Asset: adapter.Asset{Decimals: 0}, Timestamp: now},
		{From: seed, To: "large", AmountBaseUnits: "99", Asset: adapter.Asset{Decimals: 0}, Timestamp: now.Add(-time.Hour)},
		{From: seed, To: "total", AmountBaseUnits: "50", Asset: adapter.Asset{Decimals: 0}, Timestamp: now.Add(-4 * time.Hour)},
		{From: seed, To: "total", AmountBaseUnits: "50", Asset: adapter.Asset{Decimals: 0}, Timestamp: now.Add(-5 * time.Hour)},
		{From: seed, To: "active", AmountBaseUnits: "2", Asset: adapter.Asset{Decimals: 0}, Timestamp: now.Add(-2 * time.Hour)},
		{From: seed, To: "active", AmountBaseUnits: "2", Asset: adapter.Asset{Decimals: 0}, Timestamp: now.Add(-3 * time.Hour)},
	}
	if got := selectCounterpartyTransfers(transfers, seed, 1, RankingMostRecent); len(got) != 1 || got[0].To != "recent" {
		t.Fatalf("recent = %#v", got)
	}
	if got := selectCounterpartyTransfers(transfers, seed, 1, RankingTotalRawAmount); len(got) != 2 || got[0].To != "total" {
		t.Fatalf("total = %#v", got)
	}
	if got := selectCounterpartyTransfers(transfers, seed, 1, RankingMostActive); len(got) != 2 || got[0].To != "active" {
		t.Fatalf("active = %#v", got)
	}
	if got := selectCounterpartyTransfersWithKnown(transfers, seed, 1, RankingKnownEntity, map[string]bool{"large": true}); len(got) != 1 || got[0].To != "large" {
		t.Fatalf("known = %#v", got)
	}
}

func TestSelectCounterpartyTransfersRanksTotalsPerAsset(t *testing.T) {
	seed := testTraceAddress
	now := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)
	transfers := []adapter.TransferItem{
		{From: seed, To: "eth", AmountBaseUnits: "1000000000000000000", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: now},
		{From: seed, To: "usdc", AmountBaseUnits: "1000000000", Asset: adapter.Asset{Kind: "ERC20", ContractAddress: "0xusdc", Symbol: "USDC", Decimals: 6}, Timestamp: now.Add(-time.Minute)},
		{From: seed, To: "smaller-eth", AmountBaseUnits: "2", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: now.Add(-2 * time.Minute)},
	}
	got := selectCounterpartyTransfers(transfers, seed, 1, RankingTotalRawAmount)
	if len(got) != 2 || got[0].To != "eth" || got[1].To != "usdc" {
		t.Fatalf("asset-specific totals = %#v", got)
	}
}

func TestGraphControlsClampAndDefault(t *testing.T) {
	limit, ranking := graphControls(0, "")
	if limit != DefaultCounterpartyLimit || ranking != RankingMostRecent {
		t.Fatalf("defaults = %d %q", limit, ranking)
	}
	limit, _ = graphControls(MaxCounterpartyLimit+1, RankingMostRecent)
	if limit != MaxCounterpartyLimit {
		t.Fatalf("limit = %d", limit)
	}
}

func TestGraphAggregatesPerNodeTransferCountsAndOrdersEdges(t *testing.T) {
	seed := testTraceAddress
	engine := NewEngine(&retryChain{}, nil, labels.NewService(nil))
	graph := engine.graph(context.Background(), seed, []adapter.TransferItem{
		{Hash: "0xbbb", EventID: "tx", From: "source", To: seed, AmountBaseUnits: "2000000000000000000", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: time.Unix(100, 0)},
		{Hash: "0xaaa", EventID: "tx", From: "source", To: seed, AmountBaseUnits: "1000000000000000000", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: time.Unix(90, 0)},
		{Hash: "0xccc", EventID: "tx", From: seed, To: "destination", AmountBaseUnits: "3000000000000000000", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: time.Unix(110, 0)},
	}, &adapter.TransferPage{SourceStatus: adapter.SourceStatus{Source: "fixture", RetrievedAt: time.Unix(1000, 0)}})
	nodes := make(map[string]GraphNode, len(graph.Nodes))
	for _, node := range graph.Nodes {
		nodes[node.ID] = node
	}
	if graph.TotalNodes != 3 || graph.TotalEdges != 3 || nodes[seed].InTxCount != 2 || nodes[seed].OutTxCount != 1 || nodes["source"].OutTxCount != 2 || nodes["destination"].InTxCount != 1 {
		t.Fatalf("graph aggregation = %#v", graph)
	}
	if graph.Edges[0].ID != "ethereum-mainnet:0xaaa:tx" || graph.Edges[0].AmountFormatted != "1.0000 ETH" || graph.Edges[2].ID != "ethereum-mainnet:0xccc:tx" {
		t.Fatalf("edges = %#v", graph.Edges)
	}
	if volumes := nodes[seed].TotalVolumeByAsset; len(volumes) != 1 || volumes[0].AmountBaseUnits != "6000000000000000000" || volumes[0].Asset.Symbol != "ETH" {
		t.Fatalf("seed asset volume = %#v", volumes)
	}
}

func TestTraceCoverageDescribesRetrievedScope(t *testing.T) {
	page := &adapter.TransferPage{
		Transfers:  []adapter.TransferItem{{}, {}, {}},
		NextCursor: "next",
		HasMore:    true,
		SourceStatus: adapter.SourceStatus{
			IsComplete: false,
		},
	}
	coverage := traceCoverage(50, "start", page, []db.Transfer{{Provisional: false}, {Provisional: true}})
	if coverage.RequestedPageSize != 50 || coverage.ObservedTransferCount != 3 || coverage.GraphTransferCount != 2 || coverage.ConfirmationBackedTransferCount != 1 || coverage.ProvisionalTransferCount != 1 || !coverage.HasMore || coverage.ProviderComplete || coverage.Cursor != "start" || coverage.Limitation == "" {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestTraceJobRetriesTemporaryProviderFailure(t *testing.T) {
	chain := &retryChain{}
	queue := NewQueue(NewEngine(chain, nil, labels.NewService(nil)), nil, 1, 1)
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
