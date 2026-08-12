package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

const rpcRequestGap = time.Second / 10

type EVMClient struct {
	rpcURL      string
	httpClient  *http.Client
	requestMu   sync.Mutex
	lastRequest time.Time
	metrics     *providerMetrics
}

func NewEVMClient(rpcURL string) *EVMClient {
	return &EVMClient{
		rpcURL: rpcURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
		metrics: newProviderMetrics("evm-rpc", int(time.Second/rpcRequestGap)),
	}
}

type RPCRequest struct {
	JSONRPC string        `json:"jsonrpc"`
	Method  string        `json:"method"`
	Params  []interface{} `json:"params"`
	ID      int           `json:"id"`
}

type RPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *RPCError       `json:"error"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (c *EVMClient) callRPC(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	rpcCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := c.waitForRequest(rpcCtx); err != nil {
		return nil, err
	}

	reqBody := RPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
		ID:      1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(rpcCtx, "POST", c.rpcURL, bytes.NewBuffer(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.metrics.failure()
		return nil, NewProviderTransportError("evm-rpc", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		c.metrics.failure()
		return nil, err
	}
	recordAcquisition(ctx, "evm-rpc", req, body)
	if resp.StatusCode != http.StatusOK {
		c.metrics.failure()
		return nil, NewProviderHTTPError("evm-rpc", resp)
	}

	var rpcResp RPCResponse
	if err := json.Unmarshal(body, &rpcResp); err != nil {
		c.metrics.failure()
		return nil, fmt.Errorf("rpc parse error: %w, raw: %s", err, string(body))
	}

	if rpcResp.Error != nil {
		c.metrics.failure()
		return nil, fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	c.metrics.success()
	return rpcResp.Result, nil
}

func (c *EVMClient) waitForRequest(ctx context.Context) error {
	c.requestMu.Lock()
	defer c.requestMu.Unlock()
	delay := time.Until(c.lastRequest.Add(rpcRequestGap))
	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	c.metrics.request(delay)
	// ponytail: one shared RPC budget; split by configured network only when multi-network support exists.
	c.lastRequest = time.Now()
	return nil
}

func (c *EVMClient) ProviderHealth() ProviderHealth { return c.metrics.snapshot() }

func (c *EVMClient) GetLatestBlockNumber(ctx context.Context) (uint64, error) {
	raw, err := c.callRPC(ctx, "eth_blockNumber", []interface{}{})
	if err != nil {
		return 0, err
	}

	var hexStr string
	if err := json.Unmarshal(raw, &hexStr); err != nil {
		return 0, err
	}

	hexStr = strings.TrimPrefix(hexStr, "0x")
	val := new(big.Int)
	val.SetString(hexStr, 16)
	return val.Uint64(), nil
}

func (c *EVMClient) GetBalance(ctx context.Context, address string) (*big.Int, error) {
	raw, err := c.callRPC(ctx, "eth_getBalance", []interface{}{address, "latest"})
	if err != nil {
		return nil, err
	}

	var hexStr string
	if err := json.Unmarshal(raw, &hexStr); err != nil {
		return nil, err
	}

	val := new(big.Int)
	hexStr = strings.TrimPrefix(hexStr, "0x")
	val.SetString(hexStr, 16)
	return val, nil
}

func (c *EVMClient) GetTxCount(ctx context.Context, address string) (uint64, error) {
	raw, err := c.callRPC(ctx, "eth_getTransactionCount", []interface{}{address, "latest"})
	if err != nil {
		return 0, err
	}

	var hexStr string
	if err := json.Unmarshal(raw, &hexStr); err != nil {
		return 0, err
	}

	hexStr = strings.TrimPrefix(hexStr, "0x")
	val := new(big.Int)
	val.SetString(hexStr, 16)
	return val.Uint64(), nil
}

func (c *EVMClient) IsContract(ctx context.Context, address string) (bool, error) {
	raw, err := c.callRPC(ctx, "eth_getCode", []interface{}{address, "latest"})
	if err != nil {
		return false, err
	}

	var hexStr string
	if err := json.Unmarshal(raw, &hexStr); err != nil {
		return false, err
	}

	clean := strings.ToLower(strings.TrimPrefix(hexStr, "0x"))
	// EIP-7702 EOA Delegated Accounts start with 0xef0100 prefix; treat them as EOAs
	if clean == "" || clean == "0" || strings.HasPrefix(clean, "ef0100") {
		return false, nil
	}

	return len(clean) > 0, nil
}

type LogItem struct {
	Address          string   `json:"address"`
	Topics           []string `json:"topics"`
	Data             string   `json:"data"`
	BlockNumber      string   `json:"blockNumber"`
	TransactionHash  string   `json:"transactionHash"`
	TransactionIndex string   `json:"transactionIndex"`
	LogIndex         string   `json:"logIndex"`
}

func (c *EVMClient) GetTransactionReceiptLogs(ctx context.Context, hash string) ([]LogItem, error) {
	raw, err := c.callRPC(ctx, "eth_getTransactionReceipt", []interface{}{hash})
	if err != nil {
		return nil, err
	}
	var receipt struct {
		Logs []LogItem `json:"logs"`
	}
	if string(raw) == "null" || json.Unmarshal(raw, &receipt) != nil {
		return nil, fmt.Errorf("transaction receipt is unavailable")
	}
	return receipt.Logs, nil
}

// ERC20 Transfer event signature: Transfer(address,address,uint256)
const TransferEventTopic = "0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef"

func (c *EVMClient) GetERC20Transfers(ctx context.Context, address string, fromBlockHex string) ([]LogItem, error) {
	padAddress := "0x000000000000000000000000" + strings.TrimPrefix(strings.ToLower(address), "0x")

	if fromBlockHex == "" || fromBlockHex == "0x0" {
		latest, err := c.GetLatestBlockNumber(ctx)
		if err == nil && latest > 45000 {
			fromBlockHex = fmt.Sprintf("0x%x", latest-45000)
		} else {
			fromBlockHex = "0x1"
		}
	}

	filterInbound := map[string]interface{}{
		"fromBlock": fromBlockHex,
		"toBlock":   "latest",
		"topics":    []interface{}{TransferEventTopic, nil, padAddress},
	}

	filterOutbound := map[string]interface{}{
		"fromBlock": fromBlockHex,
		"toBlock":   "latest",
		"topics":    []interface{}{TransferEventTopic, padAddress, nil},
	}

	inLogsRaw, _ := c.callRPC(ctx, "eth_getLogs", []interface{}{filterInbound})
	outLogsRaw, _ := c.callRPC(ctx, "eth_getLogs", []interface{}{filterOutbound})

	var logs []LogItem
	if len(inLogsRaw) > 0 {
		var inLogs []LogItem
		_ = json.Unmarshal(inLogsRaw, &inLogs)
		logs = append(logs, inLogs...)
	}

	if len(outLogsRaw) > 0 {
		var outLogs []LogItem
		_ = json.Unmarshal(outLogsRaw, &outLogs)
		logs = append(logs, outLogs...)
	}

	return logs, nil
}

func FormatWeiToETH(wei *big.Int) string {
	if wei == nil {
		return "0.0 ETH"
	}
	return FormatAmount(wei, Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18})
}
