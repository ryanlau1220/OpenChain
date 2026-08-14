package adapter

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestEtherscanListsBoundedTransferFacts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			_, _ = writer.Write([]byte(`{"jsonrpc":"2.0","id":1,"result":{"logs":[{"address":"0x5555555555555555555555555555555555555555","topics":["0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef","0x0000000000000000000000001111111111111111111111111111111111111111","0x0000000000000000000000004444444444444444444444444444444444444444"],"data":"0x000000000000000000000000000000000000000000000000000000000012d687","logIndex":"0x9"}]}}`))
			return
		}
		query := request.URL.Query()
		if query.Get("module") != "account" || query.Get("chainid") != "8453" || query.Get("address") != "0x1111111111111111111111111111111111111111" || query.Get("page") != "1" || query.Get("offset") != "2" || query.Get("apikey") != "test-key" {
			t.Fatalf("unexpected request: %s", request.URL.String())
		}
		switch query.Get("action") {
		case "txlist":
			_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":"10","timeStamp":"100","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"42"},{"hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","blockNumber":"9","timeStamp":"90","from":"0x2222222222222222222222222222222222222222","to":"0x1111111111111111111111111111111111111111","value":"7"}]}`))
		case "txlistinternal":
			_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","blockNumber":"11","timeStamp":"110","from":"0x1111111111111111111111111111111111111111","to":"0x3333333333333333333333333333333333333333","value":"3","traceId":"0_1"}]}`))
		case "tokentx":
			_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xdddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd","blockNumber":"12","timeStamp":"120","from":"0x1111111111111111111111111111111111111111","to":"0x4444444444444444444444444444444444444444","value":"1234567","contractAddress":"0x5555555555555555555555555555555555555555","tokenSymbol":"TKN","tokenDecimal":"6"}]}`))
		default:
			t.Fatalf("unexpected action: %s", query.Get("action"))
		}
	}))
	defer server.Close()
	client := NewEVMChainAdapter("base-mainnet", "8453", server.URL, "test-key", NewEVMClient(server.URL))
	page, err := client.ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 3, "")
	if err != nil {
		t.Fatal(err)
	}
	if client.Network() != "base-mainnet" || len(page.Transfers) != 3 || !page.HasMore || page.NextCursor != "1" || page.SourceStatus.Source != EtherscanSource || page.SourceStatus.IsComplete {
		t.Fatalf("page = %#v", page)
	}
	if page.Transfers[0].TransferKind != "ERC20" || page.Transfers[0].EventID != "log:0x9" || page.Transfers[0].Asset.ContractAddress != "0x5555555555555555555555555555555555555555" || page.Transfers[0].Asset.Decimals != 6 || page.Transfers[1].EventID != "trace:0_1" || page.Transfers[2].AmountBaseUnits != "42" {
		t.Fatalf("transfers = %#v", page.Transfers)
	}
}

func TestEtherscanRejectsProviderErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = writer.Write([]byte(`{"status":"0","message":"Max rate limit reached","result":"[]"}`))
	}))
	defer server.Close()
	_, err := NewEVMChainAdapter("ethereum-mainnet", "1", server.URL, "test-key", nil).ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 1, "")
	if err == nil {
		t.Fatal("expected provider error")
	}
}

func TestEtherscanKeepsSameTransactionTokenEventsDistinct(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("action") != "tokentx" {
			_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[]}`))
			return
		}
		_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":"10","timeStamp":"100","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"1","logIndex":"3","contractAddress":"0x3333333333333333333333333333333333333333","tokenSymbol":"TOK","tokenDecimal":"18"},{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":"10","timeStamp":"100","from":"0x1111111111111111111111111111111111111111","to":"0x2222222222222222222222222222222222222222","value":"2","logIndex":"4","contractAddress":"0x3333333333333333333333333333333333333333","tokenSymbol":"TOK","tokenDecimal":"18"}]}`))
	}))
	defer server.Close()
	page, err := NewEVMChainAdapter("ethereum-mainnet", "1", server.URL, "test-key", nil).ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 6, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transfers) != 2 || page.Transfers[0].EventID == page.Transfers[1].EventID {
		t.Fatalf("token events were merged: %#v", page.Transfers)
	}
}

func TestMatchingTransferLogsOrdersHexIndexesNumerically(t *testing.T) {
	item := etherscanTxResult{ContractAddress: "0x3333333333333333333333333333333333333333", From: "0x1111111111111111111111111111111111111111", To: "0x2222222222222222222222222222222222222222", Value: "1"}
	logs := []LogItem{
		{Address: item.ContractAddress, Topics: []string{TransferEventTopic, "0x0000000000000000000000001111111111111111111111111111111111111111", "0x0000000000000000000000002222222222222222222222222222222222222222"}, Data: "0x1", LogIndex: "0x10"},
		{Address: item.ContractAddress, Topics: []string{TransferEventTopic, "0x0000000000000000000000001111111111111111111111111111111111111111", "0x0000000000000000000000002222222222222222222222222222222222222222"}, Data: "0x1", LogIndex: "0x9"},
	}
	matched := matchingTransferLogs(logs, item)
	if len(matched) != 2 || matched[0].LogIndex != "0x9" {
		t.Fatalf("matched logs = %#v", matched)
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
	transaction, source, err := NewEVMChainAdapter("ethereum-mainnet", "1", server.URL, "test-key", nil).LookupTransaction(context.Background(), "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if transaction.ValueBaseUnits != "42" || transaction.BlockNumber != 10 || source.Source != EtherscanSource {
		t.Fatalf("transaction = %#v, source = %#v", transaction, source)
	}
}

func TestEtherscanLimitsProviderCalls(t *testing.T) {
	requests := make(chan time.Time, 6)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests <- time.Now()
		_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[]}`))
	}))
	defer server.Close()
	client := NewEVMChainAdapter("ethereum-mainnet", "1", server.URL, "test-key", nil)
	for range 2 {
		if _, err := client.ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 1, ""); err != nil {
			t.Fatal(err)
		}
	}
	first, second := <-requests, <-requests
	if second.Sub(first) < etherscanRequestGap-20*time.Millisecond {
		t.Fatalf("provider calls were only %s apart", second.Sub(first))
	}
	health := client.ProviderHealth()[0]
	if health.MaxConcurrent != 1 || health.RequestsPerSecond != 5 || health.Requests < 6 || health.Throttled == 0 {
		t.Fatalf("provider health = %#v", health)
	}
}

func TestEtherscanTransportErrorDoesNotExposeAPIKey(t *testing.T) {
	client := NewEVMChainAdapter("ethereum-mainnet", "1", "https://api.example", "secret-api-key", nil)
	client.httpClient = &http.Client{Transport: roundTripperFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New(request.URL.String())
	})}
	_, err := client.ListTransfers(context.Background(), "0x1111111111111111111111111111111111111111", 1, "")
	if err == nil || strings.Contains(err.Error(), "secret-api-key") || strings.Contains(err.Error(), "api.example") {
		t.Fatalf("transport error leaked request details: %v", err)
	}
}
