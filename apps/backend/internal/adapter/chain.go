package adapter

import (
	"context"
	"math/big"
	"time"
)

// TransactionItem represents a normalized blockchain transaction/transfer across any network
type TransactionItem struct {
	Hash             string    `json:"hash"`
	From             string    `json:"from"`
	To               string    `json:"to"`
	ValueWei         string    `json:"value_wei"`
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
	Transactions []TransactionItem
	NextCursor   string
	HasMore      bool
	SourceStatus SourceStatus
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
	GetBalance(ctx context.Context, address string) (*big.Int, error)
	GetTxCount(ctx context.Context, address string) (uint64, error)
	IsContract(ctx context.Context, address string) (bool, error)
	ListNativeTransfers(ctx context.Context, address string, limit uint32, cursor uint64) (*TransferPage, error)
	LookupTransaction(ctx context.Context, hash string) (*TransactionItem, SourceStatus, error)
	GetContractMetadata(ctx context.Context, address string) (*ContractMetadata, error)
}
