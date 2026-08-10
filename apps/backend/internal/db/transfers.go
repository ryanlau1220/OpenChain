package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const graphName = "openchain"

type Transfer struct {
	ID, Network, TransactionHash, FromAddress, ToAddress, AssetSymbol, AmountBaseUnits, Source string
	EventIndex                                                                                 uint32
	BlockNumber                                                                                int64
	BlockTimestamp, RetrievedAt                                                                time.Time
}

type Address struct {
	Address, Label, EntityType string
}

const insertTransferSQL = `INSERT INTO transfers (id, network, transaction_hash, event_index, from_address, to_address, asset_symbol, amount_base_units, block_number, block_timestamp, source, retrieved_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`

const upsertAddressSQL = `SELECT * FROM cypher('openchain', $$
MERGE (address:Address {address: $address})
SET address.label = $label, address.entity_type = $entity_type
RETURN address
$$, $1) AS (result agtype)`

const upsertFundFlowSQL = `SELECT * FROM cypher('openchain', $$
MATCH (from:Address {address: $from_address}), (to:Address {address: $to_address})
MERGE (from)-[flow:FundFlow {id: $id}]->(to)
SET flow.network = $network,
    flow.transaction_hash = $transaction_hash,
    flow.event_index = $event_index,
    flow.asset_symbol = $asset_symbol,
    flow.amount_base_units = $amount_base_units,
    flow.block_number = $block_number,
    flow.block_timestamp = $block_timestamp,
    flow.source = $source
RETURN flow
$$, $1) AS (result agtype)`

func (d *DB) SaveGraph(ctx context.Context, addresses []Address, transfers []Transfer) error {
	if len(addresses) == 0 && len(transfers) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer transaction: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, insertTransferSQL)
	if err != nil {
		return fmt.Errorf("prepare transfer insert: %w", err)
	}
	defer statement.Close()
	for _, transfer := range transfers {
		if _, err := statement.ExecContext(ctx, transfer.ID, transfer.Network, transfer.TransactionHash, transfer.EventIndex, transfer.FromAddress, transfer.ToAddress, transfer.AssetSymbol, transfer.AmountBaseUnits, transfer.BlockNumber, transfer.BlockTimestamp, transfer.Source, transfer.RetrievedAt); err != nil {
			return fmt.Errorf("insert transfer: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		return fmt.Errorf("load Apache AGE: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = ag_catalog, "$user", public`); err != nil {
		return fmt.Errorf("set Apache AGE search path: %w", err)
	}
	for _, address := range addresses {
		if err := execCypher(ctx, tx, upsertAddressSQL, map[string]string{"address": address.Address, "label": address.Label, "entity_type": address.EntityType}); err != nil {
			return fmt.Errorf("upsert address graph node: %w", err)
		}
	}
	for _, transfer := range transfers {
		if err := execCypher(ctx, tx, upsertFundFlowSQL, map[string]any{
			"id": transfer.ID, "network": transfer.Network, "transaction_hash": transfer.TransactionHash,
			"event_index": transfer.EventIndex, "from_address": transfer.FromAddress, "to_address": transfer.ToAddress,
			"asset_symbol": transfer.AssetSymbol, "amount_base_units": transfer.AmountBaseUnits,
			"block_number": transfer.BlockNumber, "block_timestamp": transfer.BlockTimestamp.Unix(), "source": transfer.Source,
		}); err != nil {
			return fmt.Errorf("upsert fund flow graph edge: %w", err)
		}
	}
	return tx.Commit()
}

func execCypher(ctx context.Context, tx *sql.Tx, query string, parameters any) error {
	encoded, err := json.Marshal(parameters)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, query, string(encoded))
	return err
}
