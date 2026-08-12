package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func solanaTestKey(last byte, length int) string {
	value := make([]byte, length)
	value[len(value)-1] = last
	return encodeBase58(value)
}

func TestSolanaListsBoundedNativeAndSPLTransfersWithoutSkippingEvents(t *testing.T) {
	address := solanaTestKey(1, 32)
	destination := solanaTestKey(2, 32)
	signature := solanaTestKey(3, 64)
	mint := solanaTestKey(4, 32)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("api-key") != "test-key" {
			t.Fatalf("missing history API key")
		}
		switch request.URL.Path {
		case "/v0/addresses/" + address + "/transactions":
			if request.URL.Query().Get("before-signature") != "" {
				_, _ = writer.Write([]byte(`[]`))
				return
			}
			_, _ = writer.Write([]byte(`[{"signature":"` + signature + `","slot":9,"timestamp":100,"nativeTransfers":[{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","amount":42},{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","amount":"7"}],"tokenTransfers":[{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","mint":"` + mint + `","tokenAmount":"1000000"},{"fromUserAccount":"","toUserAccount":"","mint":"","tokenAmount":null}],"accountData":[{"tokenBalanceChanges":[{"mint":"` + mint + `","rawTokenAmount":{"decimals":6}}]}]}]`))
		case "/v0/transactions":
			var body struct {
				Transactions []string `json:"transactions"`
			}
			if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if len(body.Transactions) != 1 || body.Transactions[0] != signature {
				t.Fatalf("history lookup = %#v", body)
			}
			_, _ = writer.Write([]byte(`[{"signature":"` + signature + `","slot":9,"timestamp":100,"nativeTransfers":[{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","amount":42},{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","amount":"7"}],"tokenTransfers":[{"fromUserAccount":"` + address + `","toUserAccount":"` + destination + `","mint":"` + mint + `","tokenAmount":"1000000"},{"fromUserAccount":"","toUserAccount":"","mint":"","tokenAmount":null}],"accountData":[{"tokenBalanceChanges":[{"mint":"` + mint + `","rawTokenAmount":{"decimals":6}}]}]}]`))
		default:
			t.Fatalf("unexpected path %q", request.URL.Path)
		}
	}))
	defer server.Close()

	adapter := NewSolanaAdapter("solana-mainnet", server.URL)
	adapter.historyURL = server.URL
	adapter.historyKey = "test-key"
	first, err := adapter.ListTransfers(context.Background(), address, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Transfers) != 2 || first.Transfers[0].EventID != "native:0" || first.Transfers[1].EventID != "native:1" || !first.HasMore {
		t.Fatalf("first page = %#v", first)
	}
	second, err := adapter.ListTransfers(context.Background(), address, 2, first.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Transfers) != 1 || second.Transfers[0].EventID != "spl:0" || second.Transfers[0].Asset.Kind != "SPL" || second.Transfers[0].Asset.ContractAddress != mint || second.Transfers[0].Asset.Decimals != 6 {
		t.Fatalf("second page = %#v", second)
	}
	if !second.SourceStatus.IsComplete || second.SourceStatus.Warning != "" {
		t.Fatalf("source status = %#v", second.SourceStatus)
	}
}

func TestHeliusHistoryConfigRequiresHeliusKey(t *testing.T) {
	endpoint, key := heliusHistoryConfig("https://mainnet.helius-rpc.com/?api-key=secret")
	if endpoint != "https://api-mainnet.helius-rpc.com" || key != "secret" {
		t.Fatalf("Helius config = %q, %q", endpoint, key)
	}
	endpoint, key = heliusHistoryConfig("https://api.mainnet-beta.solana.com")
	if endpoint != "" || key != "" {
		t.Fatalf("unexpected non-Helius config = %q, %q", endpoint, key)
	}
}
