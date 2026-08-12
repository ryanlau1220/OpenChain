package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
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
	TronGridAPIURL     = "https://api.trongrid.io"
	TronGridSource     = "trongrid"
	tronGridRequestGap = time.Second / 10
)

type TronAdapter struct {
	network     string
	apiURL      string
	apiKey      string
	httpClient  *http.Client
	requestMu   sync.Mutex
	lastRequest time.Time
	metrics     *providerMetrics
}

func NewTronAdapter(network, apiURL, apiKey string) *TronAdapter {
	return &TronAdapter{network: network, apiURL: strings.TrimRight(apiURL, "/"), apiKey: apiKey, httpClient: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(TronGridSource, int(time.Second/tronGridRequestGap))}
}

func (a *TronAdapter) Network() string { return a.network }

func (a *TronAdapter) NormalizeAddress(value string) (string, error) {
	value = strings.TrimSpace(value)
	decoded, err := decodeBase58(value)
	if err != nil || len(decoded) != 25 || decoded[0] != 0x41 {
		return "", fmt.Errorf("expected a TRON address")
	}
	checksum := tronChecksum(decoded[:21])
	if !bytes.Equal(decoded[21:], checksum[:]) {
		return "", fmt.Errorf("expected a TRON address")
	}
	return value, nil
}

func (a *TronAdapter) NormalizeTransactionHash(value string) (string, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "0x")
	if len(value) != 64 {
		return "", fmt.Errorf("expected a TRON transaction hash")
	}
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return "", fmt.Errorf("expected a TRON transaction hash")
	}
	return strings.ToLower(value), nil
}

func (a *TronAdapter) NativeAsset() Asset { return Asset{Kind: "NATIVE", Symbol: "TRX", Decimals: 6} }

func (a *TronAdapter) ActivityLabel() string { return "" }

func (a *TronAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	var result struct {
		Balance json.Number `json:"balance"`
	}
	if err := a.post(ctx, "/wallet/getaccount", map[string]any{"address": address, "visible": true}, &result); err != nil {
		return nil, err
	}
	if result.Balance == "" {
		return big.NewInt(0), nil
	}
	balance, ok := new(big.Int).SetString(result.Balance.String(), 10)
	if !ok {
		return nil, fmt.Errorf("parse TRON balance")
	}
	return balance, nil
}

func (a *TronAdapter) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }

func (a *TronAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	var result struct {
		ContractAddress string `json:"contract_address"`
	}
	if err := a.post(ctx, "/wallet/getcontract", map[string]any{"value": address, "visible": true}, &result); err != nil {
		return false, err
	}
	return result.ContractAddress != "", nil
}

