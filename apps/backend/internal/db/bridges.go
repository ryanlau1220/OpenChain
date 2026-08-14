package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/bridge"
)

// BridgeTransition is an immutable observation of one bridge-message lifecycle
// state. A later relay/finalization becomes a new observation instead of
// rewriting earlier evidence.
type BridgeTransition struct {
	ID, Protocol, BridgeName, SourceNetwork, DestinationNetwork, Lifecycle, MessageID string
	SourceTransferID, DestinationTransferID                                           string
	SourceTransactionHash, DestinationTransactionHash                                 string
	SourceLogReference, DestinationLogReference                                       string
	SourceBridgeAddress, DestinationBridgeAddress                                     string
	CanonicalSourceToken, CanonicalDestinationToken, Recipient, AmountBaseUnits       string
	Asset                                                                             adapter.Asset
	SourceBlockNumber, DestinationBlockNumber                                         uint64
	SourceBlockHash, DestinationBlockHash                                             string
	SourceTimestamp, DestinationTimestamp                                             time.Time
	SourceConfirmed, DestinationConfirmed                                             bool
	Limitations                                                                       string
	SourceAcquisitionIDs, DestinationAcquisitionIDs                                   []int64
}

type BridgeTransitionAcquisition struct {
	TransitionID  string
	Side          string
	AcquisitionID int64
}

const insertBridgeTransitionSQL = `INSERT INTO bridge_transitions (
 id, protocol, bridge_name, source_network, destination_network, lifecycle, message_id,
 source_transfer_id, destination_transfer_id, source_transaction_hash, destination_transaction_hash,
 source_log_reference, destination_log_reference, source_bridge_address, destination_bridge_address,
 canonical_source_token, canonical_destination_token, recipient, asset_kind, asset_contract_address,
 asset_symbol, asset_decimals, amount_base_units, source_block_number, destination_block_number,
 source_block_hash, destination_block_hash, source_timestamp, destination_timestamp,
 source_confirmed, destination_confirmed, limitations, observed_at
) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,''),$10,NULLIF($11,''),$12,NULLIF($13,''),$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,NULLIF($25,0),$26,NULLIF($27,''),$28,$29,$30,$31,$32,$33) ON CONFLICT (id) DO NOTHING`

const insertBridgeTransitionAcquisitionSQL = `INSERT INTO bridge_transition_acquisitions (transition_id, side, acquisition_id) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`

func insertBridgeTransitions(ctx context.Context, tx *sql.Tx, transitions []bridge.Transition, snapshotsByHash map[string][]int64) error {
	if len(transitions) == 0 {
		return nil
	}
	for _, transition := range transitions {
		if transition.ID == "" || transition.Protocol == "" || transition.MessageID == "" && transition.Lifecycle != bridge.LifecycleUnresolved {
			return fmt.Errorf("invalid bridge transition")
		}
		if _, err := tx.ExecContext(ctx, insertBridgeTransitionSQL,
			transition.ID, transition.Protocol, transition.BridgeName, transition.SourceNetwork, transition.DestinationNetwork, string(transition.Lifecycle), transition.MessageID,
			transition.SourceTransferID, transition.DestinationTransferID, transition.SourceTransactionHash, transition.DestinationTransactionHash,
			transition.SourceLogReference, transition.DestinationLogReference, transition.SourceBridgeAddress, transition.DestinationBridgeAddress,
			transition.CanonicalSourceToken, transition.CanonicalDestinationToken, transition.Recipient, transition.Asset.Kind, transition.Asset.ContractAddress,
			transition.Asset.Symbol, transition.Asset.Decimals, transition.AmountBaseUnits, transition.SourceBlockNumber, transition.DestinationBlockNumber,
			transition.SourceBlockHash, transition.DestinationBlockHash, nullableBridgeTime(transition.SourceTimestamp), nullableBridgeTime(transition.DestinationTimestamp),
			transition.SourceConfirmed, transition.DestinationConfirmed, transition.Limitations, time.Now().UTC(),
		); err != nil {
			return fmt.Errorf("insert bridge transition: %w", err)
		}
		if err := linkBridgeAcquisitions(ctx, tx, transition.ID, "source", transition.SourceAcquisitionHashes, snapshotsByHash); err != nil {
			return err
		}
		if err := linkBridgeAcquisitions(ctx, tx, transition.ID, "destination", transition.DestinationAcquisitionHashes, snapshotsByHash); err != nil {
			return err
		}
	}
	return nil
}

func linkBridgeAcquisitions(ctx context.Context, tx *sql.Tx, transitionID, side string, hashes []string, snapshotsByHash map[string][]int64) error {
	for _, hash := range hashes {
		for _, snapshotID := range snapshotsByHash[hash] {
			if _, err := tx.ExecContext(ctx, insertBridgeTransitionAcquisitionSQL, transitionID, side, snapshotID); err != nil {
				return fmt.Errorf("link bridge acquisition: %w", err)
			}
		}
	}
	return nil
}

func nullableBridgeTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func (d *DB) ExportBridgeTransitions(ctx context.Context, transferIDs []string) ([]BridgeTransition, []BridgeTransitionAcquisition, error) {
	if len(transferIDs) == 0 {
		return []BridgeTransition{}, []BridgeTransitionAcquisition{}, nil
	}
	rows, err := d.SQL.QueryContext(ctx, `SELECT id, protocol, bridge_name, source_network, destination_network, lifecycle, message_id,
 source_transfer_id, COALESCE(destination_transfer_id, ''), source_transaction_hash, COALESCE(destination_transaction_hash, ''),
 source_log_reference, COALESCE(destination_log_reference, ''), source_bridge_address, destination_bridge_address,
 canonical_source_token, canonical_destination_token, recipient, asset_kind, asset_contract_address, asset_symbol, asset_decimals,
 amount_base_units, source_block_number, COALESCE(destination_block_number, 0), source_block_hash, COALESCE(destination_block_hash, ''),
 source_timestamp, destination_timestamp, source_confirmed, destination_confirmed, limitations
 FROM bridge_transitions WHERE source_transfer_id = ANY($1) ORDER BY source_timestamp, id`, pq.Array(transferIDs))
	if err != nil {
		return nil, nil, fmt.Errorf("query bridge transitions: %w", err)
	}
	defer rows.Close()
	transitions := make([]BridgeTransition, 0)
	ids := make([]string, 0)
	for rows.Next() {
		var item BridgeTransition
		var destinationTimestamp sql.NullTime
		if err := rows.Scan(&item.ID, &item.Protocol, &item.BridgeName, &item.SourceNetwork, &item.DestinationNetwork, &item.Lifecycle, &item.MessageID,
			&item.SourceTransferID, &item.DestinationTransferID, &item.SourceTransactionHash, &item.DestinationTransactionHash,
			&item.SourceLogReference, &item.DestinationLogReference, &item.SourceBridgeAddress, &item.DestinationBridgeAddress,
			&item.CanonicalSourceToken, &item.CanonicalDestinationToken, &item.Recipient, &item.Asset.Kind, &item.Asset.ContractAddress, &item.Asset.Symbol, &item.Asset.Decimals,
			&item.AmountBaseUnits, &item.SourceBlockNumber, &item.DestinationBlockNumber, &item.SourceBlockHash, &item.DestinationBlockHash,
			&item.SourceTimestamp, &destinationTimestamp, &item.SourceConfirmed, &item.DestinationConfirmed, &item.Limitations); err != nil {
			return nil, nil, fmt.Errorf("scan bridge transition: %w", err)
		}
		if destinationTimestamp.Valid {
			item.DestinationTimestamp = destinationTimestamp.Time
		}
		transitions, ids = append(transitions, item), append(ids, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate bridge transitions: %w", err)
	}
	if len(ids) == 0 {
		return transitions, []BridgeTransitionAcquisition{}, nil
	}
	links, err := d.exportBridgeTransitionAcquisitions(ctx, ids)
	return transitions, links, err
}

func (d *DB) BridgeTransitionsForTransfers(ctx context.Context, transferIDs []string) ([]bridge.Transition, error) {
	items, links, err := d.ExportBridgeTransitions(ctx, transferIDs)
	if err != nil {
		return nil, err
	}
	result := make([]bridge.Transition, 0, len(items))
	for _, item := range items {
		result = append(result, bridgeTransitionFromDB(item, links))
	}
	return result, nil
}

func (d *DB) exportBridgeTransitionAcquisitions(ctx context.Context, ids []string) ([]BridgeTransitionAcquisition, error) {
	rows, err := d.SQL.QueryContext(ctx, `SELECT transition_id, side, acquisition_id FROM bridge_transition_acquisitions WHERE transition_id = ANY($1) ORDER BY transition_id, side, acquisition_id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("query bridge acquisition links: %w", err)
	}
	defer rows.Close()
	result := make([]BridgeTransitionAcquisition, 0)
	for rows.Next() {
		var item BridgeTransitionAcquisition
		if err := rows.Scan(&item.TransitionID, &item.Side, &item.AcquisitionID); err != nil {
			return nil, fmt.Errorf("scan bridge acquisition link: %w", err)
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate bridge acquisition links: %w", err)
	}
	return result, nil
}

func bridgeTransitionFromDB(value BridgeTransition, links []BridgeTransitionAcquisition) bridge.Transition {
	result := bridge.Transition{ID: value.ID, Protocol: value.Protocol, BridgeName: value.BridgeName, SourceNetwork: value.SourceNetwork, DestinationNetwork: value.DestinationNetwork, Lifecycle: bridge.Lifecycle(value.Lifecycle), MessageID: value.MessageID, SourceTransferID: value.SourceTransferID, DestinationTransferID: value.DestinationTransferID, SourceTransactionHash: value.SourceTransactionHash, DestinationTransactionHash: value.DestinationTransactionHash, SourceLogReference: value.SourceLogReference, DestinationLogReference: value.DestinationLogReference, SourceBridgeAddress: value.SourceBridgeAddress, DestinationBridgeAddress: value.DestinationBridgeAddress, CanonicalSourceToken: value.CanonicalSourceToken, CanonicalDestinationToken: value.CanonicalDestinationToken, Recipient: value.Recipient, AmountBaseUnits: value.AmountBaseUnits, Asset: value.Asset, SourceBlockNumber: value.SourceBlockNumber, DestinationBlockNumber: value.DestinationBlockNumber, SourceBlockHash: value.SourceBlockHash, DestinationBlockHash: value.DestinationBlockHash, SourceTimestamp: value.SourceTimestamp, DestinationTimestamp: value.DestinationTimestamp, SourceConfirmed: value.SourceConfirmed, DestinationConfirmed: value.DestinationConfirmed, Limitations: value.Limitations}
	for _, link := range links {
		if link.TransitionID != value.ID {
			continue
		}
		if link.Side == "source" {
			result.SourceAcquisitionIDs = append(result.SourceAcquisitionIDs, link.AcquisitionID)
		} else {
			result.DestinationAcquisitionIDs = append(result.DestinationAcquisitionIDs, link.AcquisitionID)
		}
	}
	return result
}
