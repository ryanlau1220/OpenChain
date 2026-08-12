package adapter

import (
	"context"
	"encoding/base64"
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
	BlockscoutBaseAPIURL = "https://api.blockscout.com/8453/api/v2"
	BlockscoutSource     = "blockscout-base"
	blockscoutRequestGap = time.Second / 5
)

// BlockscoutChainAdapter reads Base transfer facts from Blockscout's authenticated API.
type BlockscoutChainAdapter struct {
	network     string
	apiURL      string
	apiKey      string
	evmClient   *EVMClient
	httpClient  *http.Client
	requestMu   sync.Mutex
	lastRequest time.Time
	metrics     *providerMetrics
}

func NewBlockscoutChainAdapter(network, apiURL, apiKey string, evmClient *EVMClient) *BlockscoutChainAdapter {
	return &BlockscoutChainAdapter{network: network, apiURL: strings.TrimRight(apiURL, "/"), apiKey: apiKey, evmClient: evmClient, httpClient: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(BlockscoutSource, int(time.Second/blockscoutRequestGap))}
}

func (a *BlockscoutChainAdapter) Network() string { return a.network }

func (a *BlockscoutChainAdapter) NormalizeAddress(value string) (string, error) {
	return normalizeEthereumAddress(value)
}

func (a *BlockscoutChainAdapter) NormalizeTransactionHash(value string) (string, error) {
	return normalizeEthereumHash(value)
}

func (a *BlockscoutChainAdapter) NativeAsset() Asset {
	return Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}

func (a *BlockscoutChainAdapter) ActivityLabel() string { return "Outgoing nonce" }

func (a *BlockscoutChainAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	if a.evmClient == nil {
		return big.NewInt(0), fmt.Errorf("Base RPC is unavailable")
	}
	return a.evmClient.GetBalance(ctx, address)
}

func (a *BlockscoutChainAdapter) GetTxCount(ctx context.Context, address string) (uint64, error) {
	if a.evmClient == nil {
		return 0, fmt.Errorf("Base RPC is unavailable")
	}
	return a.evmClient.GetTxCount(ctx, address)
}

func (a *BlockscoutChainAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	if a.evmClient == nil {
		return false, fmt.Errorf("Base RPC is unavailable")
	}
	return a.evmClient.IsContract(ctx, address)
}

type blockscoutAddress struct {
	Hash string `json:"hash"`
}

type blockscoutTransaction struct {
	Hash            string            `json:"hash"`
	TransactionHash string            `json:"transaction_hash"`
	From            blockscoutAddress `json:"from"`
	To              blockscoutAddress `json:"to"`
	Value           string            `json:"value"`
	BlockNumber     uint64            `json:"block_number"`
	BlockHash       string            `json:"block_hash"`
	Timestamp       string            `json:"timestamp"`
	Index           uint64            `json:"index"`
}

type blockscoutToken struct {
	AddressHash string `json:"address_hash"`
	Symbol      string `json:"symbol"`
	Decimals    string `json:"decimals"`
}

type blockscoutTokenTransfer struct {
	TransactionHash string            `json:"transaction_hash"`
	From            blockscoutAddress `json:"from"`
	To              blockscoutAddress `json:"to"`
	Token           blockscoutToken   `json:"token"`
	Total           struct {
		Value string `json:"value"`
	} `json:"total"`
	TokenType   string `json:"token_type"`
	LogIndex    uint64 `json:"log_index"`
	BlockNumber uint64 `json:"block_number"`
	BlockHash   string `json:"block_hash"`
	Timestamp   string `json:"timestamp"`
}

type blockscoutItems[T any] struct {
	Items          []T                       `json:"items"`
	NextPageParams *blockscoutPageParameters `json:"next_page_params"`
}

type blockscoutPageParameters struct {
	BlockNumber uint64 `json:"block_number"`
	Index       uint64 `json:"index"`
	ItemsCount  uint64 `json:"items_count"`
}

type blockscoutCursor struct {
	Transactions         *blockscoutPageParameters `json:"transactions,omitempty"`
	InternalTransactions *blockscoutPageParameters `json:"internal_transactions,omitempty"`
	TokenTransfers       *blockscoutPageParameters `json:"token_transfers,omitempty"`
}

