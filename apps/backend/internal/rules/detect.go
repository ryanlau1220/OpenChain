package rules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

type Lead struct {
	ID, RuleID, RuleVersion, Title, SubjectAddress, Rationale, Limitations string
	Parameters                                                             json.RawMessage
	TransferIDs                                                            []string
}

type Run = db.RuleRun

func Evaluate(network string, transfers []db.Transfer, completedAt time.Time) ([]Lead, []Run) {
	finalized := finalizedTransfers(transfers)
	definitions := Catalog()
	leads := make([]Lead, 0)
	runs := make([]Run, 0, len(definitions))
	for _, definition := range definitions {
		var detected []Lead
		switch definition.ID {
		case "fan-in-consolidation":
			detected = fanIn(definition, finalized)
		case "fan-out-dispersion":
			detected = fanOut(definition, finalized)
		case "rapid-onward-transfer":
			detected = rapidOnward(definition, finalized)
		}
		sort.Slice(detected, func(left, right int) bool { return detected[left].ID < detected[right].ID })
		result, _ := json.Marshal(detected)
		runs = append(runs, Run{Network: network, RuleID: definition.ID, RuleVersion: definition.Version, Parameters: definition.DefaultParameters, InputTransferIDs: transferIDs(finalized), Result: result, StartedAt: completedAt, CompletedAt: completedAt})
		leads = append(leads, detected...)
	}
	sort.Slice(leads, func(left, right int) bool { return leads[left].ID < leads[right].ID })
	return leads, runs
}

func finalizedTransfers(transfers []db.Transfer) []db.Transfer {
	result := make([]db.Transfer, 0, len(transfers))
	for _, transfer := range transfers {
		if !transfer.Provisional && !transfer.BlockTimestamp.IsZero() {
			result = append(result, transfer)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if !result[left].BlockTimestamp.Equal(result[right].BlockTimestamp) {
			return result[left].BlockTimestamp.Before(result[right].BlockTimestamp)
		}
		return result[left].ID < result[right].ID
	})
	return result
}

func fanIn(definition Definition, transfers []db.Transfer) []Lead {
	return fan(definition, transfers, true)
}

func fanOut(definition Definition, transfers []db.Transfer) []Lead {
	return fan(definition, transfers, false)
}

func fan(definition Definition, transfers []db.Transfer, inbound bool) []Lead {
	window, minimum := fanParameters(definition)
	if window <= 0 || minimum < 2 {
		return nil
	}
	groups := make(map[string][]db.Transfer)
	for _, transfer := range transfers {
		subject, counterpart := transfer.FromAddress, transfer.ToAddress
		if inbound {
			subject, counterpart = transfer.ToAddress, transfer.FromAddress
		}
		if subject == "" || counterpart == "" || subject == counterpart {
			continue
		}
		groups[subject+"\x00"+assetKey(transfer)] = append(groups[subject+"\x00"+assetKey(transfer)], transfer)
	}
	leads := make([]Lead, 0)
	for _, group := range groups {
		sortTransfers(group)
		if matches, distinct := firstDistinctWindow(group, inbound, minimum, window); len(matches) > 0 {
			subject := matches[0].FromAddress
			if inbound {
				subject = matches[0].ToAddress
			}
			direction, title := "outbound", "Fan-out / dispersion lead"
			if inbound {
				direction, title = "inbound", "Fan-in / consolidation lead"
			}
			ids := transferIDs(matches)
			leads = append(leads, lead(definition, subject, title, "Observed "+itoa(len(ids))+" "+direction+" transfers involving "+itoa(distinct)+" distinct counterparties within "+itoa(int(window.Hours()))+" hours.", ids))
		}
	}
	return leads
}

func firstDistinctWindow(group []db.Transfer, inbound bool, minimum int, duration time.Duration) ([]db.Transfer, int) {
	for start := range group {
		seen := map[string]struct{}{}
		for end := start; end < len(group); end++ {
			if group[end].BlockTimestamp.Sub(group[start].BlockTimestamp) > duration {
				break
			}
			counterpart := group[end].ToAddress
			if inbound {
				counterpart = group[end].FromAddress
			}
			seen[counterpart] = struct{}{}
			if len(seen) >= minimum {
				return append([]db.Transfer(nil), group[start:end+1]...), len(seen)
			}
		}
	}
	return nil, 0
}

