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
	"github.com/openchain/openchain/apps/backend/internal/rules"
)

const (
	maxPageSize              = 50
	DefaultCounterpartyLimit = 10
	MaxCounterpartyLimit     = 25
)

type Direction string

type Ranking string

const (
	DirectionBoth         Direction = "both"
	DirectionInbound      Direction = "inbound"
	DirectionOutbound     Direction = "outbound"
	RankingMostRecent     Ranking   = "most_recent"
	RankingTotalRawAmount Ranking   = "total_raw_amount"
	RankingMostActive     Ranking   = "most_active"
	RankingKnownEntity    Ranking   = "known_entity"
)

type GraphNode struct {
	ID, Label, EntityType, TotalVolumeBaseUnits string
	IsSeed                                      bool
	InTxCount, OutTxCount                       uint32
	Labels                                      []labels.LabelItem
}
type GraphEdge struct {
	ID, Source, Target, AmountBaseUnits, AmountFormatted, EventID, TransactionHash, TransferKind, SourceName, BlockHash string
	Asset                                                                                                               adapter.Asset
	TxCount                                                                                                             uint32
	BlockNumber                                                                                                         uint64
	Timestamp, RetrievedAt                                                                                              int64
	Provisional                                                                                                         bool
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
	Leads                  []rules.Lead
}

type Engine struct {
	chainAdapter     adapter.ChainAdapter
	database         *db.DB
	labelRegistry    *labels.Service
	bridgeCorrelator *BridgeCorrelator
}

func (e *Engine) SetBridgeCorrelator(correlator *BridgeCorrelator) { e.bridgeCorrelator = correlator }

func NewEngine(chainAdapter adapter.ChainAdapter, database *db.DB, labels *labels.Service) *Engine {
	return &Engine{chainAdapter: chainAdapter, database: database, labelRegistry: labels}
}

func (e *Engine) ResolveGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string, maxCounterparties uint32, ranking Ranking) (*GraphResult, error) {
	if e.chainAdapter == nil {
		return nil, fmt.Errorf("trace data source is not configured")
	}
	var err error
	address, err = e.chainAdapter.NormalizeAddress(address)
	if err != nil {
		return nil, err
	}
	if limit == 0 || limit > maxPageSize {
		limit = maxPageSize
	}
	maxCounterparties, ranking = graphControls(maxCounterparties, ranking)
	acquisitionContext, recorder := adapter.WithAcquisitionRecorder(ctx)
	page, err := e.chainAdapter.ListTransfers(acquisitionContext, address, limit, cursor)
	if err != nil {
		if e.database != nil {
			if persistErr := e.database.SaveAcquisitions(ctx, e.Network(), recorder.Items()); persistErr != nil {
				return nil, persistErr
			}
		}
		return nil, err
	}
	filtered := filterTransfers(page.Transfers, address, direction)
	transfers := selectCounterpartyTransfersWithKnown(filtered, address, maxCounterparties, ranking, e.knownCounterparties(ctx, filtered, address, ranking))
	result := e.graph(ctx, address, transfers, page)
	persistedTransfers := e.toTransfers(transfers, page.SourceStatus)
	completedAt := time.Now().UTC()
	leads, runs := rules.Evaluate(e.Network(), persistedTransfers, completedAt)
	result.Leads = leads
	if e.database != nil {
		if err := e.database.SaveEvidenceGraph(ctx, e.toAddresses(result.Nodes), persistedTransfers, recorder.Items()); err != nil {
			return nil, err
		}
		if err := e.database.SaveRuleRuns(ctx, runs); err != nil {
			return nil, err
		}
		if e.bridgeCorrelator != nil {
			bridgeEvidence := e.bridgeCorrelator.Correlate(ctx, e.Network(), persistedTransfers)
			candidates := make([]rules.BridgeCandidate, 0, len(bridgeEvidence))
			for _, evidence := range bridgeEvidence {
				if err := e.database.SaveEvidenceGraph(ctx, evidence.Addresses, evidence.Transfers, evidence.Acquisitions); err != nil {
					return nil, err
				}
				candidates = append(candidates, evidence.Candidate)
			}
			bridgeLeads, bridgeRuns := rules.EvaluateBridge(e.Network(), candidates, completedAt)
			if err := e.database.SaveRuleRuns(ctx, bridgeRuns); err != nil {
				return nil, err
			}
			result.Leads = append(result.Leads, bridgeLeads...)
		}
	}
	return result, nil
}

func graphControls(maxCounterparties uint32, ranking Ranking) (uint32, Ranking) {
	if maxCounterparties == 0 {
		maxCounterparties = DefaultCounterpartyLimit
	}
	if maxCounterparties > MaxCounterpartyLimit {
		maxCounterparties = MaxCounterpartyLimit
	}
	switch ranking {
	case RankingTotalRawAmount, RankingMostActive, RankingMostRecent, RankingKnownEntity:
	default:
		ranking = RankingMostRecent
	}
	return maxCounterparties, ranking
}

