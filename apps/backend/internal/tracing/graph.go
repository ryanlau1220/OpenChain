package tracing

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
)

const maxPageSize = 50

type Direction string

const (
	DirectionBoth     Direction = "both"
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

type GraphNode struct {
	ID, Label, EntityType, TotalVolumeWei string
	IsSeed                                bool
	InTxCount, OutTxCount                 uint32
}
type GraphEdge struct {
	ID, Source, Target, ValueWei, ValueFormatted, AssetSymbol, TransactionHash string
	TxCount, EventIndex                                                        uint32
	BlockNumber                                                                uint64
	Timestamp                                                                  int64
}
type GraphResult struct {
	SeedAddress            string
	Nodes                  []GraphNode
	Edges                  []GraphEdge
	TotalNodes, TotalEdges uint32
	NextCursor             string
	HasMore                bool
	Pending                bool
	SourceStatus           adapter.SourceStatus
}

type Engine struct {
	evmClient     *adapter.EVMClient
	trueBlocks    *adapter.TrueBlocksClient
	database      *db.DB
	labelRegistry *labels.Registry
}

func NewEngine(evm *adapter.EVMClient, trueBlocks *adapter.TrueBlocksClient, database *db.DB, labels *labels.Registry) *Engine {
	return &Engine{evmClient: evm, trueBlocks: trueBlocks, database: database, labelRegistry: labels}
}

func (e *Engine) ResolveGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string) (*GraphResult, error) {
	if e.trueBlocks == nil {
		return nil, fmt.Errorf("TrueBlocks is not configured")
	}
	if limit == 0 || limit > maxPageSize {
		limit = maxPageSize
	}
	offset, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}
	var latestBlock uint64
	if e.evmClient != nil {
		latestBlock, _ = e.evmClient.GetLatestBlockNumber(ctx)
	}
	page, err := e.trueBlocks.ListNativeTransfers(ctx, address, limit, offset, latestBlock)
	if err != nil {
		return nil, err
	}
	transactions := filterTransactions(page.Transactions, address, direction)
	result := e.graph(ctx, address, transactions, page)
	if e.database != nil {
		if err := e.database.SaveGraph(ctx, toAddresses(result.Nodes), toTransfers(transactions, page.SourceStatus)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (e *Engine) PendingGraph(address, warning string) *GraphResult {
	seed := strings.ToLower(address)
	return &GraphResult{
		SeedAddress: seed,
		Nodes:       []GraphNode{{ID: seed, Label: shortAddress(seed), EntityType: "EOA", IsSeed: true, TotalVolumeWei: "0"}},
		TotalNodes:  1,
		Pending:     true,
		SourceStatus: adapter.SourceStatus{
			Source:      "trueblocks-6.5.0",
			RetrievedAt: time.Now().UTC(),
			Warning:     warning,
		},
	}
}

func (e *Engine) LookupTransaction(ctx context.Context, hash string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	if e.trueBlocks == nil {
		return nil, adapter.SourceStatus{}, fmt.Errorf("TrueBlocks is not configured")
	}
	var latestBlock uint64
	if e.evmClient != nil {
		latestBlock, _ = e.evmClient.GetLatestBlockNumber(ctx)
	}
	return e.trueBlocks.LookupTransaction(ctx, hash, latestBlock)
}

func (e *Engine) SourceStatus(ctx context.Context) adapter.SourceStatus {
	if e.trueBlocks == nil {
		return adapter.SourceStatus{Source: "trueblocks-6.5.0", Warning: "TrueBlocks is not configured."}
	}
	var latestBlock uint64
	if e.evmClient != nil {
		latestBlock, _ = e.evmClient.GetLatestBlockNumber(ctx)
	}
	return e.trueBlocks.Status(ctx, latestBlock)
}

func (e *Engine) graph(ctx context.Context, seed string, transactions []adapter.TransactionItem, page *adapter.TransferPage) *GraphResult {
	seed = strings.ToLower(seed)
	nodes := map[string]GraphNode{seed: e.node(ctx, seed, true)}
	inCounts, outCounts := map[string]uint32{}, map[string]uint32{}
	edges := make([]GraphEdge, 0, len(transactions))
	for _, transaction := range transactions {
		from, to := strings.ToLower(transaction.From), strings.ToLower(transaction.To)
		if _, ok := nodes[from]; !ok {
			nodes[from] = e.node(ctx, from, from == seed)
		}
		if _, ok := nodes[to]; !ok {
			nodes[to] = e.node(ctx, to, to == seed)
		}
		outCounts[from]++
		inCounts[to]++
		edges = append(edges, GraphEdge{ID: transferID(transaction.Hash), Source: from, Target: to, ValueWei: transaction.ValueWei, ValueFormatted: formatValue(transaction.ValueWei), TxCount: 1, AssetSymbol: "ETH", TransactionHash: transaction.Hash, BlockNumber: uint64(transaction.BlockNumber), Timestamp: transaction.Timestamp.Unix()})
	}
	graphNodes := make([]GraphNode, 0, len(nodes))
	for id, node := range nodes {
		node.InTxCount, node.OutTxCount = inCounts[id], outCounts[id]
		graphNodes = append(graphNodes, node)
	}
	sort.Slice(graphNodes, func(i, j int) bool {
		if graphNodes[i].IsSeed != graphNodes[j].IsSeed {
			return graphNodes[i].IsSeed
		}
		return graphNodes[i].ID < graphNodes[j].ID
	})
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })
	return &GraphResult{SeedAddress: seed, Nodes: graphNodes, Edges: edges, TotalNodes: uint32(len(graphNodes)), TotalEdges: uint32(len(edges)), NextCursor: page.NextCursor, HasMore: page.HasMore, SourceStatus: page.SourceStatus}
}

