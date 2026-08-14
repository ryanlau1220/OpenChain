package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
)

const maxGraphEdgesPerAddress = 50

const ageOutgoingEdgesSQL = `SELECT id, from_address, to_address, transaction_hash, event_id, transfer_kind, asset_kind, asset_contract_address, asset_decimals, asset_symbol, amount_base_units, block_number, block_hash, block_timestamp, provisional, source_name, retrieved_at
FROM cypher('openchain', $$
MATCH (from:Address {network: $network, address: $address})-[flow:FundFlow]->(to:Address {network: $network})
RETURN flow.id, from.address, to.address, flow.transaction_hash, flow.event_id, flow.transfer_kind, flow.asset_kind, flow.asset_contract_address, flow.asset_decimals, flow.asset_symbol, flow.amount_base_units, flow.block_number, flow.block_hash, flow.block_timestamp, flow.provisional, flow.source, flow.retrieved_at
$$, $1) AS (id agtype, from_address agtype, to_address agtype, transaction_hash agtype, event_id agtype, transfer_kind agtype, asset_kind agtype, asset_contract_address agtype, asset_decimals agtype, asset_symbol agtype, amount_base_units agtype, block_number agtype, block_hash agtype, block_timestamp agtype, provisional agtype, source_name agtype, retrieved_at agtype)
ORDER BY block_timestamp DESC, id ASC LIMIT $2`

const ageIncomingEdgesSQL = `SELECT id, from_address, to_address, transaction_hash, event_id, transfer_kind, asset_kind, asset_contract_address, asset_decimals, asset_symbol, amount_base_units, block_number, block_hash, block_timestamp, provisional, source_name, retrieved_at
FROM cypher('openchain', $$
MATCH (from:Address {network: $network})-[flow:FundFlow]->(to:Address {network: $network, address: $address})
RETURN flow.id, from.address, to.address, flow.transaction_hash, flow.event_id, flow.transfer_kind, flow.asset_kind, flow.asset_contract_address, flow.asset_decimals, flow.asset_symbol, flow.amount_base_units, flow.block_number, flow.block_hash, flow.block_timestamp, flow.provisional, flow.source, flow.retrieved_at
$$, $1) AS (id agtype, from_address agtype, to_address agtype, transaction_hash agtype, event_id agtype, transfer_kind agtype, asset_kind agtype, asset_contract_address agtype, asset_decimals agtype, asset_symbol agtype, amount_base_units agtype, block_number agtype, block_hash agtype, block_timestamp agtype, provisional agtype, source_name agtype, retrieved_at agtype)
ORDER BY block_timestamp DESC, id ASC LIMIT $2`

const ageBothEdgesSQL = `SELECT id, from_address, to_address, transaction_hash, event_id, transfer_kind, asset_kind, asset_contract_address, asset_decimals, asset_symbol, amount_base_units, block_number, block_hash, block_timestamp, provisional, source_name, retrieved_at
FROM cypher('openchain', $$
MATCH (from:Address {network: $network})-[flow:FundFlow]->(to:Address {network: $network})
WHERE from.address = $address OR to.address = $address
RETURN flow.id, from.address, to.address, flow.transaction_hash, flow.event_id, flow.transfer_kind, flow.asset_kind, flow.asset_contract_address, flow.asset_decimals, flow.asset_symbol, flow.amount_base_units, flow.block_number, flow.block_hash, flow.block_timestamp, flow.provisional, flow.source, flow.retrieved_at
$$, $1) AS (id agtype, from_address agtype, to_address agtype, transaction_hash agtype, event_id agtype, transfer_kind agtype, asset_kind agtype, asset_contract_address agtype, asset_decimals agtype, asset_symbol agtype, amount_base_units agtype, block_number agtype, block_hash agtype, block_timestamp agtype, provisional agtype, source_name agtype, retrieved_at agtype)
ORDER BY block_timestamp DESC, id ASC LIMIT $2`

