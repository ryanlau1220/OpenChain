package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTronListsConfirmedNativeTransfersWithServerOnlyKey(t *testing.T) {
	from, err := tronAddressFromHex("410000000000000000000000000000000000000001")
	if err != nil {
		t.Fatal(err)
	}
	to, err := tronAddressFromHex("410000000000000000000000000000000000000002")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("TRON-PRO-API-KEY") != "test-key" {
			t.Fatalf("missing server API key")
		}
		switch {
		case request.URL.Path == "/wallet/getaccount":
			_, _ = writer.Write([]byte(`{"balance":1234567}`))
		case strings.HasSuffix(request.URL.Path, "/transactions"):
			if request.URL.Query().Get("only_confirmed") != "true" {
				t.Fatalf("transactions query = %s", request.URL.RawQuery)
			}
			_, _ = writer.Write([]byte(`{"data":[{"txID":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":12,"raw_data":{"timestamp":1000,"contract":[{"type":"TransferContract","parameter":{"value":{"owner_address":"410000000000000000000000000000000000000001","to_address":"410000000000000000000000000000000000000002","amount":42}}}]}}],"meta":{"fingerprint":"next-page"}}`))
		case strings.HasSuffix(request.URL.Path, "/transactions/trc20"):
			_, _ = writer.Write([]byte(`{"data":[{"transaction_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","block_timestamp":2000,"from":"` + from + `","to":"` + to + `","value":"1234567","token_info":{"address":"TR7NHqjeKQxGTCi8q8ZY4pL8otSzgjLj6t","symbol":"USDT","decimals":6}}],"meta":{"fingerprint":"token-page"}}`))
		case strings.HasSuffix(request.URL.Path, "/internal-transactions"):
			_, _ = writer.Write([]byte(`{"data":[{"transaction_id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","block_number":13,"block_timestamp":1500,"caller_address":"` + from + `","transferTo_address":"` + to + `","callValueInfo":[{"callValue":7}]}],"meta":{"fingerprint":"internal-page"}}`))
		default:
			t.Fatalf("unexpected request %s", request.URL.Path)
		}
	}))
	defer server.Close()

	adapter := NewTronAdapter("tron-mainnet", server.URL, "test-key")
	balance, err := adapter.GetBalance(context.Background(), from)
	if err != nil || balance.String() != "1234567" {
		t.Fatalf("balance = %v, %v", balance, err)
	}
	page, err := adapter.ListTransfers(context.Background(), from, 25, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transfers) != 3 || page.NextCursor == "" || !page.SourceStatus.IsComplete {
		t.Fatalf("page = %#v", page)
	}
	items := map[string]TransferItem{}
	for _, item := range page.Transfers {
		items[item.TransferKind] = item
	}
	if items["NATIVE"].From != from || items["NATIVE"].To != to || items["NATIVE"].AmountBaseUnits != "42" || items["TRC20"].Asset.Symbol != "USDT" || items["INTERNAL"].AmountBaseUnits != "7" {
		t.Fatalf("transfers = %#v", page.Transfers)
	}
	if normalized, err := adapter.NormalizeAddress(from); err != nil || normalized != from {
		t.Fatalf("normalized address = %q, %v", normalized, err)
	}
	if _, err := adapter.NormalizeAddress(from[:len(from)-1] + "1"); err == nil {
		t.Fatal("invalid checksum was accepted")
	}
}
