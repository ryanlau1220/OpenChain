package adapter

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestFormatWeiToETH(t *testing.T) {
	oneETH, _ := new(big.Int).SetString("1000000000000000000", 10)
	twoPointFiveETH, _ := new(big.Int).SetString("2500000000000000000", 10)

	tests := []struct {
		name     string
		wei      *big.Int
		expected string
	}{
		{
			name:     "Nil Wei",
			wei:      nil,
			expected: "0.0 ETH",
		},
		{
			name:     "Zero Wei",
			wei:      big.NewInt(0),
			expected: "0.0000 ETH",
		},
		{
			name:     "One ETH (1e18 Wei)",
			wei:      oneETH,
			expected: "1.0000 ETH",
		},
		{
			name:     "2.5 ETH",
			wei:      twoPointFiveETH,
			expected: "2.5000 ETH",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatWeiToETH(tt.wei)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestRPCRequestsUseSharedRateBudget(t *testing.T) {
	var mu sync.Mutex
	var calls []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		calls = append(calls, time.Now())
		mu.Unlock()
		_ = json.NewEncoder(writer).Encode(RPCResponse{JSONRPC: "2.0", ID: 1, Result: json.RawMessage(`"0x0"`)})
	}))
	defer server.Close()
	client := NewEVMClient(server.URL)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, _ = client.GetBalance(context.Background(), "0x1000000000000000000000000000000000000001")
		}()
	}
	wait.Wait()
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 || calls[1].Sub(calls[0]) < rpcRequestGap-time.Millisecond*10 {
		t.Fatalf("calls = %#v", calls)
	}
}

func TestRPCRejectsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()
	if _, err := NewEVMClient(server.URL).GetBalance(context.Background(), "0x1000000000000000000000000000000000000001"); err == nil {
		t.Fatal("HTTP provider failure was accepted")
	}
}

func TestNewEVMClient(t *testing.T) {
	client := NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	if client == nil {
		t.Fatalf("expected non-nil client")
	}
	if client.rpcURL != "https://ethereum-sepolia-rpc.publicnode.com" {
		t.Errorf("expected rpc URL set, got %s", client.rpcURL)
	}
}
