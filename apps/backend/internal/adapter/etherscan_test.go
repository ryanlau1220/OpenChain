package adapter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEtherscanListsBoundedTransactions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("module") != "account" || query.Get("action") != "txlist" || query.Get("chainid") != "1" || query.Get("address") != "0x1111111111111111111111111111111111111111" || query.Get("page") != "1" || query.Get("offset") != "3" || query.Get("apikey") != "test-key" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":"10","timeStamp":"100","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"42"},{"hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","blockNumber":"9","timeStamp":"90","from":"0x2222222222222222222222222222222222222222","to":"0x1111111111111111111111111111111111111111","value":"7"},{"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","blockNumber":"8","timeStamp":"80","from":"0x3333333333333333333333333333333333333333","to":"0x1111111111111111111111111111111111111111","value":"1"}]}`))
	}))
	defer server.Close()
	client := NewEVMChainAdapter(server.URL, "test-key", nil)
	page, err := client.ListNativeTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transactions) != 2 || !page.HasMore || page.NextCursor != "1" || page.SourceStatus.Source != EtherscanSource || !page.SourceStatus.IsComplete {
		t.Fatalf("page = %#v", page)
	}
	if page.Transactions[0].ValueWei != "42" || page.Transactions[0].AssetSymbol != "ETH" {
		t.Fatalf("transaction = %#v", page.Transactions[0])
	}
}

func TestEtherscanRejectsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"status":"0","message":"Max rate limit reached","result":"[]"}`))
	}))
	defer server.Close()
	_, err := NewEVMChainAdapter(server.URL, "test-key", nil).ListNativeTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 1, 0)
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestEtherscanLooksUpTransaction(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("action") != "eth_getTransactionByHash" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","result":{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"0x2a","blockNumber":"0xa"}}`))
	}))
	defer server.Close()
	transaction, source, err := NewEVMChainAdapter(server.URL, "test-key", nil).LookupTransaction(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if transaction.ValueWei != "42" || transaction.BlockNumber != 10 || source.Source != EtherscanSource {
		t.Fatalf("transaction = %#v, source = %#v", transaction, source)
	}
}

func TestEtherscanLimitsProviderCalls(t *testing.T) {
	requests := make(chan time.Time, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- time.Now()
		_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[]}`))
	}))
	defer server.Close()
	client := NewEVMChainAdapter(server.URL, "test-key", nil)
	for range 2 {
		if _, err := client.ListNativeTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 1, 0); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-requests, <-requests
	if second.Sub(first) < etherscanRequestGap-20*time.Millisecond {
		t.Fatalf("provider calls were only %s apart", second.Sub(first))
	}
}
