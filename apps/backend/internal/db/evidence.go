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

type RecordedAcquisitionScope struct {
	ID                       int64
	Network, Address, Cursor string
	RetrievedAt              time.Time
}

type AcquisitionScopeTransfer struct {
	ScopeID    int64
	TransferID string
}

type AcquisitionScopeSnapshot struct {
	ScopeID       int64
	AcquisitionID int64
}

type RecordedRuleRun struct {
	ID                                   int64
	Network, RuleID, RuleVersion         string
	Parameters, InputTransferIDs, Result json.RawMessage
	StartedAt, CompletedAt               time.Time
}

type EvidenceExport struct {
	Transfers                    []Transfer
	Snapshots                    []AcquisitionSnapshot
	Scopes                       []RecordedAcquisitionScope
	ScopeTransfers               []AcquisitionScopeTransfer
	ScopeSnapshots               []AcquisitionScopeSnapshot
	RuleRuns                     []RecordedRuleRun
	Labels                       []CuratedLabel
	BridgeTransitions            []BridgeTransition
	BridgeTransitionAcquisitions []BridgeTransitionAcquisition
}

const maxEvidenceTransferIDs = 250

func (d *DB) ExportEvidence(ctx context.Context, network string, transferIDs []string) (*EvidenceExport, error) {
	ids := uniqueStrings(transferIDs)
	if network == "" || len(ids) == 0 || len(ids) > maxEvidenceTransferIDs {
		return nil, fmt.Errorf("select between 1 and %d transfers", maxEvidenceTransferIDs)
	}
	transfers, err := d.exportTransfers(ctx, ids)
	if err != nil {
		return nil, err
	}
	if len(transfers) != len(ids) {
		return nil, fmt.Errorf("one or more selected transfers are unavailable")
	}
	scopes, err := d.exportAcquisitionScopes(ctx, ids)
	if err != nil {
		return nil, err
	}
	scopeIDs := make([]int64, 0, len(scopes))
	for _, scope := range scopes {
		scopeIDs = append(scopeIDs, scope.ID)
	}
	snapshots, err := d.exportSnapshots(ctx, scopeIDs)
	if err != nil {
		return nil, err
	}
	scopeTransfers, err := d.exportScopeTransfers(ctx, scopeIDs, ids)
	if err != nil {
		return nil, err
	}
	scopeSnapshots, err := d.exportScopeSnapshots(ctx, scopeIDs)
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
	labels, err := d.exportLabels(ctx, uniqueStrings(addresses))
	if err != nil {
		return nil, err
	}
	bridgeTransitions, bridgeLinks, err := d.ExportBridgeTransitions(ctx, ids)
	if err != nil {
		return nil, err
	}
	return &EvidenceExport{Transfers: transfers, Snapshots: snapshots, Scopes: scopes, ScopeTransfers: scopeTransfers, ScopeSnapshots: scopeSnapshots, RuleRuns: runs, Labels: labels, BridgeTransitions: bridgeTransitions, BridgeTransitionAcquisitions: bridgeLinks}, nil
}

func (d *DB) exportTransfers(ctx context.Context, ids []string) ([]Transfer, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT id, network, transaction_hash, event_id, transfer_kind, from_address, to_address, asset_symbol, asset_kind, asset_contract_address, asset_decimals, amount_base_units, block_number, block_hash, block_timestamp, provisional, source, retrieved_at FROM transfers WHERE id = ANY($1) ORDER BY id`, pq.Array(ids))
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

func (d *DB) exportAcquisitionScopes(ctx context.Context, ids []string) ([]RecordedAcquisitionScope, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT DISTINCT scope.id, scope.network, scope.address, scope.cursor, scope.retrieved_at FROM acquisition_scopes scope JOIN acquisition_scope_transfers link ON link.scope_id = scope.id WHERE link.transfer_id = ANY($1) ORDER BY scope.id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query package acquisition scopes: %w", err)
	}
	defer rows.Close()
	result := make([]RecordedAcquisitionScope, 0)
	for rows.Next() {
		var scope RecordedAcquisitionScope
		if err := rows.Scan(&scope.ID, &scope.Network, &scope.Address, &scope.Cursor, &scope.RetrievedAt); err != nil {
			return nil, fmt.Errorf("scan package acquisition scope: %w", err)
		}
		result = append(result, scope)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package acquisition scopes: %w", err)
	}
	return result, nil
}

func (d *DB) exportSnapshots(ctx context.Context, scopeIDs []int64) ([]AcquisitionSnapshot, error) {
	if len(scopeIDs) == 0 {
		return []AcquisitionSnapshot{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT DISTINCT snapshot.id, snapshot.network, snapshot.provider, snapshot.request_identity, snapshot.response_sha256, blob.response_body, snapshot.retrieved_at FROM acquisition_snapshots snapshot JOIN acquisition_scope_snapshots link ON link.acquisition_id = snapshot.id JOIN acquisition_blobs blob ON blob.response_sha256 = snapshot.response_sha256 WHERE link.scope_id = ANY($1) ORDER BY snapshot.id`, pq.Array(scopeIDs))
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

func (d *DB) exportScopeTransfers(ctx context.Context, scopeIDs []int64, transferIDs []string) ([]AcquisitionScopeTransfer, error) {
	if len(scopeIDs) == 0 {
		return []AcquisitionScopeTransfer{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT scope_id, transfer_id FROM acquisition_scope_transfers WHERE scope_id = ANY($1) AND transfer_id = ANY($2) ORDER BY scope_id, transfer_id`, pq.Array(scopeIDs), pq.Array(transferIDs))
	if err != nil {
		return nil, fmt.Errorf("query package scope transfers: %w", err)
	}
	defer rows.Close()
	result := make([]AcquisitionScopeTransfer, 0)
	for rows.Next() {
		var link AcquisitionScopeTransfer
		if err := rows.Scan(&link.ScopeID, &link.TransferID); err != nil {
			return nil, fmt.Errorf("scan package scope transfer: %w", err)
		}
		result = append(result, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package scope transfers: %w", err)
	}
	return result, nil
}

func (d *DB) exportScopeSnapshots(ctx context.Context, scopeIDs []int64) ([]AcquisitionScopeSnapshot, error) {
	if len(scopeIDs) == 0 {
		return []AcquisitionScopeSnapshot{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT scope_id, acquisition_id FROM acquisition_scope_snapshots WHERE scope_id = ANY($1) ORDER BY scope_id, acquisition_id`, pq.Array(scopeIDs))
	if err != nil {
		return nil, fmt.Errorf("query package scope snapshots: %w", err)
	}
	defer rows.Close()
	result := make([]AcquisitionScopeSnapshot, 0)
	for rows.Next() {
		var link AcquisitionScopeSnapshot
		if err := rows.Scan(&link.ScopeID, &link.AcquisitionID); err != nil {
			return nil, fmt.Errorf("scan package scope snapshot: %w", err)
		}
		result = append(result, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate package scope snapshots: %w", err)
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

func (d *DB) exportLabels(ctx context.Context, addresses []string) ([]CuratedLabel, error) {
	if len(addresses) == 0 {
		return []CuratedLabel{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT `+labelColumns+labelAssertionFrom+`WHERE assertion.address = ANY($1) AND `+currentLabelPredicate+` ORDER BY assertion.network, assertion.trust_tier, assertion.label`, pq.Array(addresses))
	if err != nil {
		return nil, fmt.Errorf("query package labels: %w", err)
	}
	defer rows.Close()
	result := make([]CuratedLabel, 0)
	for rows.Next() {
		label, err := scanCuratedLabel(rows)
		if err != nil {
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
