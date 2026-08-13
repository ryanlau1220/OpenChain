package adapter

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	tonAPISource         = "tonapi"
	tonAPIURL            = "https://tonapi.io/v2"
	tonAPIRequestGap     = time.Second / 5
	blockfrostSource     = "blockfrost"
	blockfrostAPIURL     = "https://cardano-mainnet.blockfrost.io/api/v0"
	blockfrostRequestGap = time.Second / 5
)

type TONAdapter struct {
	network, apiURL, key string
	client               *http.Client
	requestMu            sync.Mutex
	lastRequest          time.Time
	metrics              *providerMetrics
}

func NewTONAdapter(network, key string) *TONAdapter {
	return &TONAdapter{network: network, apiURL: tonAPIURL, key: key, client: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(tonAPISource, int(time.Second/tonAPIRequestGap))}
}
func (a *TONAdapter) Network() string { return a.network }
func (a *TONAdapter) NormalizeAddress(v string) (string, error) {
	v = strings.TrimSpace(v)
	if strings.HasPrefix(v, "0:") && len(v) == 66 {
		if _, err := hex.DecodeString(v[2:]); err == nil {
			return strings.ToLower(v), nil
		}
	}
	if len(v) == 48 && (strings.HasPrefix(v, "EQ") || strings.HasPrefix(v, "UQ")) {
		if decoded, err := base64.RawURLEncoding.DecodeString(v); err == nil && len(decoded) == 36 && tonAddressChecksum(decoded[:34]) == [2]byte{decoded[34], decoded[35]} {
			return v, nil
		}
	}
	return "", fmt.Errorf("expected a TON address")
}
func (a *TONAdapter) NormalizeTransactionHash(v string) (string, error) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "0x")
	if len(v) != 64 {
		return "", fmt.Errorf("expected a TON transaction hash")
	}
	if decoded, err := hex.DecodeString(v); err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("expected a TON transaction hash")
	}
	return strings.ToLower(v), nil
}
func (a *TONAdapter) NativeAsset() Asset    { return Asset{Kind: "NATIVE", Symbol: "TON", Decimals: 9} }
func (a *TONAdapter) ActivityLabel() string { return "" }
func (a *TONAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: tonAPISource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}
func (a *TONAdapter) ProviderHealth() []ProviderHealth { return []ProviderHealth{a.metrics.snapshot()} }
func (a *TONAdapter) request(ctx context.Context, path string, out any) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(tonAPIRequestGap))
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	u := a.apiURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+a.key)
	a.metrics.request(delay)
	a.lastRequest = time.Now()
	res, err := a.client.Do(req)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(tonAPISource, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	recordAcquisition(ctx, tonAPISource, req, body)
	if res.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(tonAPISource, res)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode TON response: %w", err)
	}
	a.metrics.success()
	return nil
}
func (a *TONAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	var r struct {
		Balance string `json:"balance"`
	}
	if err := a.request(ctx, "/accounts/"+url.PathEscape(address), &r); err != nil {
		return nil, err
	}
	n, ok := new(big.Int).SetString(r.Balance, 10)
	if !ok {
		return nil, fmt.Errorf("parse TON balance")
	}
	return n, nil
}
func (a *TONAdapter) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }
func (a *TONAdapter) IsContract(context.Context, string) (bool, error)   { return false, nil }
func (a *TONAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	var r struct {
		Transactions []struct {
			Hash  string `json:"hash"`
			Utime int64  `json:"utime"`
			InMsg *struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
				Value       string `json:"value"`
			} `json:"in_msg"`
			OutMsgs []struct {
				Source      string `json:"source"`
				Destination string `json:"destination"`
				Value       string `json:"value"`
			} `json:"out_msgs"`
		} `json:"transactions"`
		NextFrom string `json:"next_from"`
	}
	query := url.Values{"limit": {strconv.Itoa(int(limit))}}
	if cursor != "" {
		query.Set("before_lt", cursor)
	}
	if err := a.request(ctx, "/blockchain/accounts/"+url.PathEscape(address)+"/transactions?"+query.Encode(), &r); err != nil {
		return nil, err
	}
	transfers := []TransferItem{}
	add := func(hash string, t int64, from, to, value, kind string) {
		if from == "" || to == "" || value == "" || (from != address && to != address) {
			return
		}
		if _, ok := new(big.Int).SetString(value, 10); !ok {
			return
		}
		transfers = append(transfers, TransferItem{Hash: strings.ToLower(hash), EventID: kind, TransferKind: "NATIVE", From: from, To: to, AmountBaseUnits: value, Asset: a.NativeAsset(), Timestamp: time.Unix(t, 0).UTC()})
	}
	for _, tx := range r.Transactions {
		if tx.InMsg != nil {
			add(tx.Hash, tx.Utime, tx.InMsg.Source, tx.InMsg.Destination, tx.InMsg.Value, "in")
		}
		for i, msg := range tx.OutMsgs {
			add(tx.Hash, tx.Utime, msg.Source, msg.Destination, msg.Value, "out:"+strconv.Itoa(i))
		}
	}
	// TonAPI returns the next account logical time. It is opaque to callers, but
	// is passed back as before_lt on the next page.
	return &TransferPage{Transfers: transfers, NextCursor: r.NextFrom, HasMore: r.NextFrom != "", SourceStatus: a.SourceStatus()}, nil
}
func (a *TONAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var r struct {
		Hash  string `json:"hash"`
		Utime int64  `json:"utime"`
	}
	if err := a.request(ctx, "/blockchain/transactions/"+hash, &r); err != nil {
		return nil, SourceStatus{}, err
	}
	return &TransactionItem{Hash: strings.ToLower(r.Hash), AssetSymbol: "TON", Timestamp: time.Unix(r.Utime, 0).UTC()}, a.SourceStatus(), nil
}
func (a *TONAdapter) GetContractMetadata(context.Context, string) (*ContractMetadata, error) {
	return &ContractMetadata{Category: "EOA"}, nil
}