type counterpartyStats struct {
	address string
	count   int
	latest  time.Time
	largest *big.Rat
	totals  map[string]*big.Rat
	known   bool
}

func selectCounterpartyTransfers(transfers []adapter.TransferItem, seed string, maxCounterparties uint32, ranking Ranking) []adapter.TransferItem {
	return selectCounterpartyTransfersWithKnown(transfers, seed, maxCounterparties, ranking, nil)
}

func selectCounterpartyTransfersWithKnown(transfers []adapter.TransferItem, seed string, maxCounterparties uint32, ranking Ranking, known map[string]bool) []adapter.TransferItem {
	maxCounterparties, ranking = graphControls(maxCounterparties, ranking)
	stats := make(map[string]*counterpartyStats)
	for _, transfer := range transfers {
		counterparty := transfer.From
		if strings.EqualFold(counterparty, seed) {
			counterparty = transfer.To
		}
		if counterparty == "" || strings.EqualFold(counterparty, seed) {
			continue
		}
		item := stats[counterparty]
		if item == nil {
			item = &counterpartyStats{address: counterparty, largest: big.NewRat(0, 1), totals: make(map[string]*big.Rat), known: known[counterparty]}
			stats[counterparty] = item
		}
		item.count++
		if transfer.Timestamp.After(item.latest) {
			item.latest = transfer.Timestamp
		}
		amount, ok := new(big.Int).SetString(transfer.AmountBaseUnits, 10)
		if ok {
			scaled := new(big.Rat).SetFrac(amount, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(transfer.Asset.Decimals)), nil))
			assetKey := transfer.Asset.Kind + ":" + transfer.Asset.ContractAddress + ":" + transfer.Asset.Symbol
			total := item.totals[assetKey]
			if total == nil {
				total = big.NewRat(0, 1)
				item.totals[assetKey] = total
			}
			total.Add(total, scaled)
			if total.Cmp(item.largest) > 0 {
				item.largest = new(big.Rat).Set(total)
			}
		}
	}
	items := make([]*counterpartyStats, 0, len(stats))
	for _, item := range stats {
		items = append(items, item)
	}
	sort.Slice(items, func(left, right int) bool {
		first, second := items[left], items[right]
		switch ranking {
		case RankingTotalRawAmount:
			if compared := first.largest.Cmp(second.largest); compared != 0 {
				return compared > 0
			}
		case RankingMostActive:
			if first.count != second.count {
				return first.count > second.count
			}
		case RankingKnownEntity:
			if first.known != second.known {
				return first.known
			}
		}
		if !first.latest.Equal(second.latest) {
			return first.latest.After(second.latest)
		}
		return first.address < second.address
	})
	selected := make(map[string]struct{}, min(int(maxCounterparties), len(items)))
	for _, item := range items[:min(int(maxCounterparties), len(items))] {
		selected[item.address] = struct{}{}
	}
	result := make([]adapter.TransferItem, 0, len(transfers))
	for _, transfer := range transfers {
		counterparty := transfer.From
		if strings.EqualFold(counterparty, seed) {
			counterparty = transfer.To
		}
		if _, ok := selected[counterparty]; ok {
			result = append(result, transfer)
		}
	}
	return result
}

func (e *Engine) knownCounterparties(ctx context.Context, transfers []adapter.TransferItem, seed string, ranking Ranking) map[string]bool {
	if ranking != RankingKnownEntity || e.labelRegistry == nil {
		return nil
	}
	known := make(map[string]bool)
	for _, transfer := range transfers {
		counterparty := transfer.From
		if strings.EqualFold(counterparty, seed) {
			counterparty = transfer.To
		}
		if counterparty == "" || strings.EqualFold(counterparty, seed) {
			continue
		}
		if _, checked := known[counterparty]; checked {
			continue
		}
		items, err := e.labelRegistry.GetLabels(ctx, e.Network(), counterparty)
		known[counterparty] = err == nil && len(items) > 0
	}
	return known
}

