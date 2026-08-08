package risk

import (
	"context"
	"testing"

	"github.com/openchain/openchain/apps/backend/internal/labels"
)

func TestRiskEvaluator(t *testing.T) {
	registry := labels.NewRegistry()
	evaluator := NewEvaluator(registry, nil)

	ctx := context.Background()

	// Test 1: Fresh address with zero transactions
	freshAddr := "0x9999999999999999999999999999999999999999"
	evalFresh := evaluator.EvaluateAddress(ctx, freshAddr, "ETHEREUM_SEPOLIA", false, 0)
	if evalFresh.TotalScore < 5.0 {
		t.Errorf("expected score >= 5.0 for fresh address, got %f", evalFresh.TotalScore)
	}
	if evalFresh.RiskLevel != "LOW" {
		t.Errorf("expected LOW risk level, got %s", evalFresh.RiskLevel)
	}

	// Test 2: Add Sanction label to address and evaluate high score
	sanctionAddr := "0x8888888888888888888888888888888888888888"
	_, _ = registry.AddLabel(ctx, labels.LabelItem{
		Address:     sanctionAddr,
		Network:     "ETHEREUM_SEPOLIA",
		Category:    "SANCTION",
		Label:       "Designated Entity",
		Confidence:  1.0,
		EvidenceURL: "https://ofac.treasury.gov",
	})

	evalSanction := evaluator.EvaluateAddress(ctx, sanctionAddr, "ETHEREUM_SEPOLIA", false, 5)
	if evalSanction.TotalScore < 90.0 {
		t.Errorf("expected score >= 90.0 for sanctioned address, got %f", evalSanction.TotalScore)
	}
	if evalSanction.RiskLevel != "SEVERE" {
		t.Errorf("expected SEVERE risk level, got %s", evalSanction.RiskLevel)
	}
	if len(evalSanction.Flags) == 0 {
		t.Errorf("expected triggered risk flags")
	}
}
