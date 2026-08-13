package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTONNormalizersRejectMalformedValues(t *testing.T) {
	adapter := NewTONAdapter("ton-mainnet", "test-key")
	if _, err := adapter.NormalizeAddress("EQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM9c"); err != nil {
		t.Fatalf("valid TON address: %v", err)
	}
	if _, err := adapter.NormalizeAddress("EQnot-a-valid-address"); err == nil {
		t.Fatal("accepted malformed TON address")
	}
	if _, err := adapter.NormalizeAddress("EQAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAM9d"); err == nil {
		t.Fatal("accepted TON address with an invalid checksum")
	}
	if _, err := adapter.NormalizeTransactionHash(strings.Repeat("g", 64)); err == nil {
		t.Fatal("accepted malformed TON transaction hash")
	}
}

func TestCardanoListsUTXOTransferEvidence(t *testing.T) {
	const address = "addr1qabcdefghijklmnopqrstuvxyz023456789abcdefghijklmnopqrstuvxyz023456789"
	const counterpart = "addr1qbbcdefghjklmnpqrstuvwxyz023456789bcdefghjklmnpqrstuvwxyz023456789"
	const hash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("project_id") != "test-key" {
			t.Fatal("missing Blockfrost project id")
		}
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/addresses/" + address + "/transactions":
			_, _ = writer.Write([]byte(`[{"tx_hash":"` + hash + `","block_height":42,"block_time":1700000000}]`))
		case "/txs/" + hash + "/utxos":
			_, _ = writer.Write([]byte(`{"inputs":[{"address":"` + counterpart + `","amount":[{"unit":"lovelace","quantity":"1200000"}]},{"address":"` + address + `","amount":[{"unit":"lovelace","quantity":"500000"}]}],"outputs":[{"address":"` + counterpart + `","amount":[{"unit":"lovelace","quantity":"700000"}]},{"address":"` + address + `","amount":[{"unit":"lovelace","quantity":"900000"}]}]}`))
		default:
			t.Fatalf("unexpected path: %s", request.URL.Path)
		}
	}))
	defer server.Close()
	adapter := NewCardanoAdapter("cardano-mainnet", "test-key")
	adapter.apiURL = server.URL
	page, err := adapter.ListTransfers(context.Background(), address, 5, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transfers) != 2 {
		t.Fatalf("transfers = %#v", page.Transfers)
	}
	if page.Transfers[0].Asset.Symbol != "ADA" || page.Transfers[0].AmountBaseUnits != "1200000" || page.Transfers[0].To != address {
		t.Fatalf("inbound transfer = %#v", page.Transfers[0])
	}
	if page.Transfers[1].Asset.Symbol != "ADA" || page.Transfers[1].AmountBaseUnits != "700000" || page.Transfers[1].From != address {
		t.Fatalf("outbound transfer = %#v", page.Transfers[1])
	}
	if page.SourceStatus.Warning == "" {
		t.Fatal("Cardano UTXO attribution warning is missing")
	}
}

func TestCardanoNormalizersRejectMalformedValues(t *testing.T) {
	adapter := NewCardanoAdapter("cardano-mainnet", "test-key")
	if _, err := adapter.NormalizeAddress("addr1invalid!"); err == nil {
		t.Fatal("accepted malformed Cardano address")
	}
	if _, err := adapter.NormalizeTransactionHash(strings.Repeat("z", 64)); err == nil {
		t.Fatal("accepted malformed Cardano transaction hash")
	}
}
