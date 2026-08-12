package rules

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
)

const (
	alice = "0x0000000000000000000000000000000000000001"
	bob   = "0x0000000000000000000000000000000000000002"
	carol = "0x0000000000000000000000000000000000000003"
	dave  = "0x0000000000000000000000000000000000000004"
	hub   = "0x00000000000000000000000000000000000000aa"
)

func TestEvaluateFindsFanInAndRecordsVersionedRuns(t *testing.T) {
	transfers := []db.Transfer{
		ruleTransfer("fan-in-1", alice, hub, 0),
		ruleTransfer("fan-in-2", bob, hub, time.Hour),
		ruleTransfer("fan-in-3", carol, hub, 2*time.Hour),
	}
	leads, runs := Evaluate("ethereum-mainnet", transfers, time.Unix(100, 0).UTC())
	lead := findLead(t, leads, "fan-in-consolidation")
	if lead.SubjectAddress != hub || !reflect.DeepEqual(lead.TransferIDs, []string{"fan-in-1", "fan-in-2", "fan-in-3"}) {
		t.Fatalf("fan-in lead = %#v", lead)
	}
	if len(runs) != len(Catalog()) || runs[0].RuleVersion != "1.0.0" || !reflect.DeepEqual(runs[0].InputTransferIDs, []string{"fan-in-1", "fan-in-2", "fan-in-3"}) {
		t.Fatalf("rule runs = %#v", runs)
	}
	if !json.Valid(runs[0].Result) || !json.Valid(runs[0].Parameters) || runs[0].StartedAt != runs[0].CompletedAt {
		t.Fatalf("recorded run is not reproducible: %#v", runs[0])
	}
}

func TestEvaluateFanRulesRespectDistinctCounterpartiesAndWindowBoundary(t *testing.T) {
	withinBoundary := []db.Transfer{
		ruleTransfer("out-1", hub, alice, 0),
		ruleTransfer("out-2", hub, bob, 12*time.Hour),
		ruleTransfer("out-3", hub, carol, 24*time.Hour),
	}
	leads, _ := Evaluate("ethereum-mainnet", withinBoundary, time.Now().UTC())
	if findLead(t, leads, "fan-out-dispersion").SubjectAddress != hub {
		t.Fatalf("fan-out lead = %#v", leads)
	}

	outsideBoundary := append([]db.Transfer(nil), withinBoundary...)
	outsideBoundary[2].BlockTimestamp = outsideBoundary[2].BlockTimestamp.Add(time.Second)
	leads, _ = Evaluate("ethereum-mainnet", outsideBoundary, time.Now().UTC())
	if hasLead(leads, "fan-out-dispersion") {
		t.Fatalf("fan-out incorrectly crossed the 24-hour window: %#v", leads)
	}

	notDistinct := []db.Transfer{
		ruleTransfer("in-1", alice, hub, 0),
		ruleTransfer("in-2", alice, hub, time.Hour),
		ruleTransfer("in-3", bob, hub, 2*time.Hour),
	}
	leads, _ = Evaluate("ethereum-mainnet", notDistinct, time.Now().UTC())
	if hasLead(leads, "fan-in-consolidation") {
		t.Fatalf("fan-in incorrectly counted a repeated counterparty: %#v", leads)
	}
}

func TestEvaluateFindsRapidOnwardTransferOnlyWithinWindow(t *testing.T) {
	transfers := []db.Transfer{
		ruleTransfer("incoming", alice, hub, 0),
		ruleTransfer("onward", hub, bob, time.Hour-time.Second),
	}
	leads, _ := Evaluate("ethereum-mainnet", transfers, time.Now().UTC())
	lead := findLead(t, leads, "rapid-onward-transfer")
	if lead.SubjectAddress != hub || !reflect.DeepEqual(lead.TransferIDs, []string{"incoming", "onward"}) {
		t.Fatalf("rapid onward lead = %#v", lead)
	}

	transfers[1].BlockTimestamp = transfers[1].BlockTimestamp.Add(2 * time.Second)
	leads, _ = Evaluate("ethereum-mainnet", transfers, time.Now().UTC())
	if hasLead(leads, "rapid-onward-transfer") {
		t.Fatalf("rapid onward incorrectly crossed its time window: %#v", leads)
	}
}

func TestEvaluateIgnoresProvisionalTransfers(t *testing.T) {
	transfers := []db.Transfer{
		ruleTransfer("provisional-1", alice, hub, 0),
		ruleTransfer("provisional-2", bob, hub, time.Hour),
		ruleTransfer("provisional-3", carol, hub, 2*time.Hour),
	}
	transfers[2].Provisional = true
	leads, runs := Evaluate("ethereum-mainnet", transfers, time.Now().UTC())
	if hasLead(leads, "fan-in-consolidation") || !reflect.DeepEqual(runs[0].InputTransferIDs, []string{"provisional-1", "provisional-2"}) {
		t.Fatalf("provisional transfer influenced rule evaluation: leads=%#v runs=%#v", leads, runs)
	}
}

func ruleTransfer(id, from, to string, offset time.Duration) db.Transfer {
	return db.Transfer{
		ID:             id,
		Network:        "ethereum-mainnet",
		FromAddress:    from,
		ToAddress:      to,
		Asset:          adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18},
		BlockTimestamp: time.Unix(1_700_000_000, 0).UTC().Add(offset),
	}
}

func findLead(t *testing.T, leads []Lead, ruleID string) Lead {
	t.Helper()
	for _, lead := range leads {
		if lead.RuleID == ruleID {
			return lead
		}
	}
	t.Fatalf("missing %s lead in %#v", ruleID, leads)
	return Lead{}
}

func hasLead(leads []Lead, ruleID string) bool {
	for _, lead := range leads {
		if lead.RuleID == ruleID {
			return true
		}
	}
	return false
}
