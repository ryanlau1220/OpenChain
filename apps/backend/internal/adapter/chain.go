package adapter

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"time"
)

type Asset struct {
	Kind, ContractAddress, Symbol string
	Decimals                      uint32
}

// TransferItem is one observed transfer event, never an inferred flow.
type TransferItem struct {
	Hash, EventID, TransferKind, From, To, AmountBaseUnits string
	Asset                                                  Asset
	BlockNumber                                            int64
	BlockHash                                              string
	Timestamp                                              time.Time
}

// TransactionItem represents a normalized transaction lookup response.
type TransactionItem struct {
	Hash             string    `json:"hash"`
	From             string    `json:"from"`
	To               string    `json:"to"`
	ValueBaseUnits   string    `json:"value_base_units"`
	AssetSymbol      string    `json:"asset_symbol"`
	BlockNumber      int64     `json:"block_number"`
	Timestamp        time.Time `json:"timestamp"`
	IsContract       bool      `json:"is_contract"`
	ContractName     string    `json:"contract_name,omitempty"`
	FirstTxTimestamp int64     `json:"first_tx_timestamp,omitempty"`
	LastTxTimestamp  int64     `json:"last_tx_timestamp,omitempty"`
}

const EtherscanSource = "etherscan-v2"

type SourceStatus struct {
	Source           string    `json:"source"`
	RetrievedAt      time.Time `json:"retrieved_at"`
	IndexedUpToBlock int64     `json:"indexed_up_to_block"`
	LatestChainBlock int64     `json:"latest_chain_block"`
	IsComplete       bool      `json:"is_complete"`
	Warning          string    `json:"warning"`
}

type TransferPage struct {
	Transfers    []TransferItem
	NextCursor   string
	HasMore      bool
	SourceStatus SourceStatus
}

// PageStatus describes the returned page, not the provider's overall service.
// A cursor means the observed history is necessarily incomplete.
func PageStatus(status SourceStatus, hasMore bool) SourceStatus {
	if hasMore {
		status.IsComplete = false
	}
	return status
}

// withEVMChainHead adds the chain height used to classify confirmations. A
// trace remains usable if the extra RPC call fails, but its observations must
// remain provisional rather than becoming evidence-backed by elapsed time.
func withEVMChainHead(ctx context.Context, status SourceStatus, client *EVMClient) SourceStatus {
	if client == nil {
		return withFinalityWarning(status)
	}
	latest, err := client.GetLatestBlockNumber(ctx)
	if err != nil || latest > math.MaxInt64 {
		return withFinalityWarning(status)
	}
	status.LatestChainBlock = int64(latest)
	return status
}

func withFinalityWarning(status SourceStatus) SourceStatus {
	const warning = "Chain confirmation height is unavailable; observations remain provisional."
	if status.Warning == "" {
		status.Warning = warning
	} else {
		status.Warning += " " + warning
	}
	return status
}

// ContractMetadata represents dynamically resolved smart contract metadata
type ContractMetadata struct {
	ContractName string `json:"contract_name"`
	IsVerified   bool   `json:"is_verified"`
	Category     string `json:"category"`
}

// ChainAdapter defines the abstract interface for multi-chain network adapters (EVM, Solana, Bitcoin, etc.)
type ChainAdapter interface {
	Network() string
	Capabilities() NetworkCapabilities
	NormalizeAddress(value string) (string, error)
	NormalizeTransactionHash(value string) (string, error)
	NativeAsset() Asset
	ActivityLabel() string
	GetBalance(ctx context.Context, address string) (*big.Int, error)
	GetTxCount(ctx context.Context, address string) (uint64, error)
	IsContract(ctx context.Context, address string) (bool, error)
	ListTransfers(ctx context.Context, address string, limit uint32, cursor string) (*TransferPage, error)
	LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error)
	GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error)
	SourceStatus() SourceStatus
}

func FormatAmount(value *big.Int, asset Asset) string {
	if value == nil {
		if asset.Symbol == "" {
			return "0"
		}
		return "0 " + asset.Symbol
	}
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(asset.Decimals)), nil)
	amount := new(big.Rat).SetFrac(value, base)
	if asset.Symbol == "" {
		return amount.FloatString(4)
	}
	return fmt.Sprintf("%s %s", amount.FloatString(4), asset.Symbol)
}
