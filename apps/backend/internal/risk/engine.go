package risk

import (
	"context"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/labels"
)

type RiskFlag struct {
	RuleID         string  `json:"rule_id"`
	RuleName       string  `json:"rule_name"`
	Severity       string  `json:"severity"` // LOW, MEDIUM, HIGH, CRITICAL
	ScoreImpact    float64 `json:"score_impact"`
	Description    string  `json:"description"`
	EvidenceDetail string  `json:"evidence_detail"`
}

type RiskEvaluation struct {
	Address     string     `json:"address"`
	Network     string     `json:"network"`
	TotalScore  float64    `json:"total_score"` // 0.0 to 100.0
	RiskLevel   string     `json:"risk_level"`  // LOW, MEDIUM, HIGH, SEVERE
	Flags       []RiskFlag `json:"flags"`
	EvaluatedAt time.Time  `json:"evaluated_at"`
}

type Evaluator struct {
	labelRegistry *labels.Registry
}

func NewEvaluator(lr *labels.Registry) *Evaluator {
	return &Evaluator{
		labelRegistry: lr,
	}
}

func (e *Evaluator) EvaluateAddress(ctx context.Context, address string, network string, isContract bool, txCount uint64) *RiskEvaluation {
	var flags []RiskFlag
	var totalScore float64

	// Rule 1: Check known address labels (e.g. Sanction, Mixer, Hack)
	addressLabels := e.labelRegistry.GetLabels(ctx, address)
	for _, l := range addressLabels {
		cat := strings.ToUpper(l.Category)
		switch cat {
		case "SANCTION":
			flags = append(flags, RiskFlag{
				RuleID:         "R001",
				RuleName:       "Sanction List Entity",
				Severity:       "CRITICAL",
				ScoreImpact:    90.0,
				Description:    "Address is directly associated with a designated sanctioned entity.",
				EvidenceDetail: l.EvidenceURL,
			})
			totalScore += 90.0
		case "MIXER":
			flags = append(flags, RiskFlag{
				RuleID:         "R002",
				RuleName:       "Privacy Mixer Interaction",
				Severity:       "HIGH",
				ScoreImpact:    50.0,
				Description:    "Address identified as a liquidity pool or mixing contract.",
				EvidenceDetail: l.EvidenceURL,
			})
			totalScore += 50.0
		case "HACK":
			flags = append(flags, RiskFlag{
				RuleID:         "R003",
				RuleName:       "Exploit & Hack Associated",
				Severity:       "HIGH",
				ScoreImpact:    70.0,
				Description:    "Address flagged in historical exploit or security incident.",
				EvidenceDetail: l.EvidenceURL,
			})
			totalScore += 70.0
		}
	}

	// Rule 2: Contract with high transaction count or unverified creation
	if isContract && txCount > 1000 {
		flags = append(flags, RiskFlag{
			RuleID:         "R004",
			RuleName:       "High-Frequency Contract Interaction",
			Severity:       "MEDIUM",
			ScoreImpact:    15.0,
			Description:    "Smart contract exhibits unusually high volume execution.",
			EvidenceDetail: "Tx count exceeds 1,000 transactions.",
		})
		totalScore += 15.0
	}

	// Rule 3: Zero transaction fresh account
	if txCount == 0 {
		flags = append(flags, RiskFlag{
			RuleID:         "R005",
			RuleName:       "Unused / Fresh Address",
			Severity:       "LOW",
			ScoreImpact:    5.0,
			Description:    "Address has zero recorded transactions on the current testnet.",
			EvidenceDetail: "Freshly generated EOA or uninitialized state.",
		})
		totalScore += 5.0
	}

	if totalScore > 100.0 {
		totalScore = 100.0
	}

	riskLevel := "LOW"
	if totalScore >= 75.0 {
		riskLevel = "SEVERE"
	} else if totalScore >= 50.0 {
		riskLevel = "HIGH"
	} else if totalScore >= 25.0 {
		riskLevel = "MEDIUM"
	}

	return &RiskEvaluation{
		Address:     address,
		Network:     network,
		TotalScore:  totalScore,
		RiskLevel:   riskLevel,
		Flags:       flags,
		EvaluatedAt: time.Now(),
	}
}
