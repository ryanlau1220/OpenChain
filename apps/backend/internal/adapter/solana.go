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
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	SolanaRPCSource     = "solana-rpc"
	HeliusHistorySource = "helius-enhanced-transactions"
	solanaRequestGap    = time.Second / 5
	solanaSignaturePage = 50
)

type SolanaAdapter struct {
	network     string
	rpcURL      string
	historyURL  string
	historyKey  string
	httpClient  *http.Client
	requestMu   sync.Mutex
	lastRequest time.Time
	metrics     *providerMetrics
}

func NewSolanaAdapter(network, rpcURL string) *SolanaAdapter {
	historyURL, historyKey := heliusHistoryConfig(rpcURL)
	return &SolanaAdapter{network: network, rpcURL: rpcURL, historyURL: historyURL, historyKey: historyKey, httpClient: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(HeliusHistorySource, int(time.Second/solanaRequestGap))}
}

func heliusHistoryConfig(rpcURL string) (string, string) {
	parsed, err := url.Parse(rpcURL)
	if err != nil || !strings.HasSuffix(strings.ToLower(parsed.Hostname()), ".helius-rpc.com") {
		return "", ""
	}
	key := parsed.Query().Get("api-key")
	if key == "" {
		return "", ""
	}
	return "https://api-mainnet.helius-rpc.com", key
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

type heliusTransaction struct {
	Signature       string `json:"signature"`
	Slot            int64  `json:"slot"`
	Timestamp       int64  `json:"timestamp"`
	NativeTransfers []struct {
		FromUserAccount string          `json:"fromUserAccount"`
		ToUserAccount   string          `json:"toUserAccount"`
		Amount          json.RawMessage `json:"amount"`
	} `json:"nativeTransfers"`
	TokenTransfers []struct {
		FromUserAccount string          `json:"fromUserAccount"`
		ToUserAccount   string          `json:"toUserAccount"`
		Mint            string          `json:"mint"`
		TokenAmount     json.RawMessage `json:"tokenAmount"`
	} `json:"tokenTransfers"`
	AccountData []struct {
		TokenBalanceChanges []struct {
			Mint           string `json:"mint"`
			RawTokenAmount struct {
				Decimals uint32 `json:"decimals"`
			} `json:"rawTokenAmount"`
		} `json:"tokenBalanceChanges"`
	} `json:"accountData"`
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
		transaction, err := a.historyTransaction(ctx, state.PendingSignature)
		if err != nil {
			return nil, err
		}
		items, err := transfersForHeliusTransaction(transaction, address)
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
	transactions, err := a.history(ctx, address, state.Before, solanaSignaturePage)
	if err != nil {
		return nil, err
	}
	for _, transaction := range transactions {
		if transaction.Signature == "" {
			return nil, fmt.Errorf("parse Solana history transaction")
		}
		state.Before = transaction.Signature
		items, err := transfersForHeliusTransaction(&transaction, address)
		if err != nil {
			return nil, err
		}
		before := len(transfers)
		transfers, consumed := appendLimited(transfers, items, limit)
		if consumed < len(items) {
			state.PendingSignature, state.PendingOffset = transaction.Signature, consumed
			return a.transferPage(transfers, state), nil
		}
		if len(transfers) > before && uint32(len(transfers)) >= limit {
			return a.transferPage(transfers, state), nil
		}
	}
	if len(transactions) == solanaSignaturePage && state.Before != "" {
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

func (a *SolanaAdapter) history(ctx context.Context, address, before string, limit int) ([]heliusTransaction, error) {
	if a.historyURL == "" || a.historyKey == "" {
		return nil, fmt.Errorf("Solana indexed history requires a Helius RPC URL with an api-key")
	}
	endpoint, err := url.Parse(a.historyURL + "/v0/addresses/" + url.PathEscape(address) + "/transactions")
	if err != nil {
		return nil, fmt.Errorf("create Solana history request")
	}
	query := endpoint.Query()
	query.Set("api-key", a.historyKey)
	query.Set("limit", fmt.Sprintf("%d", limit))
	query.Set("token-accounts", "balanceChanged")
	if before != "" {
		query.Set("before-signature", before)
	}
	endpoint.RawQuery = query.Encode()
	var transactions []heliusTransaction
	if err := a.historyRequest(ctx, http.MethodGet, endpoint.String(), nil, &transactions); err != nil {
		return nil, err
	}
	return transactions, nil
}

func (a *SolanaAdapter) historyTransaction(ctx context.Context, signature string) (*heliusTransaction, error) {
	if a.historyURL == "" || a.historyKey == "" {
		return nil, fmt.Errorf("Solana indexed history requires a Helius RPC URL with an api-key")
	}
	endpoint, err := url.Parse(a.historyURL + "/v0/transactions")
	if err != nil {
		return nil, fmt.Errorf("create Solana history request")
	}
	query := endpoint.Query()
	query.Set("api-key", a.historyKey)
	endpoint.RawQuery = query.Encode()
	var transactions []heliusTransaction
	if err := a.historyRequest(ctx, http.MethodPost, endpoint.String(), map[string][]string{"transactions": []string{signature}}, &transactions); err != nil {
		return nil, err
	}
	if len(transactions) == 0 {
		return nil, fmt.Errorf("Solana transaction is unavailable")
	}
	return &transactions[0], nil
}

func (a *SolanaAdapter) historyRequest(ctx context.Context, method, endpoint string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	if err := a.waitForRequest(ctx); err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return fmt.Errorf("create Solana history request")
	}
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(HeliusHistorySource, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read Solana history response: %w", err)
	}
	recordAcquisition(ctx, HeliusHistorySource, request, responseBody)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(HeliusHistorySource, response)
	}
	if err := json.Unmarshal(responseBody, output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Solana history response: %w", err)
	}
	a.metrics.success()
	return nil
}

func transfersForHeliusTransaction(transaction *heliusTransaction, address string) ([]TransferItem, error) {
	if transaction == nil || transaction.Signature == "" {
		return nil, nil
	}
	decimalsByMint := map[string]uint32{}
	for _, account := range transaction.AccountData {
		for _, change := range account.TokenBalanceChanges {
			if change.Mint != "" {
				decimalsByMint[change.Mint] = change.RawTokenAmount.Decimals
			}
		}
	}
	timestamp := time.Unix(transaction.Timestamp, 0).UTC()
	transfers := make([]TransferItem, 0, len(transaction.NativeTransfers)+len(transaction.TokenTransfers))
	for index, transfer := range transaction.NativeTransfers {
		if transfer.FromUserAccount != address && transfer.ToUserAccount != address {
			continue
		}
		amount, err := rawInteger(transfer.Amount)
		if err != nil || transfer.FromUserAccount == "" || transfer.ToUserAccount == "" {
			continue
		}
		transfers = append(transfers, TransferItem{Hash: transaction.Signature, EventID: fmt.Sprintf("native:%d", index), TransferKind: "NATIVE", From: transfer.FromUserAccount, To: transfer.ToUserAccount, AmountBaseUnits: amount, Asset: Asset{Kind: "NATIVE", Symbol: "SOL", Decimals: 9}, BlockNumber: transaction.Slot, Timestamp: timestamp})
	}
	for index, transfer := range transaction.TokenTransfers {
		if transfer.FromUserAccount != address && transfer.ToUserAccount != address {
			continue
		}
		amount, err := rawInteger(transfer.TokenAmount)
		if err != nil || transfer.FromUserAccount == "" || transfer.ToUserAccount == "" || transfer.Mint == "" {
			continue
		}
		transfers = append(transfers, TransferItem{Hash: transaction.Signature, EventID: fmt.Sprintf("spl:%d", index), TransferKind: "SPL", From: transfer.FromUserAccount, To: transfer.ToUserAccount, AmountBaseUnits: amount, Asset: Asset{Kind: "SPL", ContractAddress: transfer.Mint, Decimals: decimalsByMint[transfer.Mint]}, BlockNumber: transaction.Slot, Timestamp: timestamp})
	}
	return transfers, nil
}

func rawInteger(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
	}
	if _, ok := new(big.Int).SetString(value, 10); !ok {
		return "", fmt.Errorf("invalid integer")
	}
	return value, nil
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
	return SourceStatus{Source: HeliusHistorySource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}

func (a *SolanaAdapter) call(ctx context.Context, method string, params []any, output any) error {
	if err := a.waitForRequest(ctx); err != nil {
		return err
	}
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
		a.metrics.failure()
		return NewProviderTransportError(HeliusHistorySource, err)
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read Solana RPC response: %w", err)
	}
	recordAcquisition(ctx, HeliusHistorySource, request, responseBody)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(HeliusHistorySource, response)
	}
	var envelope struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Solana RPC response: %w", err)
	}
	if envelope.Error != nil {
		a.metrics.failure()
		return fmt.Errorf("Solana RPC error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if err := json.Unmarshal(envelope.Result, output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Solana RPC result: %w", err)
	}
	a.metrics.success()
	return nil
}

func (a *SolanaAdapter) waitForRequest(ctx context.Context) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(solanaRequestGap))
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	a.metrics.request(delay)
	a.lastRequest = time.Now()
	return nil
}

func (a *SolanaAdapter) ProviderHealth() []ProviderHealth {
	return []ProviderHealth{a.metrics.snapshot()}
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
