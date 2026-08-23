package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/config"
)

const acceptancePageSize = 1

type acceptanceCase struct {
	network, address, source string
	chain                    adapter.ChainAdapter
	capabilities             adapter.NetworkCapabilities
}

func main() {
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	for _, check := range acceptanceCases(cfg) {
		if err := runAcceptance(ctx, check); err != nil {
			log.Fatalf("%s acceptance failed: %v", check.network, err)
		}
	}
	log.Println("Provider acceptance checks passed.")
}

func acceptanceCases(cfg *config.Config) []acceptanceCase {
	evmCapabilities := adapter.NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, InternalTransfers: true, HistoricalPagination: true, Finality: true, EntityClassification: true, ExactRawProvenance: true}
	baseCapabilities := evmCapabilities
	baseCapabilities.BridgeEvidence = true
	optimismCapabilities := evmCapabilities
	optimismCapabilities.BridgeEvidence = true
	return []acceptanceCase{
		{"ethereum-mainnet", "0x0000000000000000000000000000000000000000", adapter.EtherscanSource, adapter.NewEVMChainAdapter("ethereum-mainnet", "1", adapter.EtherscanAPIURL, cfg.EtherscanAPIKey, adapter.NewEVMClient(cfg.EthereumMainnetRPCURL)), evmCapabilities},
		{"base-mainnet", "0x0000000000000000000000000000000000000000", adapter.BlockscoutSource, adapter.NewBlockscoutChainAdapter("base-mainnet", adapter.BlockscoutBaseAPIURL, cfg.BlockscoutAPIKey, adapter.NewEVMClient(cfg.BaseMainnetRPCURL)), baseCapabilities},
		{"polygon-mainnet", "0x0000000000000000000000000000000000000000", adapter.AlchemySource, adapter.NewAlchemyEVMChainAdapter("polygon-mainnet", "https://polygon-mainnet.g.alchemy.com/v2", cfg.AlchemyAPIKey, adapter.Asset{Kind: "NATIVE", Symbol: "POL", Decimals: 18}, adapter.NewEVMClient("https://polygon-mainnet.g.alchemy.com/v2/"+cfg.AlchemyAPIKey)), evmCapabilities},
		{"arbitrum-one", "0x0000000000000000000000000000000000000000", adapter.AlchemySource, adapter.NewAlchemyEVMChainAdapter("arbitrum-one", "https://arb-mainnet.g.alchemy.com/v2", cfg.AlchemyAPIKey, adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, adapter.NewEVMClient("https://arb-mainnet.g.alchemy.com/v2/"+cfg.AlchemyAPIKey)), evmCapabilities},
		{"optimism-mainnet", "0x0000000000000000000000000000000000000000", adapter.AlchemySource, adapter.NewAlchemyEVMChainAdapter("optimism-mainnet", "https://opt-mainnet.g.alchemy.com/v2", cfg.AlchemyAPIKey, adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, adapter.NewEVMClient("https://opt-mainnet.g.alchemy.com/v2/"+cfg.AlchemyAPIKey)), optimismCapabilities},
		{"bnb-chain", "0x0000000000000000000000000000000000000000", adapter.AlchemySource, adapter.NewAlchemyEVMChainAdapter("bnb-chain", "https://bnb-mainnet.g.alchemy.com/v2", cfg.AlchemyAPIKey, adapter.Asset{Kind: "NATIVE", Symbol: "BNB", Decimals: 18}, adapter.NewEVMClient("https://bnb-mainnet.g.alchemy.com/v2/"+cfg.AlchemyAPIKey)), evmCapabilities},
		{"solana-mainnet", "M2mx93ekt1fmXSVkTrUL9xVFHkmME8HTUi5Cyc5aF7K", adapter.HeliusHistorySource, adapter.NewSolanaAdapter("solana-mainnet", cfg.SolanaMainnetRPCURL), adapter.NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, HistoricalPagination: true, EntityClassification: true, ExactRawProvenance: true}},
		{"tron-mainnet", "T9yD14Nj9j7xAB4dbGeiX9h8unkKHxuWwb", adapter.TronGridSource, adapter.NewTronAdapter("tron-mainnet", adapter.TronGridAPIURL, cfg.TronGridAPIKey), adapter.NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, InternalTransfers: true, HistoricalPagination: true, EntityClassification: true, ExactRawProvenance: true}},
		{"ton-mainnet", "UQBKgXCNLPexWhs2L79kiARR1phGH1LwXxRbNsCFF9doczSI", "tonapi", adapter.NewTONAdapter("ton-mainnet", cfg.TonAPIKey), adapter.NetworkCapabilities{NativeTransfers: true, HistoricalPagination: true, ExactRawProvenance: true}},
		{"cardano-mainnet", "addr1q94afl3g9fddyh0awlt088sv30ymqel00h960utw8g5ket84lr0a9hul5cdpl9pyfpvdve6wrnd02wpgezrelt8yry3qvykn5y", "blockfrost", adapter.NewCardanoAdapter("cardano-mainnet", cfg.BlockfrostProjectID), adapter.NetworkCapabilities{NativeTransfers: true, HistoricalPagination: true, ExactRawProvenance: true}},
	}
}

func runAcceptance(ctx context.Context, check acceptanceCase) error {
	if check.chain.Capabilities() != check.capabilities {
		return fmt.Errorf("capabilities changed: got %+v", check.chain.Capabilities())
	}
	address, err := check.chain.NormalizeAddress(check.address)
	if err != nil {
		return fmt.Errorf("known address is invalid: %w", err)
	}
	page, err := check.chain.ListTransfers(ctx, address, acceptancePageSize, "")
	if err != nil {
		return err
	}
	if err := validatePage(page, check.source); err != nil {
		return err
	}
	if page.HasMore {
		next, err := check.chain.ListTransfers(ctx, address, acceptancePageSize, page.NextCursor)
		if err != nil {
			return fmt.Errorf("next page: %w", err)
		}
		if err := validatePage(next, check.source); err != nil {
			return fmt.Errorf("next page: %w", err)
		}
	}
	log.Printf("%s: source=%s transfers=%d paginated=%t", check.network, page.SourceStatus.Source, len(page.Transfers), page.HasMore)
	return nil
}

func validatePage(page *adapter.TransferPage, source string) error {
	if page == nil {
		return fmt.Errorf("provider returned no page")
	}
	if page.SourceStatus.Source != source || page.SourceStatus.RetrievedAt.IsZero() {
		return fmt.Errorf("invalid source status")
	}
	if page.HasMore != (page.NextCursor != "") {
		return fmt.Errorf("invalid pagination cursor")
	}
	if len(page.Transfers) == 0 {
		return fmt.Errorf("known address returned no transfer evidence")
	}
	for _, transfer := range page.Transfers {
		if transfer.Hash == "" || transfer.EventID == "" || transfer.From == "" || transfer.To == "" || transfer.AmountBaseUnits == "" || transfer.Asset.Kind == "" || transfer.Timestamp.IsZero() {
			return fmt.Errorf("transfer evidence is incomplete")
		}
	}
	return nil
}
