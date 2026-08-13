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
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	AlchemySource     = "alchemy-transfers"
	alchemyRequestGap = time.Second / 5
)

// AlchemyEVMChainAdapter is one EVM implementation shared by networks whose
// historical transfer endpoint Alchemy supports. It does not assume that an
// EVM address belongs to a particular chain.
type AlchemyEVMChainAdapter struct {
	network, apiURL, apiKey string
	nativeAsset             Asset
	evmClient               *EVMClient
	httpClient              *http.Client
	requestMu               sync.Mutex
	lastRequest             time.Time
	metrics                 *providerMetrics
}

func NewAlchemyEVMChainAdapter(network, apiURL, apiKey string, nativeAsset Asset, evmClient *EVMClient) *AlchemyEVMChainAdapter {
	return &AlchemyEVMChainAdapter{network: network, apiURL: strings.TrimRight(apiURL, "/"), apiKey: apiKey, nativeAsset: nativeAsset, evmClient: evmClient, httpClient: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(AlchemySource, int(time.Second/alchemyRequestGap))}
}

func (a *AlchemyEVMChainAdapter) Network() string { return a.network }
func (a *AlchemyEVMChainAdapter) NormalizeAddress(value string) (string, error) {
	return normalizeEthereumAddress(value)
}
func (a *AlchemyEVMChainAdapter) NormalizeTransactionHash(value string) (string, error) {
	return normalizeEthereumHash(value)
}
func (a *AlchemyEVMChainAdapter) NativeAsset() Asset    { return a.nativeAsset }
func (a *AlchemyEVMChainAdapter) ActivityLabel() string { return "Outgoing nonce" }
func (a *AlchemyEVMChainAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	if a.evmClient == nil {
		return big.NewInt(0), fmt.Errorf("EVM RPC is unavailable")
	}
	return a.evmClient.GetBalance(ctx, address)
}
func (a *AlchemyEVMChainAdapter) GetTxCount(ctx context.Context, address string) (uint64, error) {
	if a.evmClient == nil {
		return 0, fmt.Errorf("EVM RPC is unavailable")
	}
	return a.evmClient.GetTxCount(ctx, address)
}
func (a *AlchemyEVMChainAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	if a.evmClient == nil {
		return false, fmt.Errorf("EVM RPC is unavailable")
	}
	return a.evmClient.IsContract(ctx, address)
}

type alchemyCursor struct {
	Inbound  string `json:"inbound"`
	Outbound string `json:"outbound"`
}
type alchemyResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	Error   *RPCError `json:"error"`
	Result  struct {
		Transfers []alchemyTransfer `json:"transfers"`
		PageKey   string            `json:"pageKey"`
	} `json:"result"`
}
type alchemyTransfer struct {
	BlockNum    string          `json:"blockNum"`
	Hash        string          `json:"hash"`
	From        string          `json:"from"`
	To          string          `json:"to"`
	Category    string          `json:"category"`
	UniqueID    string          `json:"uniqueId"`
	Asset       string          `json:"asset"`
	Value       json.RawMessage `json:"value"`
	RawContract struct {
		Address string `json:"address"`
		Value   string `json:"value"`
		Decimal string `json:"decimal"`
	} `json:"rawContract"`
	Metadata struct {
		BlockTimestamp string `json:"blockTimestamp"`
	} `json:"metadata"`
}

func (a *AlchemyEVMChainAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	if limit > 50 {
		limit = 50
	}
	state := alchemyCursor{}
	if cursor != "" {
		decoded, err := base64.RawURLEncoding.DecodeString(cursor)
		if err != nil || json.Unmarshal(decoded, &state) != nil {
			return nil, fmt.Errorf("invalid cursor")
		}
	}
	perDirection := (limit + 1) / 2
	inbound, inboundNext, err := a.transferPage(ctx, address, false, perDirection, state.Inbound)
	if err != nil {
		return nil, err
	}
	outbound, outboundNext, err := a.transferPage(ctx, address, true, perDirection, state.Outbound)
	if err != nil {
		return nil, err
	}
	transfers := append(inbound, outbound...)
	sortTransfers(transfers)
	if uint32(len(transfers)) > limit {
		transfers = transfers[:limit]
	}
	next := alchemyCursor{Inbound: inboundNext, Outbound: outboundNext}
	nextCursor := ""
	if next.Inbound != "" || next.Outbound != "" {
		encoded, _ := json.Marshal(next)
		nextCursor = base64.RawURLEncoding.EncodeToString(encoded)
	}
	return &TransferPage{Transfers: transfers, NextCursor: nextCursor, HasMore: nextCursor != "", SourceStatus: a.SourceStatus()}, nil
}

func (a *AlchemyEVMChainAdapter) transferPage(ctx context.Context, address string, outbound bool, limit uint32, pageKey string) ([]TransferItem, string, error) {
	filters := map[string]any{"fromBlock": "0x0", "toBlock": "latest", "category": []string{"external", "internal", "erc20"}, "excludeZeroValue": true, "withMetadata": true, "maxCount": fmt.Sprintf("0x%x", limit+1), "order": "desc"}
	if outbound {
		filters["fromAddress"] = address
	} else {
		filters["toAddress"] = address
	}
	if pageKey != "" {
		filters["pageKey"] = pageKey
	}
	var response alchemyResponse
	if err := a.call(ctx, "alchemy_getAssetTransfers", []any{filters}, &response); err != nil {
		return nil, "", err
	}
	if response.Error != nil {
		return nil, "", fmt.Errorf("Alchemy RPC error %d", response.Error.Code)
	}
	items := make([]TransferItem, 0, len(response.Result.Transfers))
	for _, transfer := range response.Result.Transfers {
		item, err := a.toTransfer(transfer)
		if err != nil {
			return nil, "", err
		}
		items = append(items, item)
	}
	if uint32(len(items)) > limit {
		items = items[:limit]
	}
	return items, response.Result.PageKey, nil
}

