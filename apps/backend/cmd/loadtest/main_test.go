package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestStubChainOnlyAcceptsNormalizedEVMValues(t *testing.T) {
	chain := &stubChain{}
	address := "0x" + strings.Repeat("A", 40)
	normalized, err := chain.NormalizeAddress(address)
	if err != nil || normalized != strings.ToLower(address) {
		t.Fatalf("NormalizeAddress() = %q, %v", normalized, err)
	}
	if _, err := chain.NormalizeAddress("0x1234"); err == nil {
		t.Fatal("NormalizeAddress accepted a malformed address")
	}
	hash := "0x" + strings.Repeat("B", 64)
	if normalized, err := chain.NormalizeTransactionHash(hash); err != nil || normalized != strings.ToLower(hash) {
		t.Fatalf("NormalizeTransactionHash() = %q, %v", normalized, err)
	}
}

func TestStubChainReturnsOneControlledTransfer(t *testing.T) {
	chain := &stubChain{delay: time.Millisecond}
	address := "0x" + strings.Repeat("0", 39) + "1"
	page, err := chain.ListTransfers(context.Background(), address, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Transfers) != 1 || page.Transfers[0].From != address || !page.SourceStatus.IsComplete {
		t.Fatalf("unexpected controlled transfer page: %#v", page)
	}
}
