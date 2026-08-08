package labels

import (
	"context"
	"testing"
)

func TestLabelRegistry(t *testing.T) {
	registry := NewRegistry()
	ctx := context.Background()

	// Verify well-known seed labels
	uniswapAddr := "0xee567fe1712faf6149d80da1e6934e354124cfe3"
	lbls := registry.GetLabels(ctx, uniswapAddr)
	if len(lbls) == 0 {
		t.Fatalf("expected seeded label for Uniswap V2 Router")
	}
	if lbls[0].Label != "Uniswap V2 Router" {
		t.Errorf("expected Uniswap V2 Router, got %s", lbls[0].Label)
	}

	// Add new label
	newLabel := LabelItem{
		Address:     "0x1234567890123456789012345678901234567890",
		Network:     "ETHEREUM_SEPOLIA",
		Category:    "Mixer",
		Label:       "Test Privacy Pool",
		Confidence:  0.95,
		EvidenceURL: "https://sepolia.etherscan.io/address/0x1234",
		Source:      "Analyst Test",
	}

	added, err := registry.AddLabel(ctx, newLabel)
	if err != nil {
		t.Fatalf("failed to add label: %v", err)
	}

	if added.ID == "" {
		t.Errorf("expected generated ID for added label")
	}

	// Retrieve added label
	retrieved := registry.GetLabels(ctx, "0x1234567890123456789012345678901234567890")
	if len(retrieved) != 1 {
		t.Fatalf("expected 1 label retrieved, got %d", len(retrieved))
	}
	if retrieved[0].Label != "Test Privacy Pool" {
		t.Errorf("expected Test Privacy Pool, got %s", retrieved[0].Label)
	}

	// Search labels
	searchResults := registry.SearchLabels(ctx, "Privacy", "Mixer", 10)
	if len(searchResults) == 0 {
		t.Errorf("expected search results for Mixer category")
	}
}
