package adapter

import (
	"bytes"
	"context"
	"crypto/sha256"
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
}

func NewTronAdapter(network, apiURL, apiKey string) *TronAdapter {
	return &TronAdapter{network: network, apiURL: strings.TrimRight(apiURL, "/"), apiKey: apiKey, httpClient: &http.Client{Timeout: 15 * time.Second}}
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
	query := url.Values{"only_confirmed": {"true"}, "limit": {fmt.Sprintf("%d", limit)}}
	if cursor != "" {
		query.Set("fingerprint", cursor)
	}
	var page tronTransactionPage
	if err := a.get(ctx, "/v1/accounts/"+url.PathEscape(address)+"/transactions", query, &page); err != nil {
		return nil, err
	}
	transfers := make([]TransferItem, 0, len(page.Data))
	for _, transaction := range page.Data {
		for index, contract := range transaction.RawData.Contracts {
			if contract.Type != "TransferContract" {
				continue
			}
			transfer, err := a.transferFromContract(transaction, index, contract.Parameter.Value)
			if err != nil {
				return nil, err
			}
			if transfer.From == address || transfer.To == address {
				transfers = append(transfers, transfer)
			}
		}
	}
	return &TransferPage{Transfers: transfers, NextCursor: page.Meta.Fingerprint, HasMore: page.Meta.Fingerprint != "", SourceStatus: a.SourceStatus()}, nil
}

func (a *TronAdapter) transferFromContract(transaction tronTransaction, index int, raw json.RawMessage) (TransferItem, error) {
	var value struct {
		OwnerAddress string      `json:"owner_address"`
		ToAddress    string      `json:"to_address"`
		Amount       json.Number `json:"amount"`
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
	return TransferItem{Hash: strings.ToLower(transaction.ID), EventID: fmt.Sprintf("contract:%d", index), TransferKind: "NATIVE", From: from, To: to, AmountBaseUnits: value.Amount.String(), Asset: a.NativeAsset(), BlockNumber: transaction.BlockNumber, Timestamp: time.UnixMilli(transaction.RawData.Timestamp).UTC()}, nil
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
		transfer, err := a.transferFromContract(transaction, index, contract.Parameter.Value)
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
	return SourceStatus{Source: TronGridSource, RetrievedAt: time.Now().UTC(), IsComplete: false, Warning: "Showing confirmed native TRX transfers only; TRC-10, TRC-20, and internal transfers are not included."}
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
	if delay := time.Until(a.lastRequest.Add(tronGridRequestGap)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	a.lastRequest = time.Now()
	request = request.WithContext(ctx)
	request.Header.Set("TRON-PRO-API-KEY", a.apiKey)
	response, err := a.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("TronGrid request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("TronGrid returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode TronGrid response: %w", err)
	}
	return nil
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
