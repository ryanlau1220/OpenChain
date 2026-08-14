package adapter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPageStatusIsIncompleteWhenAPageHasMore(t *testing.T) {
	if PageStatus(SourceStatus{IsComplete: true}, true).IsComplete {
		t.Fatal("paged transfer history was marked complete")
	}
	if !PageStatus(SourceStatus{IsComplete: true}, false).IsComplete {
		t.Fatal("complete final page was changed")
	}
}

func TestWithEVMChainHeadRecordsTheConfirmationHeight(t *testing.T) {
	rpc := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(writer, `{"jsonrpc":"2.0","id":1,"result":"0x100"}`)
	}))
	defer rpc.Close()

	status := withEVMChainHead(context.Background(), SourceStatus{Source: "test"}, NewEVMClient(rpc.URL))
	if status.LatestChainBlock != 256 || status.Warning != "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestWithEVMChainHeadKeepsObservationsProvisionalWithoutAHeight(t *testing.T) {
	status := withEVMChainHead(context.Background(), SourceStatus{Source: "test"}, nil)
	if status.LatestChainBlock != 0 || status.Warning == "" {
		t.Fatalf("status = %#v", status)
	}
}

func TestAdaptersDeclareNetworkCapabilities(t *testing.T) {
	evm := NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, InternalTransfers: true, HistoricalPagination: true, Finality: true, EntityClassification: true, ExactRawProvenance: true}
	nativeOnly := NetworkCapabilities{NativeTransfers: true, HistoricalPagination: true, ExactRawProvenance: true}
	cases := []struct {
		name  string
		chain ChainAdapter
		want  NetworkCapabilities
	}{
		{"ethereum", NewEVMChainAdapter("ethereum-mainnet", "1", "https://api.example", "key", nil), evm},
		{"base", NewBlockscoutChainAdapter("base-mainnet", "https://api.example", "key", nil), evm},
		{"polygon", NewAlchemyEVMChainAdapter("polygon-mainnet", "https://api.example", "key", Asset{}, nil), evm},
		{"arbitrum", NewAlchemyEVMChainAdapter("arbitrum-one", "https://api.example", "key", Asset{}, nil), evm},
		{"optimism", NewAlchemyEVMChainAdapter("optimism-mainnet", "https://api.example", "key", Asset{}, nil), evm},
		{"bnb", NewAlchemyEVMChainAdapter("bnb-chain", "https://api.example", "key", Asset{}, nil), evm},
		{"solana", NewSolanaAdapter("solana-mainnet", "https://mainnet.helius-rpc.com/?api-key=key"), NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, HistoricalPagination: true, EntityClassification: true, ExactRawProvenance: true}},
		{"tron", NewTronAdapter("tron-mainnet", "https://api.example", "key"), NetworkCapabilities{NativeTransfers: true, TokenTransfers: true, InternalTransfers: true, HistoricalPagination: true, EntityClassification: true, ExactRawProvenance: true}},
		{"ton", NewTONAdapter("ton-mainnet", "key"), nativeOnly},
		{"cardano", NewCardanoAdapter("cardano-mainnet", "key"), nativeOnly},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if got := test.chain.Capabilities(); got != test.want {
				t.Fatalf("capabilities = %#v, want %#v", got, test.want)
			}
		})
	}
}
