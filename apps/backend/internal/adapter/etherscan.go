package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	EtherscanAPIURL     = "https://api.etherscan.io/v2/api"
	etherscanRequestGap = time.Second / 5
)

type EVMChainAdapter struct {
	network    string
	chainID    string
	apiURL     string
	apiKey     string
	evmClient  *EVMClient
	httpClient *http.Client

	requestMu   sync.Mutex
	lastRequest time.Time
	metrics     *providerMetrics
}

func NewEVMChainAdapter(network, chainID, apiURL, apiKey string, evmClient *EVMClient) *EVMChainAdapter {
	return &EVMChainAdapter{network: network, chainID: chainID, apiURL: apiURL, apiKey: apiKey, evmClient: evmClient, httpClient: &http.Client{Timeout: 15 * time.Second}, metrics: newProviderMetrics(EtherscanSource, int(time.Second/etherscanRequestGap))}
}

func (a *EVMChainAdapter) Network() string { return a.network }

func (a *EVMChainAdapter) NormalizeAddress(value string) (string, error) {
	return normalizeEthereumAddress(value)
}

func (a *EVMChainAdapter) NormalizeTransactionHash(value string) (string, error) {
	return normalizeEthereumHash(value)
}

func (a *EVMChainAdapter) NativeAsset() Asset {
	if a.network == "bnb-chain" {
		return Asset{Kind: "NATIVE", Symbol: "BNB", Decimals: 18}
	}
	return Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}

func (a *EVMChainAdapter) ActivityLabel() string { return "Outgoing nonce" }

func (a *EVMChainAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	if a.evmClient == nil {
		return big.NewInt(0), fmt.Errorf("Ethereum RPC is unavailable")
	}
	return a.evmClient.GetBalance(ctx, address)
}

func (a *EVMChainAdapter) GetTxCount(ctx context.Context, address string) (uint64, error) {
	if a.evmClient == nil {
		return 0, fmt.Errorf("Ethereum RPC is unavailable")
	}
	return a.evmClient.GetTxCount(ctx, address)
}

func (a *EVMChainAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	if a.evmClient == nil {
		return false, fmt.Errorf("Ethereum RPC is unavailable")
	}
	return a.evmClient.IsContract(ctx, address)
}

type etherscanResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

type etherscanTxResult struct {
	Hash            string `json:"hash"`
	From            string `json:"from"`
	To              string `json:"to"`
	Value           string `json:"value"`
	BlockNumber     string `json:"blockNumber"`
	BlockHash       string `json:"blockHash"`
	TimeStamp       string `json:"timeStamp"`
	TraceID         string `json:"traceId"`
	ContractAddress string `json:"contractAddress"`
	TokenSymbol     string `json:"tokenSymbol"`
	TokenDecimal    string `json:"tokenDecimal"`
}

type etherscanSourceCodeResult struct {
	ContractName string `json:"ContractName"`
	ABI          string `json:"ABI"`
}

func (a *EVMChainAdapter) ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	page := uint64(1)
	if cursor != "" {
		offset, err := strconv.ParseUint(cursor, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid cursor")
		}
		page = offset + 1
	}
	perSourceLimit := (limit + 2) / 3
	query := func(action string) url.Values {
		return url.Values{"module": {"account"}, "action": {action}, "chainid": {a.chainID}, "address": {strings.ToLower(address)}, "startblock": {"0"}, "endblock": {"999999999"}, "page": {strconv.FormatUint(page, 10)}, "offset": {strconv.FormatUint(uint64(perSourceLimit)+1, 10)}, "sort": {"desc"}}
	}
	normal, normalMore, err := a.listTransferSource(ctx, query("txlist"), perSourceLimit)
	if err != nil {
		return nil, err
	}
	internal, internalMore, err := a.listTransferSource(ctx, query("txlistinternal"), perSourceLimit)
	if err != nil {
		return nil, err
	}
	tokens, tokenMore, err := a.listTransferSource(ctx, query("tokentx"), perSourceLimit)
	if err != nil {
		return nil, err
	}
	transfers := make([]TransferItem, 0, len(normal)+len(internal)+len(tokens))
	for _, item := range normal {
		transfer, err := nativeTransfer(item, "tx", "NATIVE")
		if err != nil {
			return nil, err
		}
		transfer.Asset = a.NativeAsset()
		transfers = append(transfers, transfer)
	}
	for _, item := range internal {
		if item.To == "" {
			item.To = item.ContractAddress
		}
		transfer, err := nativeTransfer(item, "trace:"+item.TraceID, "INTERNAL")
		if err != nil {
			return nil, err
		}
		if transfer.EventID == "trace:" {
			return nil, fmt.Errorf("Etherscan internal transfer is missing traceId")
		}
		transfer.Asset = a.NativeAsset()
		transfers = append(transfers, transfer)
	}
	tokenTransfers, err := a.tokenTransfers(ctx, tokens)
	if err != nil {
		return nil, err
	}
	for _, transfer := range tokenTransfers {
		transfers = append(transfers, transfer)
	}
	sortTransfers(transfers)
	nextCursor := ""
	hasMore := normalMore || internalMore || tokenMore
	if hasMore {
		nextCursor = strconv.FormatUint(page, 10)
	}
	return &TransferPage{Transfers: transfers, NextCursor: nextCursor, HasMore: hasMore, SourceStatus: PageStatus(a.SourceStatus(), hasMore)}, nil
}

