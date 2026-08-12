package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
)

const graphName = "openchain"

type Transfer struct {
	ID, Network, TransactionHash, EventID, TransferKind, FromAddress, ToAddress, AmountBaseUnits, Source, BlockHash string
	Asset                                                                                                           adapter.Asset
	BlockNumber                                                                                                     int64
	BlockTimestamp, RetrievedAt                                                                                     time.Time
	Provisional                                                                                                     bool
}

type Address struct {
	Network, Address, Label, EntityType string
}

const insertTransferSQL = `INSERT INTO transfers (id, network, transaction_hash, event_id, transfer_kind, from_address, to_address, asset_symbol, asset_kind, asset_contract_address, asset_decimals, amount_base_units, block_number, block_hash, block_timestamp, provisional, source, retrieved_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18) ON CONFLICT (id) DO NOTHING`
const insertAcquisitionSQL = `INSERT INTO acquisition_snapshots (network, provider, request_identity, response_sha256, response_body, retrieved_at) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id`
const insertTransferAcquisitionSQL = `INSERT INTO transfer_acquisitions (transfer_id, acquisition_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`
const upsertAssetSQL = `INSERT INTO assets (network, contract_address, kind, symbol, decimals, retrieved_at) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (network, contract_address) DO UPDATE SET kind = EXCLUDED.kind, symbol = EXCLUDED.symbol, decimals = EXCLUDED.decimals, retrieved_at = EXCLUDED.retrieved_at`

const upsertAddressSQL = `SELECT * FROM cypher('openchain', $$
MERGE (address:Address {network: $network, address: $address})
SET address.label = $label, address.entity_type = $entity_type
RETURN address
$$, $1) AS (result agtype)`

const upsertFundFlowSQL = `SELECT * FROM cypher('openchain', $$
MATCH (from:Address {network: $network, address: $from_address}), (to:Address {network: $network, address: $to_address})
MERGE (from)-[flow:FundFlow {id: $id}]->(to)
SET flow.network = $network,
    flow.transaction_hash = $transaction_hash,
    flow.event_id = $event_id,
    flow.transfer_kind = $transfer_kind,
    flow.asset_kind = $asset_kind,
    flow.asset_contract_address = $asset_contract_address,
    flow.asset_decimals = $asset_decimals,
    flow.asset_symbol = $asset_symbol,
    flow.amount_base_units = $amount_base_units,
    flow.block_number = $block_number,
    flow.block_timestamp = $block_timestamp,
    flow.source = $source
RETURN flow
$$, $1) AS (result agtype)`

func (d *DB) SaveEvidenceGraph(ctx context.Context, addresses []Address, transfers []Transfer, acquisitions []adapter.RawAcquisition) error {
	if len(addresses) == 0 && len(transfers) == 0 && len(acquisitions) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer transaction: %w", err)
	}
	defer tx.Rollback()
	acquisitionIDs, err := insertAcquisitions(ctx, tx, transfers, acquisitions)
	if err != nil {
		return err
	}
	statement, err := tx.PrepareContext(ctx, insertTransferSQL)
	if err != nil {
		return fmt.Errorf("prepare transfer insert: %w", err)
	}
	defer statement.Close()
	assetStatement, err := tx.PrepareContext(ctx, upsertAssetSQL)
	if err != nil {
		return fmt.Errorf("prepare asset upsert: %w", err)
	}
	defer assetStatement.Close()
	for _, transfer := range transfers {
		if _, err := assetStatement.ExecContext(ctx, transfer.Network, transfer.Asset.ContractAddress, transfer.Asset.Kind, transfer.Asset.Symbol, transfer.Asset.Decimals, transfer.RetrievedAt); err != nil {
			return fmt.Errorf("upsert asset: %w", err)
		}
		if _, err := statement.ExecContext(ctx, transfer.ID, transfer.Network, transfer.TransactionHash, transfer.EventID, transfer.TransferKind, transfer.FromAddress, transfer.ToAddress, transfer.Asset.Symbol, transfer.Asset.Kind, transfer.Asset.ContractAddress, transfer.Asset.Decimals, transfer.AmountBaseUnits, transfer.BlockNumber, transfer.BlockHash, transfer.BlockTimestamp, transfer.Provisional, transfer.Source, transfer.RetrievedAt); err != nil {
			return fmt.Errorf("insert transfer: %w", err)
		}
		for _, acquisitionID := range acquisitionIDs {
			if _, err := tx.ExecContext(ctx, insertTransferAcquisitionSQL, transfer.ID, acquisitionID); err != nil {
				return fmt.Errorf("link transfer acquisition: %w", err)
			}
		}
	}
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		return fmt.Errorf("load Apache AGE: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = ag_catalog, "$user", public`); err != nil {
		return fmt.Errorf("set Apache AGE search path: %w", err)
	}
	for _, address := range addresses {
		if err := execCypher(ctx, tx, upsertAddressSQL, map[string]string{"network": address.Network, "address": address.Address, "label": address.Label, "entity_type": address.EntityType}); err != nil {
			return fmt.Errorf("upsert address graph node: %w", err)
		}
	}
	for _, transfer := range transfers {
		if err := execCypher(ctx, tx, upsertFundFlowSQL, map[string]any{
			"id": transfer.ID, "network": transfer.Network, "transaction_hash": transfer.TransactionHash,
			"event_id": transfer.EventID, "transfer_kind": transfer.TransferKind, "from_address": transfer.FromAddress, "to_address": transfer.ToAddress,
			"asset_symbol": transfer.Asset.Symbol, "asset_kind": transfer.Asset.Kind, "asset_contract_address": transfer.Asset.ContractAddress, "asset_decimals": transfer.Asset.Decimals, "amount_base_units": transfer.AmountBaseUnits,
			"block_number": transfer.BlockNumber, "block_timestamp": transfer.BlockTimestamp.Unix(), "source": transfer.Source,
		}); err != nil {
			return fmt.Errorf("upsert fund flow graph edge: %w", err)
		}
	}
	return tx.Commit()
}

func (d *DB) SaveAcquisitions(ctx context.Context, network string, acquisitions []adapter.RawAcquisition) error {
	if len(acquisitions) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin acquisition transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := insertAcquisitions(ctx, tx, []Transfer{{Network: network}}, acquisitions); err != nil {
		return err
	}
	return tx.Commit()
}

func insertAcquisitions(ctx context.Context, tx *sql.Tx, transfers []Transfer, acquisitions []adapter.RawAcquisition) ([]int64, error) {
	if len(acquisitions) == 0 {
		return nil, nil
	}
	network := ""
	if len(transfers) > 0 {
		network = transfers[0].Network
	}
	if network == "" {
		return nil, fmt.Errorf("acquisition network is required")
	}
	ids := make([]int64, 0, len(acquisitions))
	for _, acquisition := range acquisitions {
		if acquisition.Provider == "" || acquisition.RequestIdentity == "" || acquisition.RetrievedAt.IsZero() {
			return nil, fmt.Errorf("invalid acquisition")
		}
		hash := sha256.Sum256(acquisition.Response)
		var id int64
		if err := tx.QueryRowContext(ctx, insertAcquisitionSQL, network, acquisition.Provider, acquisition.RequestIdentity, fmt.Sprintf("%x", hash[:]), acquisition.Response, acquisition.RetrievedAt).Scan(&id); err != nil {
			return nil, fmt.Errorf("insert acquisition snapshot: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func execCypher(ctx context.Context, tx *sql.Tx, query string, parameters any) error {
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, string(encoded))
	return err
}
