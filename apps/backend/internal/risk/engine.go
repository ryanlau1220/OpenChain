package risk

import (

	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
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

type evalRequest struct {
	Address string
	CaseID  string
}

type Evaluator struct {
	labelRegistry *labels.Registry
	DB            *db.DB
	queueMu       sync.Mutex
	queue         map[string]evalRequest // address -> evalRequest
}

func NewEvaluator(lr *labels.Registry, database *db.DB) *Evaluator {
	return &Evaluator{
		labelRegistry: lr,
		DB:            database,
		queue:         make(map[string]evalRequest),
	}
}

// EnqueueAddress adds an address to the debounced evaluation queue
func (e *Evaluator) EnqueueAddress(address string, caseID string) {
	if address == "" {
		return
	}
	e.queueMu.Lock()
	defer e.queueMu.Unlock()
	e.queue[strings.ToLower(address)] = evalRequest{
		Address: strings.ToLower(address),
		CaseID:  caseID,
	}
}

// StartDebounceWorker processes enqueued addresses asynchronously every 2-3 seconds
func (e *Evaluator) StartDebounceWorker(ctx context.Context, interval time.Duration, callback func(caseID string, eval *RiskEvaluation)) {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.processQueue(ctx, callback)
		}
	}
}

func (e *Evaluator) processQueue(ctx context.Context, callback func(caseID string, eval *RiskEvaluation)) {
	e.queueMu.Lock()
	if len(e.queue) == 0 {
		e.queueMu.Unlock()
		return
	}
	pending := e.queue
	e.queue = make(map[string]evalRequest)
	e.queueMu.Unlock()

	for _, req := range pending {
		eval := e.EvaluateAddress(ctx, req.Address, "ETHEREUM_SEPOLIA", false, 1)
		if callback != nil && req.CaseID != "" {
			callback(req.CaseID, eval)
		}
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

	// Rule 3: Zero transaction fresh account (contracts have no nonce-based
	// activity signal, so only flag EOAs)
	if txCount == 0 && !isContract {
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

	// Rule 4: Bounded Cypher hop-distance path evaluation to Tier 1 Sanctioned vertex (max 5 hops)
	if e.DB != nil {
		ag, err := e.DB.ConnectAge()
		if err == nil {
			cypher := fmt.Sprintf(`
				MATCH p=(w:Wallet {address: '%s'})-[:TRANSFER*0..5]->(target:Wallet)-[hl:HAS_LABEL]->(l:Label)
				WHERE hl.trust_tier = 1
				RETURN target, l LIMIT 50
			`, strings.ToLower(address))

			tx, err := ag.Begin()
			if err == nil {
				cursor, err := tx.ExecCypher(0, "%s", cypher)
				if err == nil {
					foundTier1 := false
					for cursor.Next() {
						foundTier1 = true
					}
					if foundTier1 {
						flags = append(flags, RiskFlag{
							RuleID:         "R006",
							RuleName:       "Proximity to Tier 1 Authoritative Sanctioned Entity",
							Severity:       "CRITICAL",
							ScoreImpact:    85.0,
							Description:    "Address is within 5 transfer hops of a Tier 1 Authoritative sanctioned vertex.",
							EvidenceDetail: "OpenChain Graph Traversal (*0..5 hops)",
						})
						totalScore += 85.0
					}
				}
				_ = tx.Rollback()
			}
		}
	} else {
		slog.Debug("DB pool unattached, skipping Cypher graph path evaluation")
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