func (a *EVMChainAdapter) listTransferSource(ctx context.Context, query url.Values, limit uint32) ([]etherscanTxResult, bool, error) {
	var response etherscanResponse
	if err := a.get(ctx, query, &response); err != nil {
		return nil, false, err
	}
	if response.Status != "1" && !strings.EqualFold(response.Message, "No transactions found") {
		if strings.Contains(strings.ToLower(response.Message), "rate limit") {
			a.metrics.failure()
			return nil, false, NewProviderRateLimitError(EtherscanSource)
		}
		return nil, false, fmt.Errorf("Etherscan transfer history: %s", response.Message)
	}
	var items []etherscanTxResult
	if len(response.Result) > 0 && string(response.Result) != "null" {
		if err := json.Unmarshal(response.Result, &items); err != nil {
			return nil, false, fmt.Errorf("decode Etherscan transfer history: %w", err)
		}
	}
	hasMore := len(items) > int(limit)
	if hasMore {
		items = items[:limit]
	}
	return items, hasMore, nil
}

func nativeTransfer(item etherscanTxResult, eventID, kind string) (TransferItem, error) {
	if item.Hash == "" || item.From == "" || item.To == "" {
		return TransferItem{}, fmt.Errorf("Etherscan %s transfer is missing an address or hash", strings.ToLower(kind))
	}
	block, timestamp, err := transferPosition(item)
	if err != nil {
		return TransferItem{}, err
	}
	if _, ok := new(big.Int).SetString(item.Value, 10); !ok {
		return TransferItem{}, fmt.Errorf("parse Etherscan %s transfer value", strings.ToLower(kind))
	}
	return TransferItem{Hash: strings.ToLower(item.Hash), EventID: eventID, TransferKind: kind, From: strings.ToLower(item.From), To: strings.ToLower(item.To), AmountBaseUnits: item.Value, Asset: Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, BlockNumber: block, BlockHash: strings.ToLower(item.BlockHash), Timestamp: time.Unix(timestamp, 0)}, nil
}

func (a *EVMChainAdapter) tokenTransfers(ctx context.Context, items []etherscanTxResult) ([]TransferItem, error) {
	eventIDs := a.tokenEventIDs(ctx, items)
	transfers := make([]TransferItem, 0, len(items))
	for index, item := range items {
		transfer, err := tokenTransfer(item, eventIDs[index])
		if err != nil {
			return nil, err
		}
		transfers = append(transfers, transfer)
	}
	return transfers, nil
}

