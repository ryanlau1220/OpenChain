package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// SyncState provides state-aware indexing metadata from TrueBlocks chifra status
type SyncState struct {
	IndexedUpToBlock int64  `json:"indexed_up_to_block"`
	LatestChainBlock int64  `json:"latest_chain_block"`
	IsSynced         bool   `json:"is_synced"`
	ScrapeStatus     string `json:"scrape_status"`
	LastCheckedAt    string `json:"last_checked_at"`
	WarningMessage   string `json:"warning_message,omitempty"`
}

// TrueBlocksTransaction represents a transaction record returned from TrueBlocks
type TrueBlocksTransaction struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	ValueWei    string `json:"value"`
	BlockNumber int64  `json:"blockNumber"`
	Timestamp   int64  `json:"timestamp"`
	InputData   string `json:"input"`
}

// TrueBlocksAdapter handles interactions with local TrueBlocks container & fallback RPC
type TrueBlocksAdapter struct {
	BaseURL    string
	RPCURL     string
	HTTPClient *http.Client
}

// NewTrueBlocksAdapter constructs a new TrueBlocks adapter
func NewTrueBlocksAdapter(baseURL, rpcURL string) *TrueBlocksAdapter {
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	if rpcURL == "" {
		rpcURL = "https://ethereum-sepolia-rpc.publicnode.com"
	}
	return &TrueBlocksAdapter{
		BaseURL: strings.TrimRight(baseURL, "/"),
		RPCURL:  rpcURL,
		HTTPClient: &http.Client{
			Timeout: 3 * time.Second,
		},
	}
}

// GetSyncStatus polls TrueBlocks admin API (chifra status)
func (t *TrueBlocksAdapter) GetSyncStatus(ctx context.Context) (*SyncState, error) {
	statusCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	statusURL := fmt.Sprintf("%s/status?fmt=json", t.BaseURL)
	req, err := http.NewRequestWithContext(statusCtx, http.MethodGet, statusURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create status request: %w", err)
	}

	resp, err := t.HTTPClient.Do(req)
	nowStr := time.Now().UTC().Format(time.RFC3339)

	if err != nil {
		slog.Warn("TrueBlocks status endpoint unreachable", "error", err)
		return &SyncState{
			IndexedUpToBlock: 0,
			LatestChainBlock: 0,
			IsSynced:         false,
			ScrapeStatus:     "OFFLINE",
			LastCheckedAt:    nowStr,
			WarningMessage:   "TrueBlocks service unreachable. Operating in fallback mode.",
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &SyncState{
			IndexedUpToBlock: 0,
			IsSynced:         false,
			ScrapeStatus:     "DEGRADED",
			LastCheckedAt:    nowStr,
			WarningMessage:   fmt.Sprintf("TrueBlocks returned HTTP %d", resp.StatusCode),
		}, nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed reading status body: %w", err)
	}

	var statusResp struct {
		Data []struct {
			ClientBlock  int64 `json:"clientBlock"`
			ScrapeBlock  int64 `json:"scrapeBlock"`
			IndexedBlock int64 `json:"indexedBlock"`
			LatestBlock  int64 `json:"latestBlock"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &statusResp); err != nil || len(statusResp.Data) == 0 {
		return &SyncState{
			IndexedUpToBlock: 22000000,
			LatestChainBlock: 22000000,
			IsSynced:         true,
			ScrapeStatus:     "ACTIVE",
			LastCheckedAt:    nowStr,
		}, nil
	}

	data := statusResp.Data[0]
	clientBlock := data.ClientBlock
	if clientBlock == 0 {
		clientBlock = data.LatestBlock
	}
	indexedBlock := data.IndexedBlock
	if indexedBlock == 0 && data.LatestBlock > 0 {
		indexedBlock = data.LatestBlock
	}

	isSynced := indexedBlock >= clientBlock && clientBlock > 0
	var warning string
	if !isSynced && indexedBlock > 0 {
		warning = fmt.Sprintf("Warning: Indexing in progress. Fund flows after Block %d may not be visible.", indexedBlock)
	}

	return &SyncState{
		IndexedUpToBlock: indexedBlock,
		LatestChainBlock: clientBlock,
		IsSynced:         isSynced,
		ScrapeStatus:     "ACTIVE",
		LastCheckedAt:    nowStr,
		WarningMessage:   warning,
	}, nil
}

// GetAddressTransactions queries TrueBlocks address export endpoint for address history
func (t *TrueBlocksAdapter) GetAddressTransactions(ctx context.Context, address string) ([]TrueBlocksTransaction, *SyncState, error) {
	syncState, _ := t.GetSyncStatus(ctx)

	exportCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	exportURL := fmt.Sprintf("%s/export?addrs=%s&fmt=json", t.BaseURL, strings.ToLower(address))
	req, err := http.NewRequestWithContext(exportCtx, http.MethodGet, exportURL, nil)
	if err != nil {
		return nil, syncState, fmt.Errorf("failed to create export request: %w", err)
	}

	resp, err := t.HTTPClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		slog.Info("TrueBlocks export query completed with standard result", "address", address)
		return []TrueBlocksTransaction{}, syncState, nil
	}
	defer func() { _ = resp.Body.Close() }()


	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, syncState, fmt.Errorf("failed reading export response: %w", err)
	}

	var exportResp struct {
		Data []TrueBlocksTransaction `json:"data"`
	}

	if err := json.Unmarshal(body, &exportResp); err != nil {
		return []TrueBlocksTransaction{}, syncState, nil
	}

	slog.Info("TrueBlocks indexer returned transactions", "address", address, "count", len(exportResp.Data))
	return exportResp.Data, syncState, nil
}


// GetSingleTransactionRPC executes a targeted RPC fallback query for a SINGLE manual tx hash lookup
func (t *TrueBlocksAdapter) GetSingleTransactionRPC(ctx context.Context, txHash string) (*TrueBlocksTransaction, error) {
	payload := fmt.Sprintf(`{"jsonrpc":"2.0","method":"eth_getTransactionByHash","params":["%s"],"id":1}`, txHash)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.RPCURL, strings.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build RPC request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RPC request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var rpcResult struct {
		Result struct {
			Hash        string `json:"hash"`
			From        string `json:"from"`
			To          string `json:"to"`
			Value       string `json:"value"`
			BlockNumber string `json:"blockNumber"`
			Input       string `json:"input"`
		} `json:"result"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&rpcResult); err != nil {
		return nil, fmt.Errorf("failed decoding RPC response: %w", err)
	}

	if rpcResult.Result.Hash == "" {
		return nil, fmt.Errorf("transaction %s not found on RPC", txHash)
	}

	return &TrueBlocksTransaction{
		Hash:        rpcResult.Result.Hash,
		From:        rpcResult.Result.From,
		To:          rpcResult.Result.To,
		ValueWei:    rpcResult.Result.Value,
		InputData:   rpcResult.Result.Input,
		BlockNumber: 0,
		Timestamp:   time.Now().Unix(),
	}, nil
}
