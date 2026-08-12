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

const neutralLimitation = "This deterministic pattern is an investigative lead based on the selected on-chain observations. It is not an attribution, risk score, or conclusion about intent."

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
			Name:              "Rapid onward transfer",
			ParameterSchema:   json.RawMessage(`{"type":"object","properties":{"window_seconds":{"type":"integer","minimum":1,"default":3600},"include_provisional":{"type":"boolean","default":false}},"additionalProperties":false}`),
			DefaultParameters: json.RawMessage(`{"window_seconds":3600,"include_provisional":false}`),
			Limitations:       neutralLimitation,
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