func (a *BlockscoutChainAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	pageCursor, err := decodeBlockscoutCursor(cursor)
	if err != nil {
		return nil, err
	}
	native, nextTransactions, err := a.transactions(ctx, address, pageCursor.Transactions)
	if err != nil {
		return nil, err
	}
	internal, nextInternal, internalErr := a.internalTransfers(ctx, address, pageCursor.InternalTransactions)
	tokens, nextTokens, tokenErr := a.tokenTransfers(ctx, address, pageCursor.TokenTransfers)
	transfers := append(append(native, internal...), tokens...)
	sortTransfers(transfers)
	if len(transfers) > int(limit) {
		transfers = transfers[:limit]
	}
	nextCursor, err := encodeBlockscoutCursor(blockscoutCursor{Transactions: nextTransactions, InternalTransactions: nextInternal, TokenTransfers: nextTokens})
	if err != nil {
		return nil, err
	}
	status := a.SourceStatus()
	if internalErr != nil || tokenErr != nil {
		status.IsComplete = false
		if tokenErr != nil {
			status.Warning = "Blockscout token transfer history is temporarily unavailable; showing the available transfer evidence."
		} else {
			status.Warning = "Blockscout internal transfer history is temporarily unavailable; showing the available transfer evidence."
		}
	}
	return &TransferPage{Transfers: transfers, NextCursor: nextCursor, HasMore: nextCursor != "", SourceStatus: status}, nil
}

func (a *BlockscoutChainAdapter) transactions(ctx context.Context, address string, cursor *blockscoutPageParameters) ([]TransferItem, *blockscoutPageParameters, error) {
	var response blockscoutItems[blockscoutTransaction]
	if err := a.get(ctx, "/addresses/"+strings.ToLower(address)+"/transactions", blockscoutQuery(cursor), &response); err != nil {
		return nil, nil, err
	}
	transfers := make([]TransferItem, 0, len(response.Items))
	for _, item := range response.Items {
		transfer, err := blockscoutNativeTransfer(item, "tx", "NATIVE")
		if err != nil {
			return nil, nil, err
		}
		transfers = append(transfers, transfer)
	}
	return transfers, response.NextPageParams, nil
}

func (a *BlockscoutChainAdapter) internalTransfers(ctx context.Context, address string, cursor *blockscoutPageParameters) ([]TransferItem, *blockscoutPageParameters, error) {
	var response blockscoutItems[blockscoutTransaction]
	if err := a.get(ctx, "/addresses/"+strings.ToLower(address)+"/internal-transactions", blockscoutQuery(cursor), &response); err != nil {
		return nil, nil, err
	}
	transfers := make([]TransferItem, 0, len(response.Items))
	for _, item := range response.Items {
		transfer, err := blockscoutNativeTransfer(item, "trace:"+strconv.FormatUint(item.Index, 10), "INTERNAL")
		if err != nil {
			return nil, nil, err
		}
		transfers = append(transfers, transfer)
	}
	return transfers, response.NextPageParams, nil
}

func (a *BlockscoutChainAdapter) tokenTransfers(ctx context.Context, address string, cursor *blockscoutPageParameters) ([]TransferItem, *blockscoutPageParameters, error) {
	var response blockscoutItems[blockscoutTokenTransfer]
	if err := a.get(ctx, "/addresses/"+strings.ToLower(address)+"/token-transfers", blockscoutQuery(cursor), &response); err != nil {
		return nil, nil, err
	}
	transfers := make([]TransferItem, 0, len(response.Items))
	for _, item := range response.Items {
		if item.TokenType != "ERC-20" {
			continue
		}
		if item.TransactionHash == "" || item.From.Hash == "" || item.To.Hash == "" || item.Token.AddressHash == "" {
			return nil, nil, fmt.Errorf("Blockscout ERC-20 transfer is missing an address or hash")
		}
		if _, ok := new(big.Int).SetString(item.Total.Value, 10); !ok {
			return nil, nil, fmt.Errorf("parse Blockscout ERC-20 transfer value")
		}
		decimals, err := strconv.ParseUint(item.Token.Decimals, 10, 32)
		if err != nil {
			return nil, nil, fmt.Errorf("parse Blockscout token decimals: %w", err)
		}
		timestamp, err := blockscoutTimestamp(item.Timestamp)
		if err != nil {
			return nil, nil, err
		}
		transfers = append(transfers, TransferItem{Hash: strings.ToLower(item.TransactionHash), EventID: "log:" + strconv.FormatUint(item.LogIndex, 10), TransferKind: "ERC20", From: strings.ToLower(item.From.Hash), To: strings.ToLower(item.To.Hash), AmountBaseUnits: item.Total.Value, Asset: Asset{Kind: "ERC20", ContractAddress: strings.ToLower(item.Token.AddressHash), Symbol: item.Token.Symbol, Decimals: uint32(decimals)}, BlockNumber: int64(item.BlockNumber), BlockHash: strings.ToLower(item.BlockHash), Timestamp: timestamp})
	}
	return transfers, response.NextPageParams, nil
}

