package adapter

import (
	"context"
	"testing"
)

func TestEVMChainAdapter(t *testing.T) {
	evmClient := NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	chainAdapter := NewEVMChainAdapter("ETHEREUM_SEPOLIA", "", "", evmClient)

	if chainAdapter.Network() != "ETHEREUM_SEPOLIA" {
		t.Errorf("expected network ETHEREUM_SEPOLIA, got %s", chainAdapter.Network())
	}

	ctx := context.Background()
	testAddr := "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D"

	txs, err := chainAdapter.GetAccountTransactions(ctx, testAddr, 5)
	if err != nil {
		t.Fatalf("unexpected error fetching account transactions: %v", err)
	}

	// Verify adapter return structure
	if len(txs) > 0 {
		if txs[0].Hash == "" || txs[0].From == "" || txs[0].To == "" {
			t.Errorf("invalid transaction item returned: %+v", txs[0])
		}
	}

	meta, err := chainAdapter.GetContractMetadata(ctx, testAddr)
	if err != nil {
		t.Fatalf("unexpected error fetching contract metadata: %v", err)
	}
	if meta == nil {
		t.Errorf("expected non-nil contract metadata")
	}
}
