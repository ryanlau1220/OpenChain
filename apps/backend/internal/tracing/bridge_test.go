package tracing

import (
	"context"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/rules"
)

func TestBridgeCorrelatorMatchesOnlyCanonicalTimedSameAmountTransfers(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	sourceTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	chain := &bridgeTestChain{page: &adapter.TransferPage{Transfers: []adapter.TransferItem{{Hash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EventID: "log:1", TransferKind: "ERC20", From: "0x4200000000000000000000000000000000000010", To: owner, AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "ERC20", Symbol: "USDC", Decimals: 6}, Timestamp: sourceTime.Add(time.Hour)}}, SourceStatus: adapter.SourceStatus{Source: "test", RetrievedAt: sourceTime.Add(2 * time.Hour)}}}
	correlator := NewBridgeCorrelator(map[string]adapter.ChainAdapter{"base-mainnet": chain})
	evidence := correlator.Correlate(context.Background(), "ethereum-mainnet", []db.Transfer{{ID: "ethereum-mainnet:source", Network: "ethereum-mainnet", FromAddress: owner, ToAddress: "0x3154cf16ccdb4c6d922629664174b904d80f2c35", AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "ERC20", Symbol: "USDC", Decimals: 6}, BlockTimestamp: sourceTime}})
	if len(evidence) != 1 || evidence[0].Candidate.DestinationNetwork != "base-mainnet" || evidence[0].Candidate.Destination.ID == "" {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestBridgeCorrelatorRejectsDifferentAssetKind(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	now := time.Now().UTC().Add(-time.Hour)
	chain := &bridgeTestChain{page: &adapter.TransferPage{Transfers: []adapter.TransferItem{{Hash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EventID: "tx", TransferKind: "NATIVE", From: "0x4200000000000000000000000000000000000010", To: owner, AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, Timestamp: now.Add(time.Minute)}}, SourceStatus: adapter.SourceStatus{RetrievedAt: now.Add(time.Hour)}}}
	correlator := NewBridgeCorrelator(map[string]adapter.ChainAdapter{"base-mainnet": chain})
	evidence := correlator.Correlate(context.Background(), "ethereum-mainnet", []db.Transfer{{Network: "ethereum-mainnet", FromAddress: owner, ToAddress: "0x3154cf16ccdb4c6d922629664174b904d80f2c35", AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "ERC20"}, BlockTimestamp: now}})
	if len(evidence) != 0 {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestBridgeCorrelatorRejectsDifferentReportedAssetSymbol(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	now := time.Now().UTC().Add(-time.Hour)
	chain := &bridgeTestChain{page: &adapter.TransferPage{Transfers: []adapter.TransferItem{{Hash: "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", EventID: "tx", TransferKind: "ERC20", From: "0x4200000000000000000000000000000000000010", To: owner, AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "ERC20", Symbol: "USDT", Decimals: 6}, Timestamp: now.Add(time.Minute)}}, SourceStatus: adapter.SourceStatus{RetrievedAt: now.Add(time.Hour)}}}
	correlator := NewBridgeCorrelator(map[string]adapter.ChainAdapter{"base-mainnet": chain})
	evidence := correlator.Correlate(context.Background(), "ethereum-mainnet", []db.Transfer{{Network: "ethereum-mainnet", FromAddress: owner, ToAddress: "0x3154cf16ccdb4c6d922629664174b904d80f2c35", AmountBaseUnits: "1000000", Asset: adapter.Asset{Kind: "ERC20", Symbol: "USDC", Decimals: 6}, BlockTimestamp: now}})
	if len(evidence) != 0 {
		t.Fatalf("evidence = %#v", evidence)
	}
}

func TestCrossChainTransitionKeepsEvidenceWithoutOwnershipClaim(t *testing.T) {
	source := db.Transfer{ID: "ethereum-mainnet:source", Network: "ethereum-mainnet", TransactionHash: "0xsource", Asset: adapter.Asset{Kind: "ERC20", Symbol: "USDC", Decimals: 6}, AmountBaseUnits: "1000000", BlockTimestamp: time.Unix(100, 0)}
	destination := db.Transfer{ID: "base-mainnet:destination", Network: "base-mainnet", TransactionHash: "0xdestination", BlockTimestamp: time.Unix(200, 0)}
	transition := crossChainTransition(rules.BridgeCandidate{BridgeName: "Base Standard Bridge", Source: source, Destination: destination}, "0xsourcebridge", "0xdestinationbridge")
	if transition.ID != "cross-chain:ethereum-mainnet:source:base-mainnet:destination" || transition.Source.TransactionHash != "0xsource" {
		t.Fatalf("transition = %#v", transition)
	}
	if !strings.Contains(transition.Limitations, "does not establish cross-chain address ownership") {
		t.Fatalf("limitations = %q", transition.Limitations)
	}
}

func TestBridgeCorrelatorFetchesDestinationHistoryOncePerOwnerAndRoute(t *testing.T) {
	owner := "0x1111111111111111111111111111111111111111"
	now := time.Now().UTC().Add(-time.Hour)
	chain := &bridgeTestChain{page: &adapter.TransferPage{SourceStatus: adapter.SourceStatus{RetrievedAt: now.Add(time.Hour)}}}
	correlator := NewBridgeCorrelator(map[string]adapter.ChainAdapter{"base-mainnet": chain})
	correlator.Correlate(context.Background(), "ethereum-mainnet", []db.Transfer{
		{Network: "ethereum-mainnet", FromAddress: owner, ToAddress: "0x3154cf16ccdb4c6d922629664174b904d80f2c35", AmountBaseUnits: "1", Asset: adapter.Asset{Kind: "ERC20"}, BlockTimestamp: now},
		{Network: "ethereum-mainnet", FromAddress: owner, ToAddress: "0x3154cf16ccdb4c6d922629664174b904d80f2c35", AmountBaseUnits: "2", Asset: adapter.Asset{Kind: "ERC20"}, BlockTimestamp: now},
	})
	if chain.calls != 1 {
		t.Fatalf("destination history calls = %d", chain.calls)
	}
}

type bridgeTestChain struct {
	page  *adapter.TransferPage
	calls int
}

func (c *bridgeTestChain) Network() string                                       { return "base-mainnet" }
func (c *bridgeTestChain) NormalizeAddress(value string) (string, error)         { return value, nil }
func (c *bridgeTestChain) NormalizeTransactionHash(value string) (string, error) { return value, nil }
func (c *bridgeTestChain) NativeAsset() adapter.Asset {
	return adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}
func (c *bridgeTestChain) ActivityLabel() string { return "" }
func (c *bridgeTestChain) GetBalance(context.Context, string) (*big.Int, error) {
	return big.NewInt(0), nil
}
func (c *bridgeTestChain) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }
func (c *bridgeTestChain) IsContract(context.Context, string) (bool, error)   { return false, nil }
func (c *bridgeTestChain) ListTransfers(context.Context, string, uint32, string) (*adapter.TransferPage, error) {
	c.calls++
	return c.page, nil
}
func (c *bridgeTestChain) LookupTransaction(context.Context, string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	return nil, adapter.SourceStatus{}, nil
}
func (c *bridgeTestChain) GetContractMetadata(context.Context, string) (*adapter.ContractMetadata, error) {
	return nil, nil
}
func (c *bridgeTestChain) SourceStatus() adapter.SourceStatus { return adapter.SourceStatus{} }
