package tracing

import (
	"context"
	"fmt"
	"math/big"
	"sort"
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
	Labels                                []labels.LabelItem
}
type GraphEdge struct {
	ID, Source, Target, AmountBaseUnits, AmountFormatted, EventID, TransactionHash, TransferKind, SourceName string
	Asset                                                                                                    adapter.Asset
	TxCount                                                                                                  uint32
	BlockNumber                                                                                              uint64
	Timestamp, RetrievedAt                                                                                   int64
}
type GraphResult struct {
	Network                string
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
	chainAdapter  adapter.ChainAdapter
	database      *db.DB
	labelRegistry *labels.Service
}

func NewEngine(chainAdapter adapter.ChainAdapter, database *db.DB, labels *labels.Service) *Engine {
	return &Engine{chainAdapter: chainAdapter, database: database, labelRegistry: labels}
}

func (e *Engine) ResolveGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string) (*GraphResult, error) {
	if e.chainAdapter == nil {
		return nil, fmt.Errorf("trace data source is not configured")
	}
	if limit == 0 || limit > maxPageSize {
		limit = maxPageSize
	}
	page, err := e.chainAdapter.ListTransfers(ctx, address, limit, cursor)
	if err != nil {
		return nil, err
	}
	transfers := filterTransfers(page.Transfers, address, direction)
	result := e.graph(ctx, address, transfers, page)
	if e.database != nil {
		if err := e.database.SaveGraph(ctx, e.toAddresses(result.Nodes), e.toTransfers(transfers, page.SourceStatus)); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func (e *Engine) PendingGraph(address, warning string) *GraphResult {
	seed := strings.ToLower(address)
	return &GraphResult{
		Network:     e.Network(),
		SeedAddress: seed,
		Nodes:       []GraphNode{{ID: seed, Label: shortAddress(seed), EntityType: "EOA", IsSeed: true, TotalVolumeWei: "0"}},
		TotalNodes:  1,
		Pending:     true,
		SourceStatus: adapter.SourceStatus{
			Source:      e.SourceStatus(context.Background()).Source,
			RetrievedAt: time.Now().UTC(),
			Warning:     warning,
		},
	}
}

func (e *Engine) Network() string {
	if e.chainAdapter == nil {
		return ""
	}
	return e.chainAdapter.Network()
}

func (e *Engine) LookupTransaction(ctx context.Context, hash string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	if e.chainAdapter == nil {
		return nil, adapter.SourceStatus{}, fmt.Errorf("trace data source is not configured")
	}
	return e.chainAdapter.LookupTransaction(ctx, hash)
}

func (e *Engine) SourceStatus(ctx context.Context) adapter.SourceStatus {
	if e.chainAdapter == nil {
		return adapter.SourceStatus{Warning: "Trace data source is not configured."}
	}
	return e.chainAdapter.SourceStatus()
}

func (e *Engine) graph(ctx context.Context, seed string, transfers []adapter.TransferItem, page *adapter.TransferPage) *GraphResult {
	seed = strings.ToLower(seed)
	nodes := map[string]GraphNode{seed: e.node(ctx, seed, true)}
	inCounts, outCounts := map[string]uint32{}, map[string]uint32{}
	edges := make([]GraphEdge, 0, len(transfers))
	for _, transfer := range transfers {
		from, to := strings.ToLower(transfer.From), strings.ToLower(transfer.To)
		if _, ok := nodes[from]; !ok {
			nodes[from] = e.node(ctx, from, from == seed)
		}
		if _, ok := nodes[to]; !ok {
			nodes[to] = e.node(ctx, to, to == seed)
		}
		outCounts[from]++
		inCounts[to]++
		edges = append(edges, GraphEdge{ID: transferID(e.Network(), transfer.Hash, transfer.EventID), Source: from, Target: to, AmountBaseUnits: transfer.AmountBaseUnits, AmountFormatted: formatAmount(transfer.AmountBaseUnits, transfer.Asset), TxCount: 1, Asset: transfer.Asset, EventID: transfer.EventID, TransactionHash: transfer.Hash, TransferKind: transfer.TransferKind, SourceName: page.SourceStatus.Source, BlockNumber: uint64(transfer.BlockNumber), Timestamp: transfer.Timestamp.Unix(), RetrievedAt: page.SourceStatus.RetrievedAt.Unix()})
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
	return &GraphResult{Network: e.Network(), SeedAddress: seed, Nodes: graphNodes, Edges: edges, TotalNodes: uint32(len(graphNodes)), TotalEdges: uint32(len(edges)), NextCursor: page.NextCursor, HasMore: page.HasMore, SourceStatus: page.SourceStatus}
}

func (e *Engine) node(ctx context.Context, address string, seed bool) GraphNode {
	label, entityType := shortAddress(address), "EOA"
	var nodeLabels []labels.LabelItem
	if e.labelRegistry != nil {
		if items, err := e.labelRegistry.GetLabels(ctx, e.Network(), address); err == nil && len(items) > 0 {
			label = items[0].Label
			nodeLabels = items
			if strings.EqualFold(items[0].Category, "exchange") {
				entityType = "EXCHANGE"
			}
		}
	}
	if e.chainAdapter != nil && seed {
		if contract, err := e.chainAdapter.IsContract(ctx, address); err == nil && contract {
			entityType = "CONTRACT"
		}
	}
	return GraphNode{ID: address, Label: label, EntityType: entityType, IsSeed: seed, TotalVolumeWei: "0", Labels: nodeLabels}
}

func filterTransfers(transfers []adapter.TransferItem, address string, direction Direction) []adapter.TransferItem {
	address = strings.ToLower(address)
	result := make([]adapter.TransferItem, 0, len(transfers))
	for _, transfer := range transfers {
		from, to := strings.ToLower(transfer.From), strings.ToLower(transfer.To)
		if direction == DirectionInbound && to != address {
			continue
		}
		if direction == DirectionOutbound && from != address {
			continue
		}
		result = append(result, transfer)
	}
	return result
}

func (e *Engine) toTransfers(items []adapter.TransferItem, source adapter.SourceStatus) []db.Transfer {
	transfers := make([]db.Transfer, 0, len(items))
	for _, item := range items {
		transfers = append(transfers, db.Transfer{ID: transferID(e.Network(), item.Hash, item.EventID), Network: e.Network(), TransactionHash: item.Hash, EventID: item.EventID, TransferKind: item.TransferKind, FromAddress: strings.ToLower(item.From), ToAddress: strings.ToLower(item.To), Asset: item.Asset, AmountBaseUnits: item.AmountBaseUnits, BlockNumber: item.BlockNumber, BlockTimestamp: item.Timestamp, Source: source.Source, RetrievedAt: source.RetrievedAt})
	}
	return transfers
}

func (e *Engine) toAddresses(nodes []GraphNode) []db.Address {
	addresses := make([]db.Address, 0, len(nodes))
	for _, node := range nodes {
		addresses = append(addresses, db.Address{Network: e.Network(), Address: node.ID, Label: node.Label, EntityType: node.EntityType})
	}
	return addresses
}

func transferID(network, hash, eventID string) string {
	return network + ":" + strings.ToLower(hash) + ":" + eventID
}
func formatAmount(value string, asset adapter.Asset) string {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return value + " " + asset.Symbol
	}
	if asset.Kind == "NATIVE" {
		return adapter.FormatWeiToETH(amount)
	}
	return formatTokenAmount(amount, asset.Decimals, asset.Symbol)
}

func formatTokenAmount(amount *big.Int, decimals uint32, symbol string) string {
	base := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	value := new(big.Rat).SetFrac(amount, base)
	return value.FloatString(4) + " " + symbol
}
func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