func rapidOnward(definition Definition, transfers []db.Transfer) []Lead {
	window := rapidParameters(definition)
	if window <= 0 {
		return nil
	}
	byMiddle := make(map[string][]db.Transfer)
	for _, transfer := range transfers {
		if transfer.FromAddress != "" {
			byMiddle[transfer.FromAddress+"\x00"+assetKey(transfer)] = append(byMiddle[transfer.FromAddress+"\x00"+assetKey(transfer)], transfer)
		}
		if transfer.ToAddress != "" {
			byMiddle[transfer.ToAddress+"\x00"+assetKey(transfer)] = append(byMiddle[transfer.ToAddress+"\x00"+assetKey(transfer)], transfer)
		}
	}
	leads := make([]Lead, 0)
	for key, group := range byMiddle {
		middle := strings.SplitN(key, "\x00", 2)[0]
		sortTransfers(group)
		found := false
		for _, inbound := range group {
			if inbound.ToAddress != middle {
				continue
			}
			for _, outbound := range group {
				if outbound.FromAddress != middle || outbound.ToAddress == inbound.FromAddress || outbound.BlockTimestamp.Before(inbound.BlockTimestamp) {
					continue
				}
				if outbound.BlockTimestamp.Sub(inbound.BlockTimestamp) <= window {
					ids := transferIDs([]db.Transfer{inbound, outbound})
					leads = append(leads, lead(definition, middle, "Rapid onward-transfer lead", "Observed an inbound transfer followed by a transfer of the same asset within "+itoa(int(window.Hours()))+" hour.", ids))
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	return leads
}

func fanParameters(definition Definition) (time.Duration, int) {
	var parameters struct {
		WindowSeconds             int `json:"window_seconds"`
		MinDistinctCounterparties int `json:"min_distinct_counterparties"`
	}
	if err := json.Unmarshal(definition.DefaultParameters, &parameters); err != nil || parameters.WindowSeconds < 1 || parameters.MinDistinctCounterparties < 2 {
		return 0, 0
	}
	return time.Duration(parameters.WindowSeconds) * time.Second, parameters.MinDistinctCounterparties
}

func rapidParameters(definition Definition) time.Duration {
	var parameters struct {
		WindowSeconds int `json:"window_seconds"`
	}
	if err := json.Unmarshal(definition.DefaultParameters, &parameters); err != nil || parameters.WindowSeconds < 1 {
		return 0
	}
	return time.Duration(parameters.WindowSeconds) * time.Second
}

func lead(definition Definition, subject, title, rationale string, ids []string) Lead {
	ids = append([]string(nil), ids...)
	sort.Strings(ids)
	hash := sha256.Sum256([]byte(definition.ID + "\x00" + definition.Version + "\x00" + subject + "\x00" + strings.Join(ids, "\x00")))
	return Lead{ID: definition.ID + ":" + hex.EncodeToString(hash[:8]), RuleID: definition.ID, RuleVersion: definition.Version, Title: title, SubjectAddress: subject, Rationale: rationale, Limitations: definition.Limitations, Parameters: definition.DefaultParameters, TransferIDs: ids}
}

func transferIDs(transfers []db.Transfer) []string {
	ids := make([]string, 0, len(transfers))
	for _, transfer := range transfers {
		ids = append(ids, transfer.ID)
	}
	sort.Strings(ids)
	return ids
}

func assetKey(transfer db.Transfer) string {
	return transfer.Asset.Kind + "\x00" + transfer.Asset.ContractAddress + "\x00" + transfer.Asset.Symbol
}

func sortTransfers(transfers []db.Transfer) {
	sort.Slice(transfers, func(left, right int) bool {
		if !transfers[left].BlockTimestamp.Equal(transfers[right].BlockTimestamp) {
			return transfers[left].BlockTimestamp.Before(transfers[right].BlockTimestamp)
		}
		return transfers[left].ID < transfers[right].ID
	})
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