func (e *Engine) PendingGraph(address, warning string) *GraphResult {
	seed := e.normalizeAddress(address)
	return &GraphResult{
		Network:     e.Network(),
		SeedAddress: seed,
		Nodes:       []GraphNode{{ID: seed, Label: shortAddress(seed), EntityType: "EOA", IsSeed: true, TotalVolumeBaseUnits: "0"}},
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
	normalized, err := e.chainAdapter.NormalizeTransactionHash(hash)
	if err != nil {
		return nil, adapter.SourceStatus{}, err
	}
	return e.chainAdapter.LookupTransaction(ctx, normalized)
}

func (e *Engine) SourceStatus(ctx context.Context) adapter.SourceStatus {
	if e.chainAdapter == nil {
		return adapter.SourceStatus{Warning: "Trace data source is not configured."}
	}
	return e.chainAdapter.SourceStatus()
}

func (e *Engine) ExportEvidence(ctx context.Context, transferIDs []string) (*db.EvidenceExport, error) {
	if e.database == nil {
		return nil, fmt.Errorf("evidence storage is not configured")
	}
	return e.database.ExportEvidence(ctx, e.Network(), transferIDs)
}

func (e *Engine) graph(ctx context.Context, seed string, transfers []adapter.TransferItem, page *adapter.TransferPage) *GraphResult {
	seed = e.normalizeAddress(seed)
	nodes := map[string]GraphNode{seed: e.node(ctx, seed, true)}
	inCounts, outCounts := map[string]uint32{}, map[string]uint32{}
	edges := make([]GraphEdge, 0, len(transfers))
	for _, transfer := range transfers {
		from, to := e.normalizeAddress(transfer.From), e.normalizeAddress(transfer.To)
		if _, ok := nodes[from]; !ok {
			nodes[from] = e.node(ctx, from, from == seed)
		}
		if _, ok := nodes[to]; !ok {
			nodes[to] = e.node(ctx, to, to == seed)
		}
		outCounts[from]++
		inCounts[to]++
		edges = append(edges, GraphEdge{ID: transferID(e.Network(), transfer.Hash, transfer.EventID), Source: from, Target: to, AmountBaseUnits: transfer.AmountBaseUnits, AmountFormatted: formatAmount(transfer.AmountBaseUnits, transfer.Asset), TxCount: 1, Asset: transfer.Asset, EventID: transfer.EventID, TransactionHash: transfer.Hash, TransferKind: transfer.TransferKind, SourceName: page.SourceStatus.Source, BlockNumber: uint64(transfer.BlockNumber), BlockHash: transfer.BlockHash, Timestamp: transfer.Timestamp.Unix(), RetrievedAt: page.SourceStatus.RetrievedAt.Unix(), Provisional: isProvisional(e.Network(), transfer.Timestamp, page.SourceStatus.RetrievedAt)})
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
	return GraphNode{ID: address, Label: label, EntityType: entityType, IsSeed: seed, TotalVolumeBaseUnits: "0", Labels: nodeLabels}
}

func filterTransfers(transfers []adapter.TransferItem, address string, direction Direction) []adapter.TransferItem {
	result := make([]adapter.TransferItem, 0, len(transfers))
	for _, transfer := range transfers {
		from, to := transfer.From, transfer.To
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
		transfers = append(transfers, db.Transfer{ID: transferID(e.Network(), item.Hash, item.EventID), Network: e.Network(), TransactionHash: item.Hash, EventID: item.EventID, TransferKind: item.TransferKind, FromAddress: e.normalizeAddress(item.From), ToAddress: e.normalizeAddress(item.To), Asset: item.Asset, AmountBaseUnits: item.AmountBaseUnits, BlockNumber: item.BlockNumber, BlockHash: item.BlockHash, BlockTimestamp: item.Timestamp, Provisional: isProvisional(e.Network(), item.Timestamp, source.RetrievedAt), Source: source.Source, RetrievedAt: source.RetrievedAt})
	}
	return transfers
}

func isProvisional(network string, blockTimestamp, retrievedAt time.Time) bool {
	if blockTimestamp.IsZero() {
		return true
	}
	window := 15 * time.Minute
	if network == "solana-mainnet" || network == "tron-mainnet" {
		window = time.Minute
	}
	return retrievedAt.Before(blockTimestamp.Add(window))
}

func (e *Engine) toAddresses(nodes []GraphNode) []db.Address {
	addresses := make([]db.Address, 0, len(nodes))
	for _, node := range nodes {
		addresses = append(addresses, db.Address{Network: e.Network(), Address: node.ID, Label: node.Label, EntityType: node.EntityType})
	}
	return addresses
}

func transferID(network, hash, eventID string) string {
	return network + ":" + hash + ":" + eventID
}
func formatAmount(value string, asset adapter.Asset) string {
	amount, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return value + " " + asset.Symbol
	}
	return adapter.FormatAmount(amount, asset)
}

func (e *Engine) normalizeAddress(value string) string {
	if e.chainAdapter == nil {
		return value
	}
	if normalized, err := e.chainAdapter.NormalizeAddress(value); err == nil {
		return normalized
	}
	return value
}
func shortAddress(address string) string {
	if len(address) <= 10 {
		return address
	}
	return address[:6] + "..." + address[len(address)-4:]
}
