package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestTrueBlocksListsBoundedTransactions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/status" {
			_, _ = writer.Write([]byte(`{"data":[{}]}`))
			return
		}
		if request.URL.Path != "/export" || request.URL.Query().Get("addrs") != "0x1111111111111111111111111111111111111111" || request.URL.Query().Get("maxRecords") != "3" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		_, _ = writer.Write([]byte(`{"data":[{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":10,"timestamp":100,"from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"42","isError":false},{"hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","blockNumber":9,"timestamp":90,"from":"0x2222222222222222222222222222222222222222","to":"0x1111111111111111111111111111111111111111","value":"7","isError":false},{"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","blockNumber":8,"timestamp":80,"from":"0x3333333333333333333333333333333333333333","to":"0x1111111111111111111111111111111111111111","value":"1","isError":false}]}`))
	}))
	defer server.Close()
	client, err := NewTrueBlocksClient(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.ListNativeTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 2, 0, 11)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transactions) != 2 || !page.HasMore || page.NextCursor != "2" {
		t.Fatalf("page = %#v", page)
	}
	if page.Transactions[0].ValueWei != "42" || page.Transactions[0].AssetSymbol != "ETH" {
		t.Fatalf("transaction = %#v", page.Transactions[0])
	}
}

func TestTrueBlocksIntegration(t *testing.T) {
	if os.Getenv("TRUEBLOCKS_INTEGRATION_TEST") != "1" {
		t.Skip("set TRUEBLOCKS_INTEGRATION_TEST=1 to test a real TrueBlocks instance")
	}
	endpoint := os.Getenv("TRUEBLOCKS_API_URL")
	if endpoint == "" {
		t.Skip("set TRUEBLOCKS_API_URL to test a real internal TrueBlocks instance")
	}
	client, err := NewTrueBlocksClient(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := client.ListNativeTransfers(ctx, "0xd8da6bf26964af9d7eed9e03e53415d37aa96045", 1, 0, 0); err != nil {
		t.Fatal(err)
	}
}
