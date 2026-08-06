package adapter

import (
	"math/big"
	"testing"
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

func TestNewEVMClient(t *testing.T) {
	client := NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	if client == nil {
		t.Fatalf("expected non-nil client")
	}
	if client.rpcURL != "https://ethereum-sepolia-rpc.publicnode.com" {
		t.Errorf("expected rpc URL set, got %s", client.rpcURL)
	}
}
