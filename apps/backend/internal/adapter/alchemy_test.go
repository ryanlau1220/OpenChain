package adapter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAlchemyListsBoundedNativeAndTokenTransfers(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if !strings.HasSuffix(request.URL.Path, "/test-key") {
			t.Fatalf("request path = %q", request.URL.Path)
		}
		var payload RPCRequest
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		filters := payload.Params[0].(map[string]any)
		outbound := filters["fromAddress"] != nil
		writer.Header().Set("Content-Type", "application/json")
		if outbound {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"transfers":[{"blockNum":"0x2","hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","category":"erc20","uniqueId":"0xbb:log:0x1","asset":"USDC","rawContract":{"address":"0x3333333333333333333333333333333333333333","value":"0xf4240","decimal":"6"},"metadata":{"blockTimestamp":"2026-01-02T03:04:05.000Z"}}]}}`))
			return
		}
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"transfers":[{"blockNum":"0x3","hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"0x4444444444444444444444444444444444444444","to":"0x1111111111111111111111111111111111111111","category":"external","uniqueId":"0xaa:external","asset":"MATIC","value":1.25,"metadata":{"blockTimestamp":"2026-01-03T03:04:05.000Z"}}]}}`))
	}))
	defer server.Close()
	client := NewAlchemyEVMChainAdapter("polygon-mainnet", server.URL, "test-key", Asset{Kind: "NATIVE", Symbol: "POL", Decimals: 18}, nil)
	page, err := client.ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 2, "")
	if err != nil {
		t.Fatal(err)
	}
	if requests != 2 || len(page.Transfers) != 2 {
		t.Fatalf("requests=%d transfers=%#v", requests, page.Transfers)
	}
	if page.Transfers[0].Asset.Symbol != "POL" || page.Transfers[0].AmountBaseUnits != "1250000000000000000" {
		t.Fatalf("native transfer = %#v", page.Transfers[0])
	}
	if page.Transfers[1].Asset.Symbol != "USDC" || page.Transfers[1].AmountBaseUnits != "1000000" || page.Transfers[1].TransferKind != "ERC20" {
		t.Fatalf("token transfer = %#v", page.Transfers[1])
	}
}

func TestAlchemyRejectsMalformedCursor(t *testing.T) {
	client := NewAlchemyEVMChainAdapter("polygon-mainnet", "https://example.test", "test-key", Asset{Kind: "NATIVE", Symbol: "POL", Decimals: 18}, nil)
	if _, err := client.ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 2, "invalid"); err == nil {
		t.Fatal("expected invalid cursor")
	}
}
