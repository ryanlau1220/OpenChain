package db

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/lib/pq"
)

type AcquisitionSnapshot struct {
	ID                                       int64
	Network, Provider, RequestIdentity, Hash string
	Response                                 []byte
	RetrievedAt                              time.Time
}

type TransferAcquisition struct {
	TransferID    string
	AcquisitionID int64
}

type RecordedRuleRun struct {
	ID                                   int64
	Network, RuleID, RuleVersion         string
	Parameters, InputTransferIDs, Result json.RawMessage
	StartedAt, CompletedAt               time.Time
}

type EvidenceExport struct {
	Transfers  []Transfer
	Snapshots  []AcquisitionSnapshot
	Provenance []TransferAcquisition
	RuleRuns   []RecordedRuleRun
	Labels     []CuratedLabel
}

const maxEvidenceTransferIDs = 250

func (d *DB) ExportEvidence(ctx context.Context, network string, transferIDs []string) (*EvidenceExport, error) {
	ids := uniqueStrings(transferIDs)
	if network == "" || len(ids) == 0 || len(ids) > maxEvidenceTransferIDs {
		return nil, fmt.Errorf("select between 1 and %d transfers", maxEvidenceTransferIDs)
	}
	transfers, err := d.exportTransfers(ctx, network, ids)
	if err != nil {
		return nil, err
	}
	if len(transfers) != len(ids) {
		return nil, fmt.Errorf("one or more selected transfers are unavailable")
	}
	snapshots, err := d.exportSnapshots(ctx, ids)
	if err != nil {
		return nil, err
	}
	provenance, err := d.exportProvenance(ctx, ids)
	if err != nil {
		return nil, err
	}
	runs, err := d.exportRuleRuns(ctx, network, ids)
	if err != nil {
		return nil, err
	}
	addresses := make([]string, 0, len(transfers)*2)
	for _, transfer := range transfers {
		addresses = append(addresses, transfer.FromAddress, transfer.ToAddress)
	}
	labels, err := d.exportLabels(ctx, network, uniqueStrings(addresses))
	if err != nil {
		return nil, err
	}
	return &EvidenceExport{Transfers: transfers, Snapshots: snapshots, Provenance: provenance, RuleRuns: runs, Labels: labels}, nil
}

func (d *DB) exportTransfers(ctx context.Context, network string, ids []string) ([]Transfer, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id, network, transaction_hash, event_id, transfer_kind, from_address, to_address, asset_symbol, asset_kind, asset_contract_address, asset_decimals, amount_base_units, block_number, block_hash, block_timestamp, provisional, source, retrieved_at FROM transfers WHERE network = $1 AND id = ANY($2) ORDER BY id`, network, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query package transfers: %w", err)
	}
	defer rows.Close()
	result := make([]Transfer, 0, len(ids))
	for rows.Next() {
		var transfer Transfer
		if err := rows.Scan(&transfer.ID, &transfer.Network, &transfer.TransactionHash, &transfer.EventID, &transfer.TransferKind, &transfer.FromAddress, &transfer.ToAddress, &transfer.Asset.Symbol, &transfer.Asset.Kind, &transfer.Asset.ContractAddress, &transfer.Asset.Decimals, &transfer.AmountBaseUnits, &transfer.BlockNumber, &transfer.BlockHash, &transfer.BlockTimestamp, &transfer.Provisional, &transfer.Source, &transfer.RetrievedAt); err != nil {
			return nil, fmt.Errorf("scan package transfer: %w", err)
		}
		result = append(result, transfer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package transfers: %w", err)
	}
	return result, nil
}

func (d *DB) exportSnapshots(ctx context.Context, ids []string) ([]AcquisitionSnapshot, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT DISTINCT snapshot.id, snapshot.network, snapshot.provider, snapshot.request_identity, snapshot.response_sha256, snapshot.response_body, snapshot.retrieved_at FROM acquisition_snapshots snapshot JOIN transfer_acquisitions link ON link.acquisition_id = snapshot.id WHERE link.transfer_id = ANY($1) ORDER BY snapshot.id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query package acquisition snapshots: %w", err)
	}
	defer rows.Close()
	result := make([]AcquisitionSnapshot, 0)
	for rows.Next() {
		var snapshot AcquisitionSnapshot
		if err := rows.Scan(&snapshot.ID, &snapshot.Network, &snapshot.Provider, &snapshot.RequestIdentity, &snapshot.Hash, &snapshot.Response, &snapshot.RetrievedAt); err != nil {
			return nil, fmt.Errorf("scan package acquisition snapshot: %w", err)
		}
		result = append(result, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package acquisition snapshots: %w", err)
	}
	return result, nil
}

func (d *DB) exportProvenance(ctx context.Context, ids []string) ([]TransferAcquisition, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT transfer_id, acquisition_id FROM transfer_acquisitions WHERE transfer_id = ANY($1) ORDER BY transfer_id, acquisition_id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query package provenance: %w", err)
	}
	defer rows.Close()
	result := make([]TransferAcquisition, 0)
	for rows.Next() {
		var link TransferAcquisition
		if err := rows.Scan(&link.TransferID, &link.AcquisitionID); err != nil {
			return nil, fmt.Errorf("scan package provenance: %w", err)
		}
		result = append(result, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package provenance: %w", err)
	}
	return result, nil
}

func (d *DB) exportRuleRuns(ctx context.Context, network string, ids []string) ([]RecordedRuleRun, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id, network, rule_id, rule_version, parameters, input_transfer_ids, result, started_at, completed_at FROM rule_runs WHERE network = $1 AND input_transfer_ids ?| $2::text[] ORDER BY id`, network, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query package rule runs: %w", err)
	}
	defer rows.Close()
	result := make([]RecordedRuleRun, 0)
	for rows.Next() {
		var run RecordedRuleRun
		if err := rows.Scan(&run.ID, &run.Network, &run.RuleID, &run.RuleVersion, &run.Parameters, &run.InputTransferIDs, &run.Result, &run.StartedAt, &run.CompletedAt); err != nil {
			return nil, fmt.Errorf("scan package rule run: %w", err)
		}
		result = append(result, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package rule runs: %w", err)
	}
	return result, nil
}

func (d *DB) exportLabels(ctx context.Context, network string, addresses []string) ([]CuratedLabel, error) {
	if len(addresses) == 0 {
		return []CuratedLabel{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT id, network, address, category, label, confidence, evidence_url, source, source_version, visibility, trust_tier, created_by, created_at FROM curated_labels WHERE network = $1 AND address = ANY($2) ORDER BY trust_tier, label`, network, pq.Array(addresses))
	if err != nil {
		return nil, fmt.Errorf("query package labels: %w", err)
	}
	defer rows.Close()
	result := make([]CuratedLabel, 0)
	for rows.Next() {
		var label CuratedLabel
		if err := rows.Scan(&label.ID, &label.Network, &label.Address, &label.Category, &label.Label, &label.Confidence, &label.EvidenceURL, &label.Source, &label.SourceVersion, &label.Visibility, &label.TrustTier, &label.CreatedBy, &label.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan package label: %w", err)
		}
		result = append(result, label)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package labels: %w", err)
	}
	return result, nil
}

func uniqueStrings(values []string) []string {
	unique := make(map[string]struct{}, len(values))
	for _, value := range values {
		if value != "" {
			unique[value] = struct{}{}
		}
	}
	result := make([]string, 0, len(unique))
	for value := range unique {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}