type CardanoAdapter struct {
	network, apiURL, key string
	client               *http.Client
	requestMu            sync.Mutex
	lastRequest          time.Time
	metrics              *providerMetrics
}

func NewCardanoAdapter(network, key string) *CardanoAdapter {
	return &CardanoAdapter{network: network, apiURL: blockfrostAPIURL, key: key, client: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(blockfrostSource, int(time.Second/blockfrostRequestGap))}
}
func (a *CardanoAdapter) Network() string { return a.network }
func (a *CardanoAdapter) NormalizeAddress(v string) (string, error) {
	v = strings.TrimSpace(v)
	if (strings.HasPrefix(v, "addr1") || strings.HasPrefix(v, "addr_test1")) && len(v) >= 50 && len(v) <= 120 && isLowerAlphaNumeric(v) {
		return v, nil
	}
	return "", fmt.Errorf("expected a Cardano address")
}
func (a *CardanoAdapter) NormalizeTransactionHash(v string) (string, error) {
	v = strings.TrimSpace(v)
	if len(v) != 64 {
		return "", fmt.Errorf("expected a Cardano transaction hash")
	}
	if decoded, err := hex.DecodeString(v); err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("expected a Cardano transaction hash")
	}
	return strings.ToLower(v), nil
}
func (a *CardanoAdapter) NativeAsset() Asset {
	return Asset{Kind: "NATIVE", Symbol: "ADA", Decimals: 6}
}
func (a *CardanoAdapter) ActivityLabel() string { return "" }
func (a *CardanoAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: blockfrostSource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}
func (a *CardanoAdapter) ProviderHealth() []ProviderHealth {
	return []ProviderHealth{a.metrics.snapshot()}
}
func (a *CardanoAdapter) request(ctx context.Context, path string, out any) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(blockfrostRequestGap))
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.apiURL+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("project_id", a.key)
	a.metrics.request(delay)
	a.lastRequest = time.Now()
	res, err := a.client.Do(req)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(blockfrostSource, err)
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return err
	}
	recordAcquisition(ctx, blockfrostSource, req, body)
	if res.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(blockfrostSource, res)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode Blockfrost response: %w", err)
	}
	a.metrics.success()
	return nil
}
func (a *CardanoAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	var r struct {
		Amount []struct {
			Unit     string `json:"unit"`
			Quantity string `json:"quantity"`
		} `json:"amount"`
	}
	if err := a.request(ctx, "/addresses/"+url.PathEscape(address), &r); err != nil {
		return nil, err
	}
	for _, x := range r.Amount {
		if x.Unit == "lovelace" {
			n, ok := new(big.Int).SetString(x.Quantity, 10)
			if ok {
				return n, nil
			}
		}
	}
	return big.NewInt(0), nil
}
func (a *CardanoAdapter) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }
func (a *CardanoAdapter) IsContract(context.Context, string) (bool, error)   { return false, nil }
func (a *CardanoAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	page := uint64(1)
	if cursor != "" {
		parsed, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil || parsed == 0 {
			return nil, fmt.Errorf("invalid cursor")
		}
		page = parsed
	}
	// Each transaction needs one UTXO lookup. Keep a bounded page so that a
	// single public trace job cannot consume an unbounded provider budget.
	perPage := limit
	if perPage > 10 {
		perPage = 10
	}
	var hashes []struct {
		Hash        string `json:"tx_hash"`
		BlockHeight int64  `json:"block_height"`
		BlockTime   int64  `json:"block_time"`
	}
	query := url.Values{"page": {strconv.FormatUint(page, 10)}, "count": {strconv.Itoa(int(perPage) + 1)}, "order": {"desc"}}
	if err := a.request(ctx, "/addresses/"+url.PathEscape(address)+"/transactions?"+query.Encode(), &hashes); err != nil {
		return nil, err
	}
	hasMore := len(hashes) > int(perPage)
	if hasMore {
		hashes = hashes[:perPage]
	}
	transfers := make([]TransferItem, 0, len(hashes))
	for _, transaction := range hashes {
		items, err := a.transactionTransfers(ctx, address, transaction.Hash, transaction.BlockHeight, transaction.BlockTime)
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, items...)
	}
	sortTransfers(transfers)
	next := ""
	if hasMore {
		next = strconv.FormatUint(page+1, 10)
	}
	status := a.SourceStatus()
	status.Warning = "Cardano UTXO transfers are transaction-level observations; review change outputs before drawing attribution conclusions."
	return &TransferPage{Transfers: transfers, NextCursor: next, HasMore: hasMore, SourceStatus: status}, nil
}
func (a *CardanoAdapter) transactionTransfers(ctx context.Context, address, hash string, blockHeight, blockTime int64) ([]TransferItem, error) {
	var transaction struct {
		Inputs  []cardanoUTXO `json:"inputs"`
		Outputs []cardanoUTXO `json:"outputs"`
	}
	if err := a.request(ctx, "/txs/"+url.PathEscape(hash)+"/utxos", &transaction); err != nil {
		return nil, err
	}
	transfers := make([]TransferItem, 0, len(transaction.Inputs)+len(transaction.Outputs))
	for index, input := range transaction.Inputs {
		if input.Address == address {
			continue
		}
		if amount := input.lovelace(); amount != "" {
			transfers = append(transfers, cardanoTransfer(hash, "in:"+strconv.Itoa(index), input.Address, address, amount, blockHeight, blockTime))
		}
	}
	for index, output := range transaction.Outputs {
		if output.Address == address {
			continue
		}
		if amount := output.lovelace(); amount != "" {
			transfers = append(transfers, cardanoTransfer(hash, "out:"+strconv.Itoa(index), address, output.Address, amount, blockHeight, blockTime))
		}
	}
	return transfers, nil
}