// GraphNeighbors reads a bounded AGE adjacency list. PostgreSQL transfers and
// acquisition records remain the evidence authority; AGE only supplies graph
// topology after those facts have committed in the same SaveEvidenceGraph call.
func (d *DB) GraphNeighbors(ctx context.Context, network, address, direction string, limit int) ([]Transfer, error) {
	if network == "" || address == "" {
		return nil, fmt.Errorf("network and address are required")
	}
	if limit <= 0 || limit > maxGraphEdgesPerAddress {
		limit = maxGraphEdgesPerAddress
	}
	query, err := ageNeighborQuery(direction)
	if err != nil {
		return nil, err
	}
	parameters, err := json.Marshal(map[string]string{"network": network, "address": address})
	if err != nil {
		return nil, fmt.Errorf("encode AGE parameters: %w", err)
	}
	tx, err := d.SQL.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("begin AGE read transaction: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		return nil, fmt.Errorf("load Apache AGE: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = ag_catalog, "$user", public`); err != nil {
		return nil, fmt.Errorf("set Apache AGE search path: %w", err)
	}
	rows, err := tx.QueryContext(ctx, query, string(parameters), limit)
	if err != nil {
		return nil, fmt.Errorf("query Apache AGE graph: %w", err)
	}
	defer rows.Close()
	transfers := make([]Transfer, 0, limit)
	for rows.Next() {
		transfer, err := scanAGETransfer(rows, network)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, transfer)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate Apache AGE graph: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit AGE read transaction: %w", err)
	}
	return transfers, nil
}

func ageNeighborQuery(direction string) (string, error) {
	switch direction {
	case "both":
		return ageBothEdgesSQL, nil
	case "inbound":
		return ageIncomingEdgesSQL, nil
	case "outbound":
		return ageOutgoingEdgesSQL, nil
	default:
		return "", fmt.Errorf("invalid graph direction")
	}
}

type ageTransferScanner interface{ Scan(...any) error }

func scanAGETransfer(row ageTransferScanner, network string) (Transfer, error) {
	var fields [17]string
	values := make([]any, len(fields))
	for index := range fields {
		values[index] = &fields[index]
	}
	if err := row.Scan(values...); err != nil {
		return Transfer{}, fmt.Errorf("scan Apache AGE graph edge: %w", err)
	}
	strings := make([]string, len(fields))
	for index, value := range fields {
		parsed, err := ageString(value)
		if err != nil {
			return Transfer{}, err
		}
		strings[index] = parsed
	}
	decimals, err := ageInt(fields[8])
	if err != nil {
		return Transfer{}, err
	}
	blockNumber, err := ageInt64(fields[11])
	if err != nil {
		return Transfer{}, err
	}
	blockTimestamp, err := ageInt64(fields[13])
	if err != nil {
		return Transfer{}, err
	}
	retrievedAt, err := ageInt64(fields[16])
	if err != nil {
		return Transfer{}, err
	}
	provisional, err := ageBool(fields[14])
	if err != nil {
		return Transfer{}, err
	}
	return Transfer{
		ID: strings[0], Network: network, FromAddress: strings[1], ToAddress: strings[2], TransactionHash: strings[3], EventID: strings[4], TransferKind: strings[5],
		Asset: adapter.Asset{Kind: strings[6], ContractAddress: strings[7], Decimals: uint32(decimals), Symbol: strings[9]}, AmountBaseUnits: strings[10], BlockNumber: blockNumber,
		BlockHash: strings[12], BlockTimestamp: unixTime(blockTimestamp), Provisional: provisional, Source: strings[15], RetrievedAt: unixTime(retrievedAt),
	}, nil
}

func ageString(raw string) (string, error) {
	if raw == "" || raw == "null" {
		return "", nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", fmt.Errorf("decode Apache AGE value: %w", err)
	}
	switch value := value.(type) {
	case string:
		return value, nil
	case float64:
		return strconv.FormatInt(int64(value), 10), nil
	case bool:
		return strconv.FormatBool(value), nil
	default:
		return "", fmt.Errorf("unexpected Apache AGE value")
	}
}

func ageInt(raw string) (int, error) {
	value, err := ageInt64(raw)
	return int(value), err
}

func ageInt64(raw string) (int64, error) {
	value, err := ageString(raw)
	if err != nil || value == "" {
		return 0, err
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("decode Apache AGE integer: %w", err)
	}
	return parsed, nil
}

func ageBool(raw string) (bool, error) {
	value, err := ageString(raw)
	if err != nil || value == "" {
		return true, err
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return true, fmt.Errorf("decode Apache AGE boolean: %w", err)
	}
	return parsed, nil
}

func unixTime(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(value, 0).UTC()
}