func (a *AlchemyEVMChainAdapter) toTransfer(value alchemyTransfer) (TransferItem, error) {
	from, err := normalizeEthereumAddress(value.From)
	if err != nil {
		return TransferItem{}, err
	}
	to, err := normalizeEthereumAddress(value.To)
	if err != nil {
		return TransferItem{}, err
	}
	hash, err := normalizeEthereumHash(value.Hash)
	if err != nil {
		return TransferItem{}, err
	}
	block, ok := new(big.Int).SetString(strings.TrimPrefix(value.BlockNum, "0x"), 16)
	if !ok {
		return TransferItem{}, fmt.Errorf("parse Alchemy block number")
	}
	timestamp, err := time.Parse(time.RFC3339, value.Metadata.BlockTimestamp)
	if err != nil {
		return TransferItem{}, fmt.Errorf("parse Alchemy timestamp: %w", err)
	}
	asset := a.nativeAsset
	transferKind := "NATIVE"
	amount := "0"
	if strings.EqualFold(value.Category, "erc20") {
		transferKind = "ERC20"
		asset = Asset{Kind: "ERC20", ContractAddress: strings.ToLower(value.RawContract.Address), Symbol: value.Asset, Decimals: decimalUint(value.RawContract.Decimal)}
		if parsed, ok := new(big.Int).SetString(strings.TrimPrefix(value.RawContract.Value, "0x"), 16); ok {
			amount = parsed.String()
		} else {
			return TransferItem{}, fmt.Errorf("parse Alchemy token amount")
		}
	} else {
		if strings.EqualFold(value.Category, "internal") {
			transferKind = "INTERNAL"
		}
		parsed, err := decimalBaseUnits(value.Value, asset.Decimals)
		if err != nil {
			return TransferItem{}, fmt.Errorf("parse Alchemy native amount: %w", err)
		}
		amount = parsed
	}
	eventID := value.UniqueID
	if eventID == "" {
		eventID = value.Category + ":" + hash
	}
	return TransferItem{Hash: hash, EventID: eventID, TransferKind: transferKind, From: from, To: to, AmountBaseUnits: amount, Asset: asset, BlockNumber: block.Int64(), Timestamp: timestamp.UTC()}, nil
}

func decimalUint(value string) uint32 {
	parsed, _ := strconv.ParseUint(value, 10, 32)
	return uint32(parsed)
}
func decimalBaseUnits(raw json.RawMessage, decimals uint32) (string, error) {
	value := strings.Trim(strings.TrimSpace(string(raw)), "\"")
	if value == "" || value == "null" {
		return "0", nil
	}
	rational, ok := new(big.Rat).SetString(value)
	if !ok {
		return "", fmt.Errorf("invalid decimal")
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	rational.Mul(rational, new(big.Rat).SetInt(scale))
	if !rational.IsInt() {
		return "", fmt.Errorf("fractional base unit")
	}
	return rational.Num().String(), nil
}

func (a *AlchemyEVMChainAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	if a.evmClient == nil {
		return nil, SourceStatus{}, fmt.Errorf("EVM RPC is unavailable")
	}
	transaction, err := a.evmClient.GetTransaction(ctx, hash)
	if err != nil {
		return nil, SourceStatus{}, err
	}
	transaction.AssetSymbol = a.nativeAsset.Symbol
	return transaction, a.SourceStatus(), nil
}
func (a *AlchemyEVMChainAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	isContract, err := a.IsContract(ctx, address)
	if err != nil {
		return nil, err
	}
	if !isContract {
		return &ContractMetadata{Category: "EOA"}, nil
	}
	return &ContractMetadata{Category: "CONTRACT"}, nil
}
func (a *AlchemyEVMChainAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: AlchemySource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}
func (a *AlchemyEVMChainAdapter) ProviderHealth() []ProviderHealth {
	health := []ProviderHealth{a.metrics.snapshot()}
	if a.evmClient != nil {
		health = append(health, a.evmClient.ProviderHealth())
	}
	return health
}

func (a *AlchemyEVMChainAdapter) call(ctx context.Context, method string, params []any, output *alchemyResponse) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(alchemyRequestGap))
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
	payload, err := json.Marshal(RPCRequest{JSONRPC: "2.0", Method: method, Params: params, ID: 1})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, a.apiURL+"/"+a.apiKey, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := a.httpClient.Do(request)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(AlchemySource, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read Alchemy response: %w", err)
	}
	recordAcquisition(ctx, AlchemySource, request, body)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(AlchemySource, response)
	}
	if err := json.Unmarshal(body, output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Alchemy response: %w", err)
	}
	a.metrics.success()
	return nil
}

var _ ChainAdapter = (*AlchemyEVMChainAdapter)(nil)
