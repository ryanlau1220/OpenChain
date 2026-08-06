package tracing

import (
	"context"
	"fmt"
	"math/big"
	"strings"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
)

type GraphNode struct {
	ID             string  `json:"id"`
	Label          string  `json:"label"`
	EntityType     string  `json:"entity_type"`
	RiskScore      float64 `json:"risk_score"`
	Category       string  `json:"category"`
	IsSeed         bool    `json:"is_seed"`
	TotalVolumeWei string  `json:"total_volume_wei"`
}

type GraphEdge struct {
	ID                string `json:"id"`
	Source            string `json:"source"`
	Target            string `json:"target"`
	ValueWei          string `json:"value_wei"`
	ValueFormatted    string `json:"value_formatted"`
	TxCount           uint32 `json:"tx_count"`
	AssetSymbol       string `json:"asset_symbol"`
	FirstTxTimestamp  int64  `json:"first_tx_timestamp"`
	LastTxTimestamp   int64  `json:"last_tx_timestamp"`
}

type GraphResult struct {
	SeedAddress string      `json:"seed_address"`
	Nodes       []GraphNode `json:"nodes"`
	Edges       []GraphEdge `json:"edges"`
	TotalNodes  uint32      `json:"total_nodes"`
	TotalEdges  uint32      `json:"total_edges"`
}

type Engine struct {
	evmClient     *adapter.EVMClient
	labelRegistry *labels.Registry
	riskEvaluator *risk.Evaluator
}

func NewEngine(evm *adapter.EVMClient, lr *labels.Registry, re *risk.Evaluator) *Engine {
	return &Engine{
		evmClient:     evm,
		labelRegistry: lr,
		riskEvaluator: re,
	}
}

func (e *Engine) TraceMultiHopGraph(ctx context.Context, seedAddress string, network string, maxHops uint32, direction string) (*GraphResult, error) {
	seedAddrClean := strings.ToLower(seedAddress)
	nodeMap := make(map[string]GraphNode)
	edgeMap := make(map[string]GraphEdge)

	// Fetch real details for seed address
	isContract, _ := e.evmClient.IsContract(ctx, seedAddrClean)
	txCount, _ := e.evmClient.GetTxCount(ctx, seedAddrClean)
	bal, _ := e.evmClient.GetBalance(ctx, seedAddrClean)
	riskEval := e.riskEvaluator.EvaluateAddress(ctx, seedAddrClean, network, isContract, txCount)

	entityType := "EOA"
	if isContract {
		entityType = "CONTRACT"
	}

	seedLabel := "Seed Address"
	labelsList := e.labelRegistry.GetLabels(ctx, seedAddrClean)
	if len(labelsList) > 0 {
		seedLabel = labelsList[0].Label
	}

	nodeMap[seedAddrClean] = GraphNode{
		ID:             seedAddrClean,
		Label:          seedLabel,
		EntityType:     entityType,
		RiskScore:      riskEval.TotalScore,
		Category:       riskEval.RiskLevel,
		IsSeed:         true,
		TotalVolumeWei: bal.String(),
	}

	// Fetch logs / transfers via EVM RPC
	logs, _ := e.evmClient.GetERC20Transfers(ctx, seedAddrClean, "0x0")

	for idx, l := range logs {
		if len(l.Topics) < 3 {
			continue
		}

		from := "0x" + strings.TrimPrefix(l.Topics[1], "0x000000000000000000000000")
		to := "0x" + strings.TrimPrefix(l.Topics[2], "0x000000000000000000000000")

		from = strings.ToLower(from)
		to = strings.ToLower(to)

		// Create nodes for counter-parties
		for _, addr := range []string{from, to} {
			if _, exists := nodeMap[addr]; !exists {
				nodeMap[addr] = GraphNode{
					ID:             addr,
					Label:          shortAddress(addr),
					EntityType:     "EOA",
					RiskScore:      0.0,
					Category:       "LOW",
					IsSeed:         false,
					TotalVolumeWei: "0",
				}
			}
		}

		// Create or aggregate edge
		edgeID := fmt.Sprintf("%s-%s-%d", from, to, idx)
		rawVal := new(big.Int)
		rawVal.SetString(strings.TrimPrefix(l.Data, "0x"), 16)

		edgeMap[edgeID] = GraphEdge{
			ID:             edgeID,
			Source:         from,
			Target:         to,
			ValueWei:       rawVal.String(),
			ValueFormatted: fmt.Sprintf("%s Token", rawVal.String()),
			TxCount:        1,
			AssetSymbol:    "ERC20",
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}

	edges := make([]GraphEdge, 0, len(edgeMap))
	for _, eg := range edgeMap {
		edges = append(edges, eg)
	}

	return &GraphResult{
		SeedAddress: seedAddrClean,
		Nodes:       nodes,
		Edges:       edges,
		TotalNodes:  uint32(len(nodes)),
		TotalEdges:  uint32(len(edges)),
	}, nil
}

func shortAddress(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
