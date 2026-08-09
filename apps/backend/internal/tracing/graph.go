package tracing

import (
	"context"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
)

const transactionLimit = 15

type GraphNode struct {
	ID             string
	Label          string
	EntityType     string
	IsSeed         bool
	TotalVolumeWei string
	InTxCount      uint32
	OutTxCount     uint32
}

type GraphEdge struct {
	ID             string
	Source         string
	Target         string
	ValueWei       string
	ValueFormatted string
	TxCount        uint32
	AssetSymbol    string
}

type GraphResult struct {
	SeedAddress string
	Nodes       []GraphNode
	Edges       []GraphEdge
	TotalNodes  uint32
	TotalEdges  uint32
}

// Engine builds a one-hop graph from the configured EVM data source. It does not
// persist user-supplied graph data or invent data when the upstream source fails.
type Engine struct {
	evmClient     *adapter.EVMClient
	chainAdapter  adapter.ChainAdapter
	labelRegistry *labels.Registry
}

func NewEngine(evm *adapter.EVMClient, lr *labels.Registry) *Engine {
	var chain adapter.ChainAdapter
	if evm != nil {
		chain = adapter.NewEVMChainAdapter("ETHEREUM_SEPOLIA", "", "", evm)
	}
	return &Engine{evmClient: evm, chainAdapter: chain, labelRegistry: lr}
}

func (e *Engine) ChainAdapter() adapter.ChainAdapter { return e.chainAdapter }

func (e *Engine) TraceGraph(ctx context.Context, address string) (*GraphResult, error) {
	seed := strings.ToLower(strings.TrimSpace(address))
	nodeMap := map[string]GraphNode{}
	inCounts := map[string]uint32{}
	outCounts := map[string]uint32{}

	seedNode := e.node(ctx, seed, true)
	if e.evmClient != nil {
		if balance, err := e.evmClient.GetBalance(ctx, seed); err == nil {
			seedNode.TotalVolumeWei = balance.String()
		}
	}
	nodeMap[seed] = seedNode

	edges := make([]GraphEdge, 0)
	if e.chainAdapter != nil {
		txs, err := e.chainAdapter.GetAccountTransactions(ctx, seed, transactionLimit)
		if err != nil {
			return nil, fmt.Errorf("get account transactions: %w", err)
		}
		for _, tx := range txs {
			from, to := strings.ToLower(tx.From), strings.ToLower(tx.To)
			if from == "" || to == "" {
				continue
			}
			if _, ok := nodeMap[from]; !ok {
				nodeMap[from] = e.node(ctx, from, from == seed)
			}
			if _, ok := nodeMap[to]; !ok {
				nodeMap[to] = e.node(ctx, to, to == seed)
			}
			outCounts[from]++
			inCounts[to]++
			edges = append(edges, GraphEdge{
				ID: tx.Hash, Source: from, Target: to, ValueWei: tx.ValueWei,
				ValueFormatted: formatValue(tx.ValueWei, tx.AssetSymbol), TxCount: 1, AssetSymbol: tx.AssetSymbol,
			})
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for id, node := range nodeMap {
		node.InTxCount, node.OutTxCount = inCounts[id], outCounts[id]
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsSeed != nodes[j].IsSeed {
			return nodes[i].IsSeed
		}
		return nodes[i].ID < nodes[j].ID
	})
	sort.Slice(edges, func(i, j int) bool { return edges[i].ID < edges[j].ID })

	return &GraphResult{SeedAddress: seed, Nodes: nodes, Edges: edges, TotalNodes: uint32(len(nodes)), TotalEdges: uint32(len(edges))}, nil
}

func (e *Engine) node(ctx context.Context, address string, isSeed bool) GraphNode {
	label, entityType := shortAddress(address), "EOA"
	if e.labelRegistry != nil {
		if items := e.labelRegistry.GetLabels(ctx, address); len(items) > 0 {
			label = items[0].Label
			if strings.EqualFold(items[0].Category, "exchange") {
				entityType = "EXCHANGE"
			}
		}
	}
	if e.evmClient != nil {
		if isContract, err := e.evmClient.IsContract(ctx, address); err == nil && isContract {
			entityType = "CONTRACT"
		}
	}
	return GraphNode{ID: address, Label: label, EntityType: entityType, IsSeed: isSeed, TotalVolumeWei: "0"}
}

func formatValue(value, symbol string) string {
	if value == "" {
		return "0"
	}
	if symbol == "ETH" {
		wei, ok := new(big.Int).SetString(value, 10)
		if ok {
			return adapter.FormatWeiToETH(wei)
		}
	}
	if symbol == "" {
		return value
	}
	return value + " " + symbol
}

func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