type tronTransactionPage struct {
	Data []tronTransaction `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

type tronCursor struct {
	Transactions string `json:"transactions,omitempty"`
	TRC20        string `json:"trc20,omitempty"`
	Internal     string `json:"internal,omitempty"`
}

type tronTransaction struct {
	ID          string `json:"txID"`
	BlockNumber int64  `json:"blockNumber"`
	RawData     struct {
		Timestamp int64 `json:"timestamp"`
		Contracts []struct {
			Type      string `json:"type"`
			Parameter struct {
				Value json.RawMessage `json:"value"`
			} `json:"parameter"`
		} `json:"contract"`
	} `json:"raw_data"`
}

func (a *TronAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	state, err := decodeTronCursor(cursor)
	if err != nil {
		return nil, err
	}
	perSourceLimit := limit / 3
	if perSourceLimit == 0 {
		perSourceLimit = 1
	}
	native, nextTransactions, err := a.nativeTransfers(ctx, address, perSourceLimit, state.Transactions)
	if err != nil {
		return nil, err
	}
	tokens, nextTRC20, tokenErr := a.trc20Transfers(ctx, address, perSourceLimit, state.TRC20)
	internal, nextInternal, internalErr := a.internalTransfers(ctx, address, perSourceLimit, state.Internal)
	transfers := append(append(native, tokens...), internal...)
	sortTransfers(transfers)
	next, err := encodeTronCursor(tronCursor{Transactions: nextTransactions, TRC20: nextTRC20, Internal: nextInternal})
	if err != nil {
		return nil, err
	}
	status := a.SourceStatus()
	if tokenErr != nil || internalErr != nil {
		status.IsComplete = false
		if tokenErr != nil {
			status.Warning = "TronGrid TRC-20 history is temporarily unavailable; showing the available transfer evidence."
		} else {
			status.Warning = "TronGrid internal-transfer history is temporarily unavailable; showing the available transfer evidence."
		}
	}
	return &TransferPage{Transfers: transfers, NextCursor: next, HasMore: next != "", SourceStatus: status}, nil
}

func tronHistoryQuery(limit uint32, cursor string) url.Values {
	query := url.Values{"only_confirmed": {"true"}, "limit": {fmt.Sprintf("%d", limit)}}
	if cursor != "" {
		query.Set("fingerprint", cursor)
	}
	return query
}

func (a *TronAdapter) nativeTransfers(ctx context.Context, address string, limit uint32, cursor string) ([]TransferItem, string, error) {
	var page tronTransactionPage
	if err := a.get(ctx, "/v1/accounts/"+url.PathEscape(address)+"/transactions", tronHistoryQuery(limit, cursor), &page); err != nil {
		return nil, "", err
	}
	transfers := make([]TransferItem, 0, len(page.Data))
	for _, transaction := range page.Data {
		for index, contract := range transaction.RawData.Contracts {
			if contract.Type != "TransferContract" && contract.Type != "TransferAssetContract" {
				continue
			}
			transfer, err := a.transferFromContract(transaction, index, contract.Type, contract.Parameter.Value)
			if err != nil {
				return nil, "", err
			}
			if transfer.From == address || transfer.To == address {
				transfers = append(transfers, transfer)
			}
		}
	}
	return transfers, page.Meta.Fingerprint, nil
}

func (a *TronAdapter) transferFromContract(transaction tronTransaction, index int, contractType string, raw json.RawMessage) (TransferItem, error) {
	var value struct {
		OwnerAddress string      `json:"owner_address"`
		ToAddress    string      `json:"to_address"`
		Amount       json.Number `json:"amount"`
		AssetName    string      `json:"asset_name"`
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return TransferItem{}, fmt.Errorf("decode TRON transfer: %w", err)
	}
	from, err := tronAddressFromHex(value.OwnerAddress)
	if err != nil {
		return TransferItem{}, err
	}
	to, err := tronAddressFromHex(value.ToAddress)
	if err != nil {
		return TransferItem{}, err
	}
	if _, ok := new(big.Int).SetString(value.Amount.String(), 10); !ok {
		return TransferItem{}, fmt.Errorf("parse TRON transfer")
	}
	asset, kind := a.NativeAsset(), "NATIVE"
	if contractType == "TransferAssetContract" {
		if value.AssetName == "" {
			return TransferItem{}, fmt.Errorf("parse TRC-10 asset")
		}
		asset, kind = Asset{Kind: "TRC10", ContractAddress: value.AssetName, Symbol: "TRC10", Decimals: 0}, "TRC10"
	}
	return TransferItem{Hash: strings.ToLower(transaction.ID), EventID: fmt.Sprintf("contract:%d", index), TransferKind: kind, From: from, To: to, AmountBaseUnits: value.Amount.String(), Asset: asset, BlockNumber: transaction.BlockNumber, Timestamp: time.UnixMilli(transaction.RawData.Timestamp).UTC()}, nil
}

type tronTRC20Page struct {
	Data []struct {
		TransactionID  string `json:"transaction_id"`
		BlockTimestamp int64  `json:"block_timestamp"`
		From           string `json:"from"`
		To             string `json:"to"`
		Value          string `json:"value"`
		TokenInfo      struct {
			Address  string      `json:"address"`
			Symbol   string      `json:"symbol"`
			Decimals json.Number `json:"decimals"`
		} `json:"token_info"`
	} `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

func (a *TronAdapter) trc20Transfers(ctx context.Context, address string, limit uint32, cursor string) ([]TransferItem, string, error) {
	var page tronTRC20Page
	if err := a.get(ctx, "/v1/accounts/"+url.PathEscape(address)+"/transactions/trc20", tronHistoryQuery(limit, cursor), &page); err != nil {
		return nil, "", err
	}
	transfers := make([]TransferItem, 0, len(page.Data))
	for index, item := range page.Data {
		if item.TransactionID == "" || item.From == "" || item.To == "" || item.TokenInfo.Address == "" || item.TokenInfo.Symbol == "" {
			return nil, "", fmt.Errorf("parse TRC-20 transfer")
		}
		if _, ok := new(big.Int).SetString(item.Value, 10); !ok {
			return nil, "", fmt.Errorf("parse TRC-20 amount")
		}
		decimals, ok := new(big.Int).SetString(item.TokenInfo.Decimals.String(), 10)
		if !ok || !decimals.IsUint64() || decimals.Uint64() > 255 {
			return nil, "", fmt.Errorf("parse TRC-20 decimals")
		}
		from, err := a.canonicalAddress(item.From)
		if err != nil {
			return nil, "", err
		}
		to, err := a.canonicalAddress(item.To)
		if err != nil {
			return nil, "", err
		}
		transfers = append(transfers, TransferItem{Hash: strings.ToLower(item.TransactionID), EventID: fmt.Sprintf("trc20:%d", index), TransferKind: "TRC20", From: from, To: to, AmountBaseUnits: item.Value, Asset: Asset{Kind: "TRC20", ContractAddress: item.TokenInfo.Address, Symbol: item.TokenInfo.Symbol, Decimals: uint32(decimals.Uint64())}, Timestamp: time.UnixMilli(item.BlockTimestamp).UTC()})
	}
	return transfers, page.Meta.Fingerprint, nil
}

type tronInternalPage struct {
	Data []struct {
		TransactionID  string `json:"transaction_id"`
		BlockNumber    int64  `json:"block_number"`
		BlockTimestamp int64  `json:"block_timestamp"`
		CallerAddress  string `json:"caller_address"`
		ToAddress      string `json:"transferTo_address"`
		Rejected       bool   `json:"rejected"`
		CallValues     []struct {
			CallValue json.Number `json:"callValue"`
			TokenID   string      `json:"tokenId"`
		} `json:"callValueInfo"`
	} `json:"data"`
	Meta struct {
		Fingerprint string `json:"fingerprint"`
	} `json:"meta"`
}

func (a *TronAdapter) internalTransfers(ctx context.Context, address string, limit uint32, cursor string) ([]TransferItem, string, error) {
	var page tronInternalPage
	if err := a.get(ctx, "/v1/accounts/"+url.PathEscape(address)+"/internal-transactions", tronHistoryQuery(limit, cursor), &page); err != nil {
		return nil, "", err
	}
	transfers := make([]TransferItem, 0, len(page.Data))
	for index, item := range page.Data {
		if item.Rejected || item.TransactionID == "" || item.CallerAddress == "" || item.ToAddress == "" {
			continue
		}
		from, err := a.canonicalAddress(item.CallerAddress)
		if err != nil {
			return nil, "", err
		}
		to, err := a.canonicalAddress(item.ToAddress)
		if err != nil {
			return nil, "", err
		}
		for valueIndex, value := range item.CallValues {
			if _, ok := new(big.Int).SetString(value.CallValue.String(), 10); !ok {
				return nil, "", fmt.Errorf("parse TRON internal transfer")
			}
			asset, kind := a.NativeAsset(), "INTERNAL"
			if value.TokenID != "" {
				asset, kind = Asset{Kind: "TRC10", ContractAddress: value.TokenID, Symbol: "TRC10", Decimals: 0}, "INTERNAL_TRC10"
			}
			transfers = append(transfers, TransferItem{Hash: strings.ToLower(item.TransactionID), EventID: fmt.Sprintf("internal:%d:%d", index, valueIndex), TransferKind: kind, From: from, To: to, AmountBaseUnits: value.CallValue.String(), Asset: asset, BlockNumber: item.BlockNumber, Timestamp: time.UnixMilli(item.BlockTimestamp).UTC()})
		}
	}
	return transfers, page.Meta.Fingerprint, nil
}

func (a *TronAdapter) canonicalAddress(value string) (string, error) {
	if strings.HasPrefix(strings.TrimPrefix(value, "0x"), "41") {
		return tronAddressFromHex(value)
	}
	return a.NormalizeAddress(value)
}

func (a *TronAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var transaction tronTransaction
	if err := a.post(ctx, "/wallet/gettransactionbyid", map[string]any{"value": hash}, &transaction); err != nil {
		return nil, SourceStatus{}, err
	}
	for index, contract := range transaction.RawData.Contracts {
		if contract.Type != "TransferContract" {
			continue
		}
		transfer, err := a.transferFromContract(transaction, index, contract.Type, contract.Parameter.Value)
		if err != nil {
			return nil, SourceStatus{}, err
		}
		return &TransactionItem{Hash: transfer.Hash, From: transfer.From, To: transfer.To, ValueBaseUnits: transfer.AmountBaseUnits, AssetSymbol: "TRX", BlockNumber: transfer.BlockNumber, Timestamp: transfer.Timestamp}, a.SourceStatus(), nil
	}
	return &TransactionItem{Hash: hash, BlockNumber: transaction.BlockNumber, Timestamp: time.UnixMilli(transaction.RawData.Timestamp).UTC(), AssetSymbol: "TRX"}, a.SourceStatus(), nil
}

func (a *TronAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	isContract, err := a.IsContract(ctx, address)
	if err != nil {
		return nil, err
	}
	if isContract {
		return &ContractMetadata{Category: "CONTRACT"}, nil
	}
	return &ContractMetadata{Category: "EOA"}, nil
}

func (a *TronAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: TronGridSource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}

func decodeTronCursor(cursor string) (tronCursor, error) {
	if cursor == "" {
		return tronCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return tronCursor{}, fmt.Errorf("invalid cursor")
	}
	var result tronCursor
	if err := json.Unmarshal(decoded, &result); err != nil {
		return tronCursor{}, fmt.Errorf("invalid cursor")
	}
	return result, nil
}

func encodeTronCursor(cursor tronCursor) (string, error) {
	if cursor.Transactions == "" && cursor.TRC20 == "" && cursor.Internal == "" {
		return "", nil
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func (a *TronAdapter) get(ctx context.Context, path string, query url.Values, output any) error {
	requestURL := a.apiURL + path
	if query != nil {
		requestURL += "?" + query.Encode()
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return err
	}
	return a.do(ctx, request, output)
}

func (a *TronAdapter) post(ctx context.Context, path string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	return a.do(ctx, request, output)
}

func (a *TronAdapter) do(ctx context.Context, request *http.Request, output any) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(tronGridRequestGap))
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
	request = request.WithContext(ctx)
	request.Header.Set("TRON-PRO-API-KEY", a.apiKey)
	response, err := a.httpClient.Do(request)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(TronGridSource, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read TronGrid response: %w", err)
	}
	recordAcquisition(ctx, TronGridSource, request, body)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(TronGridSource, response)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode TronGrid response: %w", err)
	}
	a.metrics.success()
	return nil
}

func (a *TronAdapter) ProviderHealth() []ProviderHealth {
	return []ProviderHealth{a.metrics.snapshot()}
}

func tronAddressFromHex(value string) (string, error) {
	value = strings.TrimPrefix(strings.TrimSpace(value), "0x")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 21 || decoded[0] != 0x41 {
		return "", fmt.Errorf("parse TRON address")
	}
	checksum := tronChecksum(decoded)
	return encodeBase58(append(decoded, checksum[:]...)), nil
}

func tronChecksum(value []byte) [4]byte {
	first := sha256.Sum256(value)
	second := sha256.Sum256(first[:])
	return [4]byte(second[:4])
}

var _ ChainAdapter = (*TronAdapter)(nil)
