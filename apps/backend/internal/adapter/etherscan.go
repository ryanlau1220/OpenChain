package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type EVMChainAdapter struct {
	network    string
	apiBaseURL string
	apiKey     string
	evmClient  *EVMClient
	httpClient *http.Client
}

func NewEVMChainAdapter(network string, apiBaseURL string, apiKey string, evmClient *EVMClient) *EVMChainAdapter {
	if apiBaseURL == "" {
		if strings.Contains(strings.ToUpper(network), "SEPOLIA") {
			apiBaseURL = "https://api-sepolia.etherscan.io/api"
		} else {
			apiBaseURL = "https://api.etherscan.io/api"
		}
	}
	return &EVMChainAdapter{
		network:    network,
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		apiKey:     apiKey,
		evmClient:  evmClient,
		httpClient: &http.Client{
			Timeout: 8 * time.Second,
		},
	}
}

func (a *EVMChainAdapter) Network() string {
	return a.network
}

func (a *EVMChainAdapter) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	if a.evmClient != nil {
		return a.evmClient.GetBalance(ctx, address)
	}
	return big.NewInt(0), nil
}

func (a *EVMChainAdapter) GetTxCount(ctx context.Context, address string) (uint64, error) {
	if a.evmClient != nil {
		return a.evmClient.GetTxCount(ctx, address)
	}
	return 0, nil
}

func (a *EVMChainAdapter) IsContract(ctx context.Context, address string) (bool, error) {
	if a.evmClient != nil {
		isC, err := a.evmClient.IsContract(ctx, address)
		if err == nil && isC {
			return true, nil
		}
	}
	meta, err := a.GetContractMetadata(ctx, address)
	if err == nil && meta != nil && meta.ContractName != "" {
		return true, nil
	}
	return false, nil
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
	TokenSymbol string `json:"tokenSymbol"`
}

type etherscanSourceCodeResult struct {
	ContractName string `json:"ContractName"`
	ABI          string `json:"ABI"`
}

func (a *EVMChainAdapter) GetAccountTransactions(ctx context.Context, address string, limit int) ([]TransactionItem, error) {
	if limit <= 0 {
		limit = 15
	}
	clean := strings.ToLower(strings.TrimSpace(address))
	if clean == "" {
		return nil, fmt.Errorf("empty address")
	}

	url := fmt.Sprintf("%s?module=account&action=txlist&address=%s&startblock=0&endblock=99999999&page=1&offset=%d&sort=desc", a.apiBaseURL, clean, limit)
	if a.apiKey != "" {
		url += "&apikey=" + a.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		slog.Warn("Etherscan API request failed", "address", clean, "error", err)
		return []TransactionItem{}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return []TransactionItem{}, nil
	}

	var apiResp etherscanResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return []TransactionItem{}, nil
	}

	var rawTxs []etherscanTxResult
	if err := json.Unmarshal(apiResp.Result, &rawTxs); err != nil {
		return []TransactionItem{}, nil
	}

	var txs []TransactionItem
	for _, tx := range rawTxs {
		if tx.From == "" || tx.To == "" {
			continue
		}
		blk, _ := strconv.ParseInt(tx.BlockNumber, 10, 64)
		tsSec, _ := strconv.ParseInt(tx.TimeStamp, 10, 64)
		sym := tx.TokenSymbol
		if sym == "" {
			sym = "ETH"
		}
		txs = append(txs, TransactionItem{
			Hash:        tx.Hash,
			From:        strings.ToLower(tx.From),
			To:          strings.ToLower(tx.To),
			ValueWei:    tx.Value,
			AssetSymbol: sym,
			BlockNumber: blk,
			Timestamp:   time.Unix(tsSec, 0),
		})
	}

	return txs, nil
}

func (a *EVMChainAdapter) GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error) {
	clean := strings.ToLower(strings.TrimSpace(address))
	url := fmt.Sprintf("%s?module=contract&action=getsourcecode&address=%s", a.apiBaseURL, clean)
	if a.apiKey != "" {
		url += "&apikey=" + a.apiKey
	}

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var apiResp etherscanResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, err
	}

	var codeResults []etherscanSourceCodeResult
	if err := json.Unmarshal(apiResp.Result, &codeResults); err != nil || len(codeResults) == 0 {
		return &ContractMetadata{ContractName: "", IsVerified: false, Category: "EOA"}, nil
	}

	res := codeResults[0]
	if res.ContractName == "" {
		return &ContractMetadata{ContractName: "", IsVerified: false, Category: "EOA"}, nil
	}

	cat := "CONTRACT"
	if strings.Contains(strings.ToLower(res.ContractName), "router") {
		cat = "DeFi"
	} else if strings.Contains(strings.ToLower(res.ContractName), "vault") || strings.Contains(strings.ToLower(res.ContractName), "pool") {
		cat = "DeFi Pool"
	}

	return &ContractMetadata{
		ContractName: res.ContractName,
		IsVerified:   res.ABI != "Contract source code not verified",
		Category:     cat,
	}, nil
}
