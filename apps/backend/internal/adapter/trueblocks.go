package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	trueBlocksSource = "trueblocks-6.5.0"
)

type SourceStatus struct {
	Source           string
	RetrievedAt      time.Time
	IndexedUpToBlock int64
	LatestChainBlock int64
	IsComplete       bool
	Warning          string
}

type TransferPage struct {
	Transactions []TransactionItem
	NextCursor   string
	HasMore      bool
	SourceStatus SourceStatus
}

type TrueBlocksClient struct {
	baseURL    *url.URL
	httpClient *http.Client
}

func NewTrueBlocksClient(rawURL string) (*TrueBlocksClient, error) {
	baseURL, err := url.Parse(rawURL)
	if err != nil || baseURL.Host == "" || (baseURL.Scheme != "http" && baseURL.Scheme != "https") {
		return nil, fmt.Errorf("TRUEBLOCKS_API_URL must be an absolute HTTP URL")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/")
	return &TrueBlocksClient{baseURL: baseURL, httpClient: &http.Client{}}, nil
}

type trueBlocksEnvelope struct {
	Data []trueBlocksTransaction `json:"data"`
}
type trueBlocksTransaction struct {
	Hash        string      `json:"hash"`
	BlockNumber json.Number `json:"blockNumber"`
	Timestamp   json.Number `json:"timestamp"`
	From        string      `json:"from"`
	To          string      `json:"to"`
	Value       json.Number `json:"value"`
}

func (c *TrueBlocksClient) ListNativeTransfers(ctx context.Context, address string, limit uint32, cursor uint64, latestBlock uint64) (*TransferPage, error) {
	query := url.Values{}
	query.Set("addrs", address)
	query.Set("firstRecord", strconv.FormatUint(cursor, 10))
	query.Set("maxRecords", strconv.FormatUint(uint64(limit)+1, 10))
	query.Set("reversed", "true")
	var response trueBlocksEnvelope
	if err := c.get(ctx, "/export", query, &response); err != nil {
		return nil, err
	}
	status := c.Status(ctx, latestBlock)
	hasMore := len(response.Data) > int(limit)
	if hasMore {
		response.Data = response.Data[:limit]
	}
	transactions := make([]TransactionItem, 0, len(response.Data))
	for _, item := range response.Data {
		if item.Hash == "" || item.From == "" || item.To == "" {
			continue
		}
		block, _ := item.BlockNumber.Int64()
		timestamp, _ := item.Timestamp.Int64()
		transactions = append(transactions, TransactionItem{Hash: strings.ToLower(item.Hash), From: strings.ToLower(item.From), To: strings.ToLower(item.To), ValueWei: item.Value.String(), AssetSymbol: "ETH", BlockNumber: block, Timestamp: time.Unix(timestamp, 0), IsContract: false})
	}
	next := ""
	if hasMore {
		next = strconv.FormatUint(cursor+uint64(limit), 10)
	}
	return &TransferPage{Transactions: transactions, NextCursor: next, HasMore: hasMore, SourceStatus: status}, nil
}

func (c *TrueBlocksClient) LookupTransaction(ctx context.Context, hash string, latestBlock uint64) (*TransactionItem, SourceStatus, error) {
	query := url.Values{}
	query.Set("transactions", hash)
	var response trueBlocksEnvelope
	if err := c.get(ctx, "/transactions", query, &response); err != nil {
		return nil, SourceStatus{}, err
	}
	if len(response.Data) != 1 {
		return nil, SourceStatus{}, fmt.Errorf("transaction not found")
	}
	item := response.Data[0]
	block, _ := item.BlockNumber.Int64()
	timestamp, _ := item.Timestamp.Int64()
	transaction := &TransactionItem{Hash: strings.ToLower(item.Hash), From: strings.ToLower(item.From), To: strings.ToLower(item.To), ValueWei: item.Value.String(), AssetSymbol: "ETH", BlockNumber: block, Timestamp: time.Unix(timestamp, 0)}
	return transaction, c.Status(ctx, latestBlock), nil
}

func (c *TrueBlocksClient) Status(ctx context.Context, latestBlock uint64) SourceStatus {
	status := SourceStatus{Source: trueBlocksSource, RetrievedAt: time.Now().UTC(), LatestChainBlock: int64(latestBlock), Warning: "TrueBlocks index height is unavailable; results are not marked complete."}
	var response map[string]any
	if err := c.get(ctx, "/status", url.Values{"healthcheck": {"true"}}, &response); err != nil {
		status.Warning = "TrueBlocks status unavailable; results are not marked complete."
		return status
	}
	if indexed, ok := findBlockHeight(response); ok {
		status.IndexedUpToBlock = indexed
		status.IsComplete = latestBlock > 0 && uint64(indexed) >= latestBlock
		if status.IsComplete {
			status.Warning = ""
		} else {
			status.Warning = "TrueBlocks index is behind the configured RPC head."
		}
	}
	return status
}

func (c *TrueBlocksClient) get(ctx context.Context, endpoint string, query url.Values, output any) error {
	requestURL := *c.baseURL
	requestURL.Path += endpoint
	requestURL.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL.String(), nil)
	if err != nil {
		return err
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("trueblocks request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("trueblocks returned %s", response.Status)
	}
	decoder := json.NewDecoder(io.LimitReader(response.Body, 4<<20))
	decoder.UseNumber()
	if err := decoder.Decode(output); err != nil {
		return fmt.Errorf("decode trueblocks response: %w", err)
	}
	return nil
}

func findBlockHeight(value any) (int64, bool) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if strings.EqualFold(key, "indexedUpToBlock") || strings.EqualFold(key, "lastBlock") {
				if number, ok := child.(json.Number); ok {
					parsed, err := number.Int64()
					return parsed, err == nil
				}
			}
			if height, ok := findBlockHeight(child); ok {
				return height, true
			}
		}
	case []any:
		for _, child := range value {
			if height, ok := findBlockHeight(child); ok {
				return height, true
			}
		}
	}
	return 0, false
}
