package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBlockscoutListsDirectBaseTransferFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case strings.HasSuffix(request.URL.Path, "/token-transfers"):
			_, _ = writer.Write([]byte(`{"items":[{"transaction_hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","from":{"hash":"0x1111111111111111111111111111111111111111"},"to":{"hash":"0x4444444444444444444444444444444444444444"},"token":{"address_hash":"0x5555555555555555555555555555555555555555","symbol":"USDC","decimals":"6"},"total":{"value":"1234567"},"token_type":"ERC-20","log_index":9,"block_number":12,"timestamp":"2026-08-10T00:00:00.000000Z"}]}`))
		case strings.HasSuffix(request.URL.Path, "/internal-transactions"):
			_, _ = writer.Write([]byte(`{"items":[{"transaction_hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","from":{"hash":"0x1111111111111111111111111111111111111111"},"to":{"hash":"0x3333333333333333333333333333333333333333"},"value":"3","index":1,"block_number":11,"timestamp":"2026-08-09T00:00:00.000000Z"}]}`))
		case strings.HasSuffix(request.URL.Path, "/transactions"):
			_, _ = writer.Write([]byte(`{"items":[{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":{"hash":"0x1111111111111111111111111111111111111111"},"to":{"hash":"0x2222222222222222222222222222222222222222"},"value":"42","block_number":10,"timestamp":"2026-08-08T00:00:00.000000Z"}]}`))
		default:
			t.Fatalf("unexpected request path: %s", request.URL.Path)
		}
	}))
	defer server.Close()

	page, err := NewBlockscoutChainAdapter("base-mainnet", server.URL, nil).ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transfers) != 3 || page.SourceStatus.Source != BlockscoutSource || !page.SourceStatus.IsComplete {
		t.Fatalf("page = %#v", page)
	}
	if page.Transfers[0].EventID != "log:9" || page.Transfers[0].Asset.Symbol != "USDC" || page.Transfers[1].EventID != "trace:1" || page.Transfers[2].AmountBaseUnits != "42" {
		t.Fatalf("transfers = %#v", page.Transfers)
	}
}

func TestBlockscoutCursorKeepsEachHistoryCategorySeparate(t *testing.T) {
	cursor, err := encodeBlockscoutCursor(blockscoutCursor{
		Transactions:         &blockscoutPageParameters{BlockNumber: 10, Index: 2, ItemsCount: 50},
		InternalTransactions: &blockscoutPageParameters{BlockNumber: 9, Index: 1, ItemsCount: 50},
	})
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeBlockscoutCursor(cursor)
	if err != nil || decoded.Transactions == nil || decoded.InternalTransactions == nil || decoded.Transactions.BlockNumber != 10 || decoded.InternalTransactions.BlockNumber != 9 {
		t.Fatalf("cursor = %#v, err = %v", decoded, err)
	}
}