func tokenTransfer(item etherscanTxResult, eventID string) (TransferItem, error) {
	if item.ContractAddress == "" || item.Hash == "" || item.From == "" || item.To == "" {
		return TransferItem{}, fmt.Errorf("Etherscan ERC-20 transfer is missing an address or hash")
	}
	block, timestamp, err := transferPosition(item)
	if err != nil {
		return TransferItem{}, err
	}
	decimals, err := strconv.ParseUint(item.TokenDecimal, 10, 32)
	if err != nil {
		return TransferItem{}, fmt.Errorf("parse Etherscan token decimals: %w", err)
	}
	if _, ok := new(big.Int).SetString(item.Value, 10); !ok {
		return TransferItem{}, fmt.Errorf("parse Etherscan ERC-20 transfer value")
	}
	return TransferItem{Hash: strings.ToLower(item.Hash), EventID: eventID, TransferKind: "ERC20", From: strings.ToLower(item.From), To: strings.ToLower(item.To), AmountBaseUnits: item.Value, Asset: Asset{Kind: "ERC20", ContractAddress: strings.ToLower(item.ContractAddress), Symbol: item.TokenSymbol, Decimals: uint32(decimals)}, BlockNumber: block, BlockHash: strings.ToLower(item.BlockHash), Timestamp: time.Unix(timestamp, 0)}, nil
}

func (a *EVMChainAdapter) tokenEventIDs(ctx context.Context, items []etherscanTxResult) []string {
	ids := make([]string, len(items))
	byHash := make(map[string][]int)
	for index, item := range items {
		byHash[strings.ToLower(item.Hash)] = append(byHash[strings.ToLower(item.Hash)], index)
	}
	for hash, indexes := range byHash {
		logs, err := a.receiptLogs(ctx, hash)
		if err != nil {
			for ordinal, index := range indexes {
				ids[index] = providerEventID(items[index], ordinal)
			}
			continue
		}
		used := make(map[string]int)
		for _, index := range indexes {
			item := items[index]
			key := tokenFingerprint(item)
			matches := matchingTransferLogs(logs, item)
			if position := used[key]; position < len(matches) {
				ids[index] = "log:" + matches[position].LogIndex
				used[key]++
			} else {
				ids[index] = providerEventID(item, position)
				used[key]++
			}
		}
	}
	return ids
}

func (a *EVMChainAdapter) receiptLogs(ctx context.Context, hash string) ([]LogItem, error) {
	if a.evmClient == nil {
		return nil, fmt.Errorf("Ethereum RPC is unavailable")
	}
	return a.evmClient.GetTransactionReceiptLogs(ctx, hash)
}

func matchingTransferLogs(logs []LogItem, item etherscanTxResult) []LogItem {
	matches := make([]LogItem, 0, 1)
	for _, log := range logs {
		if len(log.Topics) < 3 || !strings.EqualFold(log.Topics[0], TransferEventTopic) || !strings.EqualFold(log.Address, item.ContractAddress) || !strings.EqualFold(topicAddress(log.Topics[1]), item.From) || !strings.EqualFold(topicAddress(log.Topics[2]), item.To) || !hexAmountEquals(log.Data, item.Value) {
			continue
		}
		if log.LogIndex != "" {
			matches = append(matches, log)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return logIndexNumber(matches[i].LogIndex) < logIndexNumber(matches[j].LogIndex) })
	return matches
}

func logIndexNumber(value string) uint64 {
	index, err := strconv.ParseUint(strings.TrimPrefix(value, "0x"), 16, 64)
	if err != nil {
		return ^uint64(0)
	}
	return index
}

func topicAddress(topic string) string {
	value := strings.TrimPrefix(strings.ToLower(topic), "0x")
	if len(value) < 40 {
		return ""
	}
	return "0x" + value[len(value)-40:]
}

func hexAmountEquals(value, decimal string) bool {
	hexValue := strings.TrimPrefix(value, "0x")
	amount, ok := new(big.Int).SetString(hexValue, 16)
	return ok && amount.String() == decimal
}

func tokenFingerprint(item etherscanTxResult) string {
	return strings.Join([]string{strings.ToLower(item.Hash), strings.ToLower(item.ContractAddress), strings.ToLower(item.From), strings.ToLower(item.To), item.Value}, ":")
}

func providerEventID(item etherscanTxResult, ordinal int) string {
	digest := sha256.Sum256([]byte(tokenFingerprint(item)))
	return fmt.Sprintf("provider:%x:%d", digest[:8], ordinal)
}

func transferPosition(item etherscanTxResult) (int64, int64, error) {
	block, err := strconv.ParseInt(item.BlockNumber, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse Etherscan block number: %w", err)
	}
	timestamp, err := strconv.ParseInt(item.TimeStamp, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse Etherscan timestamp: %w", err)
	}
	return block, timestamp, nil
}

