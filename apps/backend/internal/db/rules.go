package db

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

type RuleCatalogEntry struct {
	RuleID, Version, Name, Limitations string
	ParameterSchema, DefaultParameters json.RawMessage
}

type RuleRun struct {
	Network, RuleID, RuleVersion string
	Parameters                   json.RawMessage
	InputTransferIDs             []string
	Result                       json.RawMessage
	StartedAt, CompletedAt       time.Time
}

const insertRuleCatalogSQL = `INSERT INTO rule_catalog (rule_id, version, name, parameter_schema, default_parameters, limitations) VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (rule_id, version) DO NOTHING`
const insertRuleRunSQL = `INSERT INTO rule_runs (network, rule_id, rule_version, parameters, input_transfer_ids, result, started_at, completed_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

func (d *DB) ImportRuleCatalog(ctx context.Context, entries []RuleCatalogEntry) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rule catalog transaction: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, insertRuleCatalogSQL)
	if err != nil {
		return fmt.Errorf("prepare rule catalog insert: %w", err)
	}
	defer statement.Close()
	for _, entry := range entries {
		if entry.RuleID == "" || entry.Version == "" || entry.Name == "" || entry.Limitations == "" || !json.Valid(entry.ParameterSchema) || !json.Valid(entry.DefaultParameters) {
			return fmt.Errorf("invalid rule catalog entry")
		}
		if _, err := statement.ExecContext(ctx, entry.RuleID, entry.Version, entry.Name, entry.ParameterSchema, entry.DefaultParameters, entry.Limitations); err != nil {
			return fmt.Errorf("insert rule catalog entry: %w", err)
		}
	}
	return tx.Commit()
}

func (d *DB) SaveRuleRuns(ctx context.Context, runs []RuleRun) error {
	if len(runs) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rule run transaction: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, insertRuleRunSQL)
	if err != nil {
		return fmt.Errorf("prepare rule run insert: %w", err)
	}
	defer statement.Close()
	for _, run := range runs {
		inputIDs, err := json.Marshal(run.InputTransferIDs)
		if err != nil {
			return fmt.Errorf("encode rule run transfer ids: %w", err)
		}
		if run.Network == "" || run.RuleID == "" || run.RuleVersion == "" || run.StartedAt.IsZero() || run.CompletedAt.IsZero() || !json.Valid(run.Parameters) || !json.Valid(inputIDs) || !json.Valid(run.Result) {
			return fmt.Errorf("invalid rule run")
		}
		if _, err := statement.ExecContext(ctx, run.Network, run.RuleID, run.RuleVersion, run.Parameters, inputIDs, run.Result, run.StartedAt, run.CompletedAt); err != nil {
			return fmt.Errorf("insert rule run: %w", err)
		}
	}
	return tx.Commit()
}