type cardanoUTXO struct {
	Address string `json:"address"`
	Amount  []struct {
		Unit     string `json:"unit"`
		Quantity string `json:"quantity"`
	} `json:"amount"`
}

func (u cardanoUTXO) lovelace() string {
	for _, amount := range u.Amount {
		if amount.Unit != "lovelace" {
			continue
		}
		if value, ok := new(big.Int).SetString(amount.Quantity, 10); ok && value.Sign() >= 0 {
			return value.String()
		}
	}
	return ""
}

func cardanoTransfer(hash, eventID, from, to, amount string, blockHeight, blockTime int64) TransferItem {
	return TransferItem{Hash: strings.ToLower(hash), EventID: eventID, TransferKind: "NATIVE", From: from, To: to, AmountBaseUnits: amount, Asset: Asset{Kind: "NATIVE", Symbol: "ADA", Decimals: 6}, BlockNumber: blockHeight, Timestamp: time.Unix(blockTime, 0).UTC()}
}

func (a *CardanoAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var transaction struct {
		Hash        string `json:"hash"`
		BlockHeight int64  `json:"block_height"`
		BlockTime   int64  `json:"block_time"`
	}
	if err := a.request(ctx, "/txs/"+url.PathEscape(hash), &transaction); err != nil {
		return nil, SourceStatus{}, err
	}
	if transaction.Hash == "" {
		return nil, SourceStatus{}, fmt.Errorf("Cardano transaction not found")
	}
	return &TransactionItem{Hash: strings.ToLower(transaction.Hash), BlockNumber: transaction.BlockHeight, Timestamp: time.Unix(transaction.BlockTime, 0).UTC(), AssetSymbol: "ADA"}, a.SourceStatus(), nil
}
func (a *CardanoAdapter) GetContractMetadata(context.Context, string) (*ContractMetadata, error) {
	return &ContractMetadata{Category: "EOA"}, nil
}

var _ ChainAdapter = (*TONAdapter)(nil)
var _ ChainAdapter = (*CardanoAdapter)(nil)

func isLowerAlphaNumeric(value string) bool {
	for _, character := range value {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func tonAddressChecksum(data []byte) [2]byte {
	var checksum uint16
	for _, value := range data {
		checksum ^= uint16(value) << 8
		for bit := 0; bit < 8; bit++ {
			if checksum&0x8000 != 0 {
				checksum = checksum<<1 ^ 0x1021
			} else {
				checksum <<= 1
			}
		}
	}
	return [2]byte{byte(checksum >> 8), byte(checksum)}
}