func (e *Engine) node(ctx context.Context, address string, seed bool) GraphNode {
	label, entityType := shortAddress(address), "EOA"
	if e.labelRegistry != nil {
		if labels := e.labelRegistry.GetLabels(ctx, address); len(labels) > 0 {
			label = labels[0].Label
			if strings.EqualFold(labels[0].Category, "exchange") {
				entityType = "EXCHANGE"
			}
		}
	}
	if e.evmClient != nil {
		if contract, err := e.evmClient.IsContract(ctx, address); err == nil && contract {
			entityType = "CONTRACT"
		}
	}
	return GraphNode{ID: address, Label: label, EntityType: entityType, IsSeed: seed, TotalVolumeWei: "0"}
}

func filterTransactions(transactions []adapter.TransactionItem, address string, direction Direction) []adapter.TransactionItem {
	address = strings.ToLower(address)
	result := make([]adapter.TransactionItem, 0, len(transactions))
	for _, transaction := range transactions {
		from, to := strings.ToLower(transaction.From), strings.ToLower(transaction.To)
		if direction == DirectionInbound && to != address {
			continue
		}
		if direction == DirectionOutbound && from != address {
			continue
		}
		result = append(result, transaction)
	}
	return result
}

func toTransfers(transactions []adapter.TransactionItem, source adapter.SourceStatus) []db.Transfer {
	transfers := make([]db.Transfer, 0, len(transactions))
	for _, transaction := range transactions {
		transfers = append(transfers, db.Transfer{ID: transferID(transaction.Hash), Network: "ethereum-mainnet", TransactionHash: transaction.Hash, FromAddress: strings.ToLower(transaction.From), ToAddress: strings.ToLower(transaction.To), AssetSymbol: "ETH", AmountBaseUnits: transaction.ValueWei, BlockNumber: transaction.BlockNumber, BlockTimestamp: transaction.Timestamp, Source: source.Source, RetrievedAt: source.RetrievedAt})
	}
	return transfers
}

func toAddresses(nodes []GraphNode) []db.Address {
	addresses := make([]db.Address, 0, len(nodes))
	for _, node := range nodes {
		addresses = append(addresses, db.Address{Address: node.ID, Label: node.Label, EntityType: node.EntityType})
	}
	return addresses
}

func parseCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	value, err := strconv.ParseUint(cursor, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid cursor")
	}
	return value, nil
}
func transferID(hash string) string { return "ethereum-mainnet:" + strings.ToLower(hash) + ":0" }
func formatValue(value string) string {
	wei, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return value + " Wei"
	}
	return adapter.FormatWeiToETH(wei)
}
func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
