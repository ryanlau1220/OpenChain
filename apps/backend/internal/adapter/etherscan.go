package adapter

import (
	"context"
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
	EtherscanAPIURL     = "https://api.etherscan.io/v2/api"
	etherscanRequestGap = time.Second / 5
)

type EVMChainAdapter struct {
	apiURL     string
	apiKey     string
	evmClient  *EVMClient
	httpClient *http.Client

	requestMu   sync.Mutex
	lastRequest time.Time
}

func NewEVMChainAdapter(apiURL, apiKey string, evmClient *EVMClient) *EVMChainAdapter {
	return &EVMChainAdapter{apiURL: apiURL, apiKey: apiKey, evmClient: evmClient, httpClient: &http.Client{Timeout: 15 * time.Second}}
}

func (a *EVMChainAdapter) Network() string { return "ethereum-mainnet" }

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
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	BlockNumber string `json:"blockNumber"`
	TimeStamp   string `json:"timeStamp"`
}

type etherscanSourceCodeResult struct {
	ContractName string `json:"ContractName"`
	ABI          string `json:"ABI"`
}

func (a *EVMChainAdapter) ListNativeTransfers(ctx context.Context, address string, limit uint32, cursor uint64) (*TransferPage, error) {
	if limit == 0 {
		limit = 25
	}
	page := cursor + 1
	var response etherscanResponse
	if err := a.get(ctx, url.Values{"module": {"account"}, "action": {"txlist"}, "chainid": {"1"}, "address": {strings.ToLower(address)}, "startblock": {"0"}, "endblock": {"999999999"}, "page": {strconv.FormatUint(page, 10)}, "offset": {strconv.FormatUint(uint64(limit)+1, 10)}, "sort": {"desc"}}, &response); err != nil {
		return nil, err
	}
	if response.Status != "1" && !strings.EqualFold(response.Message, "No transactions found") {
		return nil, fmt.Errorf("Etherscan transaction history: %s", response.Message)
	}
	var rawTransactions []etherscanTxResult
	if len(response.Result) > 0 && string(response.Result) != "null" {
		if err := json.Unmarshal(response.Result, &rawTransactions); err != nil {
			return nil, fmt.Errorf("decode Etherscan transaction history: %w", err)
		}
	}
	hasMore := len(rawTransactions) > int(limit)
	if hasMore {
		rawTransactions = rawTransactions[:limit]
	}
	transactions := make([]TransactionItem, 0, len(rawTransactions))
	for _, item := range rawTransactions {
		if item.Hash == "" || item.From == "" || item.To == "" {
			continue
		}
		block, err := strconv.ParseInt(item.BlockNumber, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse Etherscan block number: %w", err)
		}
		timestamp, err := strconv.ParseInt(item.TimeStamp, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse Etherscan timestamp: %w", err)
		}
		if _, ok := new(big.Int).SetString(item.Value, 10); !ok {
			return nil, fmt.Errorf("parse Etherscan transaction value")
		}
		transactions = append(transactions, TransactionItem{Hash: strings.ToLower(item.Hash), From: strings.ToLower(item.From), To: strings.ToLower(item.To), ValueWei: item.Value, AssetSymbol: "ETH", BlockNumber: block, Timestamp: time.Unix(timestamp, 0)})
	}
	nextCursor := ""
	if hasMore {
		nextCursor = strconv.FormatUint(page, 10)
	}
	return &TransferPage{Transactions: transactions, NextCursor: nextCursor, HasMore: hasMore, SourceStatus: a.SourceStatus()}, nil
}

func (a *EVMChainAdapter) LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error) {
	var response etherscanResponse
	if err := a.get(ctx, url.Values{"module": {"proxy"}, "action": {"eth_getTransactionByHash"}, "txhash": {hash}}, &response); err != nil {
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
	return &TransactionItem{Hash: strings.ToLower(item.Hash), From: strings.ToLower(item.From), To: strings.ToLower(item.To), ValueWei: value.String(), AssetSymbol: "ETH", BlockNumber: block}, a.SourceStatus(), nil
}

func (a *EVMChainAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	var response etherscanResponse
	if err := a.get(ctx, url.Values{"module": {"contract"}, "action": {"getsourcecode"}, "chainid": {"1"}, "address": {strings.ToLower(address)}}, &response); err != nil {
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
	if delay := time.Until(a.lastRequest.Add(etherscanRequestGap)); delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
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
		return fmt.Errorf("Etherscan request failed")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Etherscan returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode Etherscan response: %w", err)
	}
	return nil
}
