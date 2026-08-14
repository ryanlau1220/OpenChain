package rules

import (
	"encoding/json"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

type Definition struct {
	ID                string
	Version           string
	Name              string
	ParameterSchema   json.RawMessage
	DefaultParameters json.RawMessage
	Limitations       string
}

const neutralLimitation = "This deterministic pattern is an investigative lead within the retrieved graph scope and finalized observations only. It is not an attribution, risk score, conclusion about intent, or complete address history."

func Catalog() []Definition {
	return []Definition{
		{
			ID:                "fan-in-consolidation",
			Version:           "1.0.0",
			Name:              "Fan-in / consolidation",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":86400},"min_distinct_counterparties":{"type":"integer","minimum":2,"default":3},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":86400,"min_distinct_counterparties":3,"include_provisional":false}`),
			Limitations:       neutralLimitation,
		},
		{
			ID:                "fan-out-dispersion",
			Version:           "1.0.0",
			Name:              "Fan-out / dispersion",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":86400},"min_distinct_counterparties":{"type":"integer","minimum":2,"default":3},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":86400,"min_distinct_counterparties":3,"include_provisional":false}`),
			Limitations:       neutralLimitation,
		},
		{
			ID:                "rapid-onward-transfer",
			Version:           "1.0.0",
			Name:              "Rapid onward transfer pattern",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":3600},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":3600,"include_provisional":false}`),
			Limitations:       neutralLimitation,
		},
		{
			ID:                "repeated-equal-amount-dispersion",
			Version:           "1.0.0",
			Name:              "Repeated equal-amount dispersion pattern",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":86400},"min_distinct_counterparties":{"type":"integer","minimum":2,"default":3},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":86400,"min_distinct_counterparties":3,"include_provisional":false}`),
			Limitations:       "This deterministic pattern compares equal raw amounts of the same asset within the retrieved graph scope's finalized observations. It does not determine whether an amount is small, a fiat value, ownership, or intent.",
		},
		{
			ID:                "sequential-decreasing-transfer",
			Version:           "1.0.0",
			Name:              "Sequential decreasing-transfer pattern",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":3600},"min_hops":{"type":"integer","minimum":3,"default":3},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":3600,"min_hops":3,"include_provisional":false}`),
			Limitations:       neutralLimitation,
		},
		{
			ID:                "brief-intermediary-pass-through",
			Version:           "1.0.0",
			Name:              "Briefly observed intermediary pattern",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":3600},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":3600,"include_provisional":false}`),
			Limitations:       "This deterministic pattern is limited to the retrieved graph scope's finalized observations. It identifies a same-amount pass-through, not an address lifetime, ownership, attribution, risk score, or intent.",
		},
		{
			ID:                "op-stack-bridge-correlation",
			Version:           "1.0.0",
			Name:              "OP Stack bridge correlation",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"max_delay_seconds":{"type":"integer","minimum":1,"default":604800}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"max_delay_seconds":604800}`),
			Limitations:       "This is a deterministic bridge-transfer match within retrieved provider pages using known bridge contracts, recipient, raw amount, and time window. It does not prove wallet ownership, token equivalence, or intent.",
		},
	}
}

func CatalogEntries() []db.RuleCatalogEntry {
	definitions := Catalog()
	entries := make([]db.RuleCatalogEntry, 0, len(definitions))
	for _, definition := range definitions {
		entries = append(entries, db.RuleCatalogEntry{
			RuleID:            definition.ID,
			Version:           definition.Version,
			Name:              definition.Name,
			ParameterSchema:   definition.ParameterSchema,
			DefaultParameters: definition.DefaultParameters,
			Limitations:       definition.Limitations,
		})
	}
	return entries
}
