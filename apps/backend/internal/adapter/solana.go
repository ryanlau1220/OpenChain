package adapter

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	SolanaRPCSource     = "solana-rpc"
	solanaRequestGap    = time.Second / 5
	solanaSignaturePage = 50
)

type SolanaAdapter struct {
	network     string
	rpcURL      string
	httpClient  *http.Client
	requestMu   sync.Mutex
	lastRequest time.Time
}

func NewSolanaAdapter(network, rpcURL string) *SolanaAdapter {
	return &SolanaAdapter{network: network, rpcURL: rpcURL, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

func (a *SolanaAdapter) Network() string { return a.network }

func (a *SolanaAdapter) NormalizeAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	decoded, err := decodeBase58(value)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("expected a Solana address")
	}
	return value, nil
}

func (a *SolanaAdapter) NormalizeTransactionHash(value string) (string, error) {
	value = strings.TrimSpace(value)
	decoded, err := decodeBase58(value)
	if err != nil || len(decoded) != 64 {
		return "", fmt.Errorf("expected a Solana transaction signature")
	}
	return value, nil
}

func (a *SolanaAdapter) NativeAsset() Asset { return Asset{Kind: "NATIVE", Symbol: "SOL", Decimals: 9} }

func (a *SolanaAdapter) ActivityLabel() string { return "" }

func (a *SolanaAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	var result struct {
		Value json.Number `json:"value"`
	}
	if err := a.call(ctx, "getBalance", []any{address, map[string]any{"commitment": "confirmed"}}, &result); err != nil {
		return nil, err
	}
	balance, ok := new(big.Int).SetString(result.Value.String(), 10)
	if !ok {
		return nil, fmt.Errorf("parse Solana balance")
	}
	return balance, nil
}

func (a *SolanaAdapter) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }

func (a *SolanaAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	var result struct {
		Value *struct {
			Executable bool `json:"executable"`
		} `json:"value"`
	}
	if err := a.call(ctx, "getAccountInfo", []any{address, map[string]any{"encoding": "jsonParsed", "commitment": "confirmed"}}, &result); err != nil {
		return false, err
	}
	return result.Value != nil && result.Value.Executable, nil
}

type solanaCursor struct {
	Before           string `json:"before,omitempty"`
	PendingSignature string `json:"pending_signature,omitempty"`
	PendingOffset    int    `json:"pending_offset,omitempty"`
}

type solanaSignature struct {
	Signature string `json:"signature"`
	Slot      int64  `json:"slot"`
	BlockTime *int64 `json:"blockTime"`
	Err       any    `json:"err"`
}

func (a *SolanaAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	state, err := decodeSolanaCursor(cursor)
	if err != nil {
		return nil, err
	}
	transfers := make([]TransferItem, 0, limit)
	if state.PendingSignature != "" {
		items, err := a.transfersForSignature(ctx, state.PendingSignature, address)
		if err != nil {
			return nil, err
		}
		if state.PendingOffset > len(items) {
			return nil, fmt.Errorf("invalid cursor")
		}
		var consumed int
		transfers, consumed = appendLimited(transfers, items[state.PendingOffset:], limit)
		state.PendingOffset += consumed
		if state.PendingOffset < len(items) {
			return a.transferPage(transfers, state), nil
		}
		state.Before, state.PendingSignature, state.PendingOffset = state.PendingSignature, "", 0
	}
	params := map[string]any{"limit": solanaSignaturePage, "commitment": "confirmed"}
	if state.Before != "" {
		params["before"] = state.Before
	}
	var signatures []solanaSignature
	if err := a.call(ctx, "getSignaturesForAddress", []any{address, params}, &signatures); err != nil {
		return nil, err
	}
	for _, signature := range signatures {
		state.Before = signature.Signature
		if signature.Err != nil {
			continue
		}
		items, err := a.transfersForSignature(ctx, signature.Signature, address)
		if err != nil {
			return nil, err
		}
		before := len(transfers)
		transfers, consumed := appendLimited(transfers, items, limit)
		if consumed < len(items) {
			state.PendingSignature, state.PendingOffset = signature.Signature, consumed
			return a.transferPage(transfers, state), nil
		}
		if len(transfers) > before && uint32(len(transfers)) >= limit {
			return a.transferPage(transfers, state), nil
		}
	}
	if len(signatures) == solanaSignaturePage && state.Before != "" {
		return a.transferPage(transfers, state), nil
	}
	return &TransferPage{Transfers: transfers, SourceStatus: a.SourceStatus()}, nil
}

func appendLimited(existing, items []TransferItem, limit uint32) ([]TransferItem, int) {
	remaining := int(limit) - len(existing)
	if remaining <= 0 {
		return existing, 0
	}
	if len(items) <= remaining {
		return append(existing, items...), len(items)
	}
	return append(existing, items[:remaining]...), remaining
}

func (a *SolanaAdapter) transferPage(transfers []TransferItem, cursor solanaCursor) *TransferPage {
	next, err := encodeSolanaCursor(cursor)
	if err != nil {
		return &TransferPage{Transfers: transfers, SourceStatus: a.SourceStatus()}
	}
	return &TransferPage{Transfers: transfers, NextCursor: next, HasMore: next != "", SourceStatus: a.SourceStatus()}
}

