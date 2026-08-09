package db

import (
	"context"
	"fmt"
	"time"
)

type Transfer struct {
	ID, Network, TransactionHash, FromAddress, ToAddress, AssetSymbol, AmountBaseUnits, Source string
	EventIndex                                                                                 uint32
	BlockNumber                                                                                int64
	BlockTimestamp, RetrievedAt                                                                time.Time
}

func (d *DB) SaveTransfers(ctx context.Context, transfers []Transfer) error {
	if len(transfers) == 0 {
		return nil
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transfer transaction: %w", err)
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO transfers (id, network, transaction_hash, event_index, from_address, to_address, asset_symbol, amount_base_units, block_number, block_timestamp, source, retrieved_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) ON CONFLICT (id) DO NOTHING`)
	if err != nil {
		return fmt.Errorf("prepare transfer insert: %w", err)
	}
	defer statement.Close()
	for _, transfer := range transfers {
		if _, err := statement.ExecContext(ctx, transfer.ID, transfer.Network, transfer.TransactionHash, transfer.EventIndex, transfer.FromAddress, transfer.ToAddress, transfer.AssetSymbol, transfer.AmountBaseUnits, transfer.BlockNumber, transfer.BlockTimestamp, transfer.Source, transfer.RetrievedAt); err != nil {
			return fmt.Errorf("insert transfer: %w", err)
		}
	}
	return tx.Commit()
}