func sortTransfers(transfers []TransferItem) {
	sort.Slice(transfers, func(i, j int) bool {
		if transfers[i].BlockNumber != transfers[j].BlockNumber {
			return transfers[i].BlockNumber > transfers[j].BlockNumber
		}
		if !transfers[i].Timestamp.Equal(transfers[j].Timestamp) {
			return transfers[i].Timestamp.After(transfers[j].Timestamp)
		}
		if transfers[i].Hash != transfers[j].Hash {
			return transfers[i].Hash < transfers[j].Hash
		}
		return transfers[i].EventID < transfers[j].EventID
	})
}

func (a *EVMChainAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var response etherscanResponse
	if err := a.get(ctx, url.Values{"module": {"proxy"}, "action": {"eth_getTransactionByHash"}, "chainid": {a.chainID}, "txhash": {hash}}, &response); err != nil {
		return nil, SourceStatus{}, err
	}
	var item struct {
		Hash        string `json:"hash"`
		From        string `json:"from"`
		To          string `json:"to"`
		Value       string `json:"value"`
		BlockNumber string `json:"blockNumber"`
	}
	if len(response.Result) == 0 || string(response.Result) == "null" || json.Unmarshal(response.Result, &item) != nil || item.Hash == "" || item.From == "" || item.To == "" {
		return nil, SourceStatus{}, fmt.Errorf("transaction not found")
	}
	block, err := strconv.ParseInt(strings.TrimPrefix(item.BlockNumber, "0x"), 16, 64)
	if err != nil {
		return nil, SourceStatus{}, fmt.Errorf("parse Etherscan transaction block: %w", err)
	}
	value, ok := new(big.Int).SetString(strings.TrimPrefix(item.Value, "0x"), 16)
	if !ok {
		return nil, SourceStatus{}, fmt.Errorf("parse Etherscan transaction value")
	}
	return &TransactionItem{Hash: strings.ToLower(item.Hash), From: strings.ToLower(item.From), To: strings.ToLower(item.To), ValueBaseUnits: value.String(), AssetSymbol: a.NativeAsset().Symbol, BlockNumber: block}, a.SourceStatus(), nil
}

func (a *EVMChainAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	var response etherscanResponse
	if err := a.get(ctx, url.Values{"module": {"contract"}, "action": {"getsourcecode"}, "chainid": {a.chainID}, "address": {strings.ToLower(address)}}, &response); err != nil {
		return nil, err
	}
	var values []etherscanSourceCodeResult
	if err := json.Unmarshal(response.Result, &values); err != nil || len(values) == 0 || values[0].ContractName == "" {
		return &ContractMetadata{Category: "EOA"}, nil
	}
	return &ContractMetadata{ContractName: values[0].ContractName, IsVerified: values[0].ABI != "Contract source code not verified", Category: "CONTRACT"}, nil
}

func (a *EVMChainAdapter) SourceStatus() SourceStatus {
	return SourceStatus{Source: EtherscanSource, RetrievedAt: time.Now().UTC(), IsComplete: true}
}

func (a *EVMChainAdapter) get(ctx context.Context, query url.Values, output *etherscanResponse) error {
	a.requestMu.Lock()
	defer a.requestMu.Unlock()
	delay := time.Until(a.lastRequest.Add(etherscanRequestGap))
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
	query.Set("apikey", a.apiKey)
	requestURL, err := url.Parse(a.apiURL)
	if err != nil {
		return err
	}
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return err
	}
	response, err := a.httpClient.Do(request)
	if err != nil {
		a.metrics.failure()
		return NewProviderTransportError(EtherscanSource, err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		a.metrics.failure()
		return fmt.Errorf("read Etherscan response: %w", err)
	}
	recordAcquisition(ctx, EtherscanSource, request, body)
	if response.StatusCode != http.StatusOK {
		a.metrics.failure()
		return NewProviderHTTPError(EtherscanSource, response)
	}
	if err := json.Unmarshal(body, output); err != nil {
		a.metrics.failure()
		return fmt.Errorf("decode Etherscan response: %w", err)
	}
	a.metrics.success()
	return nil
}

func (a *EVMChainAdapter) ProviderHealth() []ProviderHealth {
	health := []ProviderHealth{a.metrics.snapshot()}
	if a.evmClient != nil {
		health = append(health, a.evmClient.ProviderHealth())
	}
	return health
}