func blockscoutQuery(cursor *blockscoutPageParameters) url.Values {
	if cursor == nil {
		return nil
	}
	return url.Values{"block_number": {strconv.FormatUint(cursor.BlockNumber, 10)}, "index": {strconv.FormatUint(cursor.Index, 10)}, "items_count": {strconv.FormatUint(cursor.ItemsCount, 10)}}
}

func decodeBlockscoutCursor(cursor string) (blockscoutCursor, error) {
	if cursor == "" {
		return blockscoutCursor{}, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return blockscoutCursor{}, fmt.Errorf("invalid cursor")
	}
	var value blockscoutCursor
	if err := json.Unmarshal(decoded, &value); err != nil {
		return blockscoutCursor{}, fmt.Errorf("invalid cursor")
	}
	return value, nil
}

func encodeBlockscoutCursor(cursor blockscoutCursor) (string, error) {
	if cursor.Transactions == nil && cursor.InternalTransactions == nil && cursor.TokenTransfers == nil {
		return "", nil
	}
	encoded, err := json.Marshal(cursor)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

func blockscoutNativeTransfer(item blockscoutTransaction, eventID, kind string) (TransferItem, error) {
	hash := item.Hash
	if hash == "" {
		hash = item.TransactionHash
	}
	if hash == "" || item.From.Hash == "" || item.To.Hash == "" {
		return TransferItem{}, fmt.Errorf("Blockscout %s transfer is missing an address or hash", strings.ToLower(kind))
	}
	if _, ok := new(big.Int).SetString(item.Value, 10); !ok {
		return TransferItem{}, fmt.Errorf("parse Blockscout %s transfer value", strings.ToLower(kind))
	}
	timestamp, err := blockscoutTimestamp(item.Timestamp)
	if err != nil {
		return TransferItem{}, err
	}
	return TransferItem{Hash: strings.ToLower(hash), EventID: eventID, TransferKind: kind, From: strings.ToLower(item.From.Hash), To: strings.ToLower(item.To.Hash), AmountBaseUnits: item.Value, Asset: Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, BlockNumber: int64(item.BlockNumber), BlockHash: strings.ToLower(item.BlockHash), Timestamp: timestamp}, nil
}

func blockscoutTimestamp(value string) (time.Time, error) {
	timestamp, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse Blockscout timestamp: %w", err)
	}
	return timestamp, nil
}

func (a *BlockscoutChainAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var item blockscoutTransaction
	if err := a.get(ctx, "/transactions/"+strings.ToLower(hash), nil, &item); err != nil {
		return nil, SourceStatus{}, err
	}
	transaction, err := blockscoutNativeTransfer(item, "tx", "NATIVE")
	if err != nil {
		return nil, SourceStatus{}, err
	}
	return &TransactionItem{Hash: transaction.Hash, From: transaction.From, To: transaction.To, ValueBaseUnits: transaction.AmountBaseUnits, AssetSymbol: "ETH", BlockNumber: transaction.BlockNumber, Timestamp: transaction.Timestamp}, a.SourceStatus(), nil
}

func (a *BlockscoutChainAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	isContract, err := a.IsContract(ctx, address)
	if err != nil {
		return nil, err
	}
	if !isContract {
		return &ContractMetadata{Category: "EOA"}, nil
	}
	return &ContractMetadata{Category: "CONTRACT"}, nil
}

func (a *BlockscoutChainAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: BlockscoutSource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}

func (a *BlockscoutChainAdapter) get(ctx context.Context, path string, query url.Values, output any) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(blockscoutRequestGap))
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
	requestURL, err := url.Parse(a.apiURL + path)
	if err != nil {
		return err
	}
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+a.apiKey)
	response, err := a.httpClient.Do(request)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(BlockscoutSource, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read Blockscout response: %w", err)
	}
	recordAcquisition(ctx, BlockscoutSource, request, body)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(BlockscoutSource, response)
	}
	if err := json.Unmarshal(body, output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Blockscout response: %w", err)
	}
	a.metrics.success()
	return nil
}

func (a *BlockscoutChainAdapter) ProviderHealth() []ProviderHealth {
	health := []ProviderHealth{a.metrics.snapshot()}
	if a.evmClient != nil {
		health = append(health, a.evmClient.ProviderHealth())
	}
	return health
}

var _ ChainAdapter = (*BlockscoutChainAdapter)(nil)