type solanaTransaction struct {
	Slot        int64  `json:"slot"`
	BlockTime   *int64 `json:"blockTime"`
	Transaction struct {
		Message struct {
			Instructions []struct {
				Program string `json:"program"`
				Parsed  *struct {
					Type string          `json:"type"`
					Info json.RawMessage `json:"info"`
				} `json:"parsed"`
			} `json:"instructions"`
		} `json:"message"`
	} `json:"transaction"`
}

func (a *SolanaAdapter) transfersForSignature(ctx context.Context, signature, address string) ([]TransferItem, error) {
	var transaction *solanaTransaction
	if err := a.call(ctx, "getTransaction", []any{signature, map[string]any{"commitment": "confirmed", "encoding": "jsonParsed", "maxSupportedTransactionVersion": 0}}, &transaction); err != nil {
		return nil, err
	}
	if transaction == nil {
		return nil, nil
	}
	timestamp := time.Now().UTC()
	if transaction.BlockTime != nil {
		timestamp = time.Unix(*transaction.BlockTime, 0).UTC()
	}
	transfers := make([]TransferItem, 0)
	for index, instruction := range transaction.Transaction.Message.Instructions {
		if instruction.Program != "system" || instruction.Parsed == nil || instruction.Parsed.Type != "transfer" {
			continue
		}
		var info struct {
			Source      string      `json:"source"`
			Destination string      `json:"destination"`
			Lamports    json.Number `json:"lamports"`
		}
		decoder := json.NewDecoder(bytes.NewReader(instruction.Parsed.Info))
		decoder.UseNumber()
		if err := decoder.Decode(&info); err != nil {
			return nil, fmt.Errorf("decode Solana transfer: %w", err)
		}
		if info.Source != address && info.Destination != address {
			continue
		}
		if _, ok := new(big.Int).SetString(info.Lamports.String(), 10); !ok || info.Source == "" || info.Destination == "" {
			return nil, fmt.Errorf("parse Solana transfer")
		}
		transfers = append(transfers, TransferItem{Hash: signature, EventID: fmt.Sprintf("instruction:%d", index), TransferKind: "NATIVE", From: info.Source, To: info.Destination, AmountBaseUnits: info.Lamports.String(), Asset: a.NativeAsset(), BlockNumber: transaction.Slot, Timestamp: timestamp})
	}
	return transfers, nil
}

func (a *SolanaAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var transaction *solanaTransaction
	if err := a.call(ctx, "getTransaction", []any{hash, map[string]any{"commitment": "confirmed", "encoding": "jsonParsed", "maxSupportedTransactionVersion": 0}}, &transaction); err != nil {
		return nil, SourceStatus{}, err
	}
	if transaction == nil {
		return nil, SourceStatus{}, fmt.Errorf("Solana transaction is unavailable")
	}
	timestamp := time.Time{}
	if transaction.BlockTime != nil {
		timestamp = time.Unix(*transaction.BlockTime, 0).UTC()
	}
	return &TransactionItem{Hash: hash, BlockNumber: transaction.Slot, Timestamp: timestamp, AssetSymbol: "SOL"}, a.SourceStatus(), nil
}

func (a *SolanaAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	isProgram, err := a.IsContract(ctx, address)
	if err != nil {
		return nil, err
	}
	if isProgram {
		return &ContractMetadata{Category: "CONTRACT"}, nil
	}
	return &ContractMetadata{Category: "EOA"}, nil
}

func (a *SolanaAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: SolanaRPCSource, RetrievedAt: time.Now().UTC(), IsComplete: false, Warning: "Showing confirmed native SOL transfers only; SPL token and program movements are not included."}
}

func (a *SolanaAdapter) call(ctx context.Context, method string, params []any, output any) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	if delay := time.Until(a.lastRequest.Add(solanaRequestGap)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	a.lastRequest = time.Now()
	body, err := json.Marshal(struct {
		JSONRPC string `json:"jsonrpc"`
		ID      int    `json:"id"`
		Method  string `json:"method"`
		Params  []any  `json:"params"`
	}{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.rpcURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("Solana RPC request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Solana RPC returned %s", response.Status)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(&envelope); err != nil {
		return fmt.Errorf("decode Solana RPC response: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("Solana RPC error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if err := json.Unmarshal(envelope.Result, output); err != nil {
		return fmt.Errorf("decode Solana RPC result: %w", err)
	}
	return nil
}

func decodeSolanaCursor(cursor string) (solanaCursor, error) {
	if cursor == "" {
		return solanaCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return solanaCursor{}, fmt.Errorf("invalid cursor")
	}
	var result solanaCursor
	if err := json.Unmarshal(decoded, &result); err != nil {
		return solanaCursor{}, fmt.Errorf("invalid cursor")
	}
	if result.PendingOffset < 0 {
		return solanaCursor{}, fmt.Errorf("invalid cursor")
	}
	return result, nil
}

func encodeSolanaCursor(cursor solanaCursor) (string, error) {
	if cursor.Before == "" && cursor.PendingSignature == "" {
		return "", nil
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

var _ ChainAdapter = (*SolanaAdapter)(nil)
