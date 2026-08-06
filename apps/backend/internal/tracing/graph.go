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
	EntityType     string  `json:"entity_type"` // EOA, CONTRACT, EXCHANGE, MIXER, SCAMMER
	RiskScore      float64 `json:"risk_score"`
	Category       string  `json:"category"`
	IsSeed         bool    `json:"is_seed"`
	TotalVolumeWei string  `json:"total_volume_wei"`
	InTxCount      uint32  `json:"in_tx_count"`
	OutTxCount     uint32  `json:"out_tx_count"`
}

type GraphEdge struct {
	ID               string `json:"id"`
	Source           string `json:"source"`
	Target           string `json:"target"`
	ValueWei         string `json:"value_wei"`
	ValueFormatted   string `json:"value_formatted"`
	TxCount          uint32 `json:"tx_count"`
	AssetSymbol      string `json:"asset_symbol"`
	FirstTxTimestamp int64  `json:"first_tx_timestamp"`
	LastTxTimestamp  int64  `json:"last_tx_timestamp"`
}

type GraphResult struct {
	SeedAddresses []string    `json:"seed_addresses"`
	SeedAddress   string      `json:"seed_address,omitempty"`
	Nodes         []GraphNode `json:"nodes"`
	Edges         []GraphEdge `json:"edges"`
	TotalNodes    uint32      `json:"total_nodes"`
	TotalEdges    uint32      `json:"total_edges"`
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
	return e.TraceMultiAddressGraph(ctx, []string{seedAddress}, network, maxHops, direction, nil)
}

func (e *Engine) TraceMultiAddressGraph(ctx context.Context, seedAddresses []string, network string, maxHops uint32, direction string, tokens []string) (*GraphResult, error) {
	nodeMap := make(map[string]GraphNode)
	edgeMap := make(map[string]GraphEdge)
	inCountMap := make(map[string]uint32)
	outCountMap := make(map[string]uint32)

	var cleanedSeeds []string
	for _, addr := range seedAddresses {
		clean := strings.ToLower(strings.TrimSpace(addr))
		if clean == "" {
			continue
		}
		cleanedSeeds = append(cleanedSeeds, clean)

		isContract, _ := e.evmClient.IsContract(ctx, clean)
		txCount, _ := e.evmClient.GetTxCount(ctx, clean)
		bal, _ := e.evmClient.GetBalance(ctx, clean)
		riskEval := e.riskEvaluator.EvaluateAddress(ctx, clean, network, isContract, txCount)

		entityType := "EOA"
		if isContract {
			entityType = "CONTRACT"
		}

		seedLabel := "Target Wallet"
		labelsList := e.labelRegistry.GetLabels(ctx, clean)
		if len(labelsList) > 0 {
			seedLabel = labelsList[0].Label
			if strings.Contains(strings.ToLower(seedLabel), "binance") {
				entityType = "EXCHANGE"
			} else if strings.Contains(strings.ToLower(seedLabel), "scam") {
				entityType = "SCAMMER"
			}
		}

		nodeMap[clean] = GraphNode{
			ID:             clean,
			Label:          seedLabel,
			EntityType:     entityType,
			RiskScore:      riskEval.TotalScore,
			Category:       riskEval.RiskLevel,
			IsSeed:         true,
			TotalVolumeWei: bal.String(),
		}

		// Fetch logs / transfers via EVM RPC
		logs, _ := e.evmClient.GetERC20Transfers(ctx, clean, "0x0")

		for idx, l := range logs {
			if len(l.Topics) < 3 {
				continue
			}

			from := strings.ToLower("0x" + strings.TrimPrefix(l.Topics[1], "0x000000000000000000000000"))
			to := strings.ToLower("0x" + strings.TrimPrefix(l.Topics[2], "0x000000000000000000000000"))

			// Direction filter
			if direction == "INFLOW" && to != clean {
				continue
			}
			if direction == "OUTFLOW" && from != clean {
				continue
			}

			// Update transaction direction metrics
			outCountMap[from]++
			inCountMap[to]++

			// Create nodes for counter-parties
			for _, addr := range []string{from, to} {
				if _, exists := nodeMap[addr]; !exists {
					nodeLabel := shortAddress(addr)
					nodeEntity := "EOA"

					regLabels := e.labelRegistry.GetLabels(ctx, addr)
					if len(regLabels) > 0 {
						nodeLabel = regLabels[0].Label
						if strings.Contains(strings.ToLower(nodeLabel), "binance") || strings.Contains(strings.ToLower(nodeLabel), "exchange") {
							nodeEntity = "EXCHANGE"
						} else if strings.Contains(strings.ToLower(nodeLabel), "scam") {
							nodeEntity = "SCAMMER"
						}
					}

					nodeMap[addr] = GraphNode{
						ID:             addr,
						Label:          nodeLabel,
						EntityType:     nodeEntity,
						RiskScore:      0.0,
						Category:       "LOW",
						IsSeed:         false,
						TotalVolumeWei: "0",
					}
				}
			}

			// Create edge
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
				AssetSymbol:    "USDT",
			}
		}
	}

	nodes := make([]GraphNode, 0, len(nodeMap))
	for id, n := range nodeMap {
		n.InTxCount = inCountMap[id]
		n.OutTxCount = outCountMap[id]
		nodes = append(nodes, n)
	}

	edges := make([]GraphEdge, 0, len(edgeMap))
	for _, eg := range edgeMap {
		edges = append(edges, eg)
	}

	seedFirst := ""
	if len(cleanedSeeds) > 0 {
		seedFirst = cleanedSeeds[0]
	}

	return &GraphResult{
		SeedAddresses: cleanedSeeds,
		SeedAddress:   seedFirst,
		Nodes:         nodes,
		Edges:         edges,
		TotalNodes:    uint32(len(nodes)),
		TotalEdges:    uint32(len(edges)),
	}, nil
}

func shortAddress(addr string) string {
	if len(addr) <= 10 {
		return addr
	}
	return addr[:6] + "..." + addr[len(addr)-4:]
}
