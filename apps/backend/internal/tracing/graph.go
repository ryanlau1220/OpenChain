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
	DefaultGraphDepth        = 1
	MaxGraphDepth            = 3
	maxGraphTraversalNodes   = 100
	maxGraphTraversalEdges   = 250
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
	ID, Label, EntityType string
	IsSeed                bool
	InTxCount, OutTxCount uint32
	TotalVolumeByAsset    []AssetVolume
	Labels                []labels.LabelItem
}

type AssetVolume struct {
	Asset           adapter.Asset
	AmountBaseUnits string
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
	Coverage               TraceCoverage
	Leads                  []rules.Lead
}

type TraceCoverage struct {
	RequestedPageSize, ObservedTransferCount, GraphTransferCount, ConfirmationBackedTransferCount, ProvisionalTransferCount uint32
	Cursor                                                                                                                  string
	HasMore, ProviderComplete                                                                                               bool
	Limitation                                                                                                              string
}

const retrievedScopeLimitation = "This graph and its findings are limited to the retrieved provider page after the selected direction and counterparty scope. An observation is confirmation-backed only when OpenChain obtained a current chain height and the network confirmation threshold was met; this is not an evidentiary finality claim."

type Engine struct {
	chainAdapter  adapter.ChainAdapter
	database      *db.DB
	labelRegistry *labels.Service
}

func NewEngine(chainAdapter adapter.ChainAdapter, database *db.DB, labels *labels.Service) *Engine {
	return &Engine{chainAdapter: chainAdapter, database: database, labelRegistry: labels}
}

func (e *Engine) ResolveGraph(ctx context.Context, address string, direction Direction, limit uint32, cursor string, maxCounterparties uint32, ranking Ranking, maxDepth uint32) (*GraphResult, error) {
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
	maxDepth = graphDepth(maxDepth)
	acquisitionContext, recorder := adapter.WithAcquisitionRecorder(ctx)
	page, err := e.chainAdapter.ListTransfers(acquisitionContext, address, limit, cursor)
	if err != nil {
		if e.database != nil {
			scope := db.AcquisitionScope{Network: e.Network(), Address: address, Cursor: cursor, RetrievedAt: time.Now().UTC()}
			if persistErr := e.database.SaveAcquisitions(ctx, scope, recorder.Items()); persistErr != nil {
				return nil, persistErr
			}
		}
		return nil, err
	}
	page.SourceStatus = confirmationSourceStatus(e.Network(), page.SourceStatus)
	filtered := filterTransfers(page.Transfers, address, direction)
	transfers := selectCounterpartyTransfersWithKnown(filtered, address, maxCounterparties, ranking, e.knownCounterparties(ctx, filtered, address, ranking))
	persistedTransfers := e.toTransfers(transfers, page.SourceStatus)
	completedAt := time.Now().UTC()
	leads, runs := rules.Evaluate(e.Network(), persistedTransfers, completedAt)
	result := e.graph(ctx, address, transfers, page)
	if e.database != nil {
		retrievedAt := page.SourceStatus.RetrievedAt
		if retrievedAt.IsZero() {
			retrievedAt = completedAt
		}
		scope := db.AcquisitionScope{Network: e.Network(), Address: address, Cursor: cursor, RetrievedAt: retrievedAt}
		if err := e.database.SaveEvidenceGraph(ctx, scope, e.toAddresses(result.Nodes), persistedTransfers, recorder.Items()); err != nil {
			return nil, err
		}
		storedTransfers, err := e.traverseStoredGraph(ctx, address, direction, limit, maxCounterparties, ranking, maxDepth)
		if err != nil {
			return nil, err
		}
		result = e.graphFromStored(ctx, address, storedTransfers, page)
		if err := e.database.SaveRuleRuns(ctx, runs); err != nil {
			return nil, err
		}
	}
	result.Coverage = traceCoverage(limit, cursor, page, persistedTransfers)
	result.Coverage.Limitation = retrievedScopeLimitation + " AGE traversal only includes persisted observations within the selected depth and bounded per-address graph scope."
	result.Leads = leads
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

func graphDepth(depth uint32) uint32 {
	if depth == 0 {
		return DefaultGraphDepth
	}
	if depth > MaxGraphDepth {
		return MaxGraphDepth
	}
	return depth
}

type counterpartyStats struct {
	address string
	count   int
	latest  time.Time
	totals  map[string]*big.Int
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
			item = &counterpartyStats{address: counterparty, totals: make(map[string]*big.Int), known: known[counterparty]}
			stats[counterparty] = item
		}
		item.count++
		if transfer.Timestamp.After(item.latest) {
			item.latest = transfer.Timestamp
		}
		amount, ok := new(big.Int).SetString(transfer.AmountBaseUnits, 10)
		if ok {
			assetKey := assetKey(transfer.Asset)
			total := item.totals[assetKey]
			if total == nil {
				total = big.NewInt(0)
				item.totals[assetKey] = total
			}
			total.Add(total, amount)
		}
	}
	items := make([]*counterpartyStats, 0, len(stats))
	for _, item := range stats {
		items = append(items, item)
	}
	if ranking == RankingTotalRawAmount {
		return selectCounterpartiesByAsset(transfers, stats, maxCounterparties, seed)
	}
	sort.Slice(items, func(left, right int) bool {
		first, second := items[left], items[right]
		switch ranking {
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

func selectCounterpartiesByAsset(transfers []adapter.TransferItem, stats map[string]*counterpartyStats, limit uint32, seed string) []adapter.TransferItem {
	byAsset := make(map[string][]*counterpartyStats)
	for _, item := range stats {
		for key := range item.totals {
			byAsset[key] = append(byAsset[key], item)
		}
	}
	keys := make([]string, 0, len(byAsset))
	for key := range byAsset {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selected := make(map[string]struct{})
	for _, key := range keys {
		items := byAsset[key]
		sort.Slice(items, func(left, right int) bool {
			first, second := items[left], items[right]
			if compared := first.totals[key].Cmp(second.totals[key]); compared != 0 {
				return compared > 0
			}
			if !first.latest.Equal(second.latest) {
				return first.latest.After(second.latest)
			}
			return first.address < second.address
		})
		for _, item := range items[:min(int(limit), len(items))] {
			selected[item.address] = struct{}{}
		}
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

// traverseStoredGraph performs a bounded breadth-first traversal over AGE.
// Every returned relationship is an already-persisted transfer, so callers can
// still export the exact relational evidence and acquisition scope behind it.
func (e *Engine) traverseStoredGraph(ctx context.Context, seed string, direction Direction, limit, maxCounterparties uint32, ranking Ranking, maxDepth uint32) ([]db.Transfer, error) {
	if e.database == nil {
		return nil, nil
	}
	seed = e.normalizeAddress(seed)
	depth := graphDepth(maxDepth)
	frontier := []string{seed}
	visited := map[string]struct{}{seed: {}}
	stored := make(map[string]db.Transfer)
	for hop := uint32(0); hop < depth && len(frontier) > 0 && len(stored) < maxGraphTraversalEdges; hop++ {
		next := make([]string, 0, len(frontier)*int(maxCounterparties))
		for _, current := range frontier {
			if len(stored) >= maxGraphTraversalEdges || len(visited) >= maxGraphTraversalNodes {
				break
			}
			neighbors, err := e.database.GraphNeighbors(ctx, e.Network(), current, string(direction), int(limit))
			if err != nil {
				return nil, err
			}
			items := storedItems(neighbors)
			items = filterTransfers(items, current, direction)
			selected := selectCounterpartyTransfersWithKnown(items, current, maxCounterparties, ranking, e.knownCounterparties(ctx, items, current, ranking))
			selectedIDs := make(map[string]struct{}, len(selected))
			for _, transfer := range selected {
				selectedIDs[transferID(e.Network(), transfer.Hash, transfer.EventID)] = struct{}{}
			}
			for _, transfer := range neighbors {
				if _, ok := selectedIDs[transfer.ID]; !ok {
					continue
				}
				stored[transfer.ID] = transfer
				neighbor := transfer.ToAddress
				if strings.EqualFold(neighbor, current) {
					neighbor = transfer.FromAddress
				}
				neighbor = e.normalizeAddress(neighbor)
				if _, seen := visited[neighbor]; !seen && len(visited) < maxGraphTraversalNodes {
					visited[neighbor] = struct{}{}
					next = append(next, neighbor)
				}
			}
		}
		sort.Strings(next)
		frontier = next
	}
	result := make([]db.Transfer, 0, len(stored))
	for _, transfer := range stored {
		result = append(result, transfer)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, nil
}

func storedItems(transfers []db.Transfer) []adapter.TransferItem {
	items := make([]adapter.TransferItem, 0, len(transfers))
	for _, transfer := range transfers {
		items = append(items, adapter.TransferItem{Hash: transfer.TransactionHash, EventID: transfer.EventID, TransferKind: transfer.TransferKind, From: transfer.FromAddress, To: transfer.ToAddress, AmountBaseUnits: transfer.AmountBaseUnits, Asset: transfer.Asset, BlockNumber: transfer.BlockNumber, BlockHash: transfer.BlockHash, Timestamp: transfer.BlockTimestamp})
	}
	return items
}

func (e *Engine) PendingGraph(address, warning string) *GraphResult {
	seed := e.normalizeAddress(address)
	return &GraphResult{
		Network:     e.Network(),
		SeedAddress: seed,
		Nodes:       []GraphNode{{ID: seed, Label: shortAddress(seed), EntityType: "EOA", IsSeed: true}},
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
	edges := make([]GraphEdge, 0, len(transfers))
	for _, transfer := range transfers {
		edges = append(edges, GraphEdge{ID: transferID(e.Network(), transfer.Hash, transfer.EventID), Source: e.normalizeAddress(transfer.From), Target: e.normalizeAddress(transfer.To), AmountBaseUnits: transfer.AmountBaseUnits, AmountFormatted: formatAmount(transfer.AmountBaseUnits, transfer.Asset), TxCount: 1, Asset: transfer.Asset, EventID: transfer.EventID, TransactionHash: transfer.Hash, TransferKind: transfer.TransferKind, SourceName: page.SourceStatus.Source, BlockNumber: uint64(transfer.BlockNumber), BlockHash: transfer.BlockHash, Timestamp: transfer.Timestamp.Unix(), RetrievedAt: page.SourceStatus.RetrievedAt.Unix(), Provisional: isProvisional(e.Network(), transfer.BlockNumber, page.SourceStatus)})
	}
	return e.graphFromEdges(ctx, seed, edges, page)
}

func (e *Engine) graphFromStored(ctx context.Context, seed string, transfers []db.Transfer, page *adapter.TransferPage) *GraphResult {
	edges := make([]GraphEdge, 0, len(transfers))
	for _, transfer := range transfers {
		edges = append(edges, GraphEdge{ID: transfer.ID, Source: e.normalizeAddress(transfer.FromAddress), Target: e.normalizeAddress(transfer.ToAddress), AmountBaseUnits: transfer.AmountBaseUnits, AmountFormatted: formatAmount(transfer.AmountBaseUnits, transfer.Asset), TxCount: 1, Asset: transfer.Asset, EventID: transfer.EventID, TransactionHash: transfer.TransactionHash, TransferKind: transfer.TransferKind, SourceName: transfer.Source, BlockNumber: uint64(transfer.BlockNumber), BlockHash: transfer.BlockHash, Timestamp: transfer.BlockTimestamp.Unix(), RetrievedAt: transfer.RetrievedAt.Unix(), Provisional: transfer.Provisional})
	}
	return e.graphFromEdges(ctx, seed, edges, page)
}

func (e *Engine) graphFromEdges(ctx context.Context, seed string, edges []GraphEdge, page *adapter.TransferPage) *GraphResult {
	seed = e.normalizeAddress(seed)
	nodes := map[string]GraphNode{seed: e.node(ctx, seed, true)}
	inCounts, outCounts := map[string]uint32{}, map[string]uint32{}
	volumes := map[string]map[string]*big.Int{}
	assets := map[string]adapter.Asset{}
	for _, edge := range edges {
		from, to := e.normalizeAddress(edge.Source), e.normalizeAddress(edge.Target)
		if _, ok := nodes[from]; !ok {
			nodes[from] = e.node(ctx, from, from == seed)
		}
		if _, ok := nodes[to]; !ok {
			nodes[to] = e.node(ctx, to, to == seed)
		}
		outCounts[from]++
		inCounts[to]++
		if amount, ok := new(big.Int).SetString(edge.AmountBaseUnits, 10); ok {
			key := assetKey(edge.Asset)
			assets[key] = edge.Asset
			addAssetVolume(volumes, from, key, amount)
			addAssetVolume(volumes, to, key, amount)
		}
	}
	graphNodes := make([]GraphNode, 0, len(nodes))
	for id, node := range nodes {
		node.InTxCount, node.OutTxCount = inCounts[id], outCounts[id]
		node.TotalVolumeByAsset = sortedAssetVolumes(volumes[id], assets)
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
	label, entityType := shortAddress(address), "UNKNOWN"
	var nodeLabels []labels.LabelItem
	if e.labelRegistry != nil {
		if items, err := e.labelRegistry.GetLabels(ctx, e.Network(), address); err == nil && len(items) > 0 {
			label = items[0].Label
			nodeLabels = items
			if items[0].Category == labels.CategoryExchange {
				entityType = "EXCHANGE"
			}
		}
	}
	if e.chainAdapter != nil && seed && e.chainAdapter.Capabilities().EntityClassification {
		if contract, err := e.chainAdapter.IsContract(ctx, address); err == nil {
			if contract {
				entityType = "CONTRACT"
			} else {
				entityType = "EOA"
			}
		}
	}
	return GraphNode{ID: address, Label: label, EntityType: entityType, IsSeed: seed, Labels: nodeLabels}
}

func addAssetVolume(volumes map[string]map[string]*big.Int, address, key string, amount *big.Int) {
	if volumes[address] == nil {
		volumes[address] = make(map[string]*big.Int)
	}
	if volumes[address][key] == nil {
		volumes[address][key] = big.NewInt(0)
	}
	volumes[address][key].Add(volumes[address][key], amount)
}

func sortedAssetVolumes(volumes map[string]*big.Int, assets map[string]adapter.Asset) []AssetVolume {
	keys := make([]string, 0, len(volumes))
	for key := range volumes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]AssetVolume, 0, len(keys))
	for _, key := range keys {
		result = append(result, AssetVolume{Asset: assets[key], AmountBaseUnits: volumes[key].String()})
	}
	return result
}

func assetKey(asset adapter.Asset) string {
	return strings.Join([]string{asset.Kind, asset.ContractAddress, asset.Symbol, fmt.Sprintf("%d", asset.Decimals)}, ":")
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
		transfers = append(transfers, db.Transfer{ID: transferID(e.Network(), item.Hash, item.EventID), Network: e.Network(), TransactionHash: item.Hash, EventID: item.EventID, TransferKind: item.TransferKind, FromAddress: e.normalizeAddress(item.From), ToAddress: e.normalizeAddress(item.To), Asset: item.Asset, AmountBaseUnits: item.AmountBaseUnits, BlockNumber: item.BlockNumber, BlockHash: item.BlockHash, BlockTimestamp: item.Timestamp, Provisional: isProvisional(e.Network(), item.BlockNumber, source), Source: source.Source, RetrievedAt: source.RetrievedAt})
	}
	return transfers
}

func traceCoverage(limit uint32, cursor string, page *adapter.TransferPage, transfers []db.Transfer) TraceCoverage {
	coverage := TraceCoverage{
		RequestedPageSize:     limit,
		ObservedTransferCount: uint32(len(page.Transfers)),
		GraphTransferCount:    uint32(len(transfers)),
		Cursor:                cursor,
		HasMore:               page.HasMore,
		ProviderComplete:      page.SourceStatus.IsComplete,
		Limitation:            retrievedScopeLimitation,
	}
	for _, transfer := range transfers {
		if transfer.Provisional {
			coverage.ProvisionalTransferCount++
		} else {
			coverage.ConfirmationBackedTransferCount++
		}
	}
	return coverage
}

func isProvisional(network string, blockNumber int64, source adapter.SourceStatus) bool {
	threshold, supported := confirmationThreshold(network)
	if !supported || blockNumber <= 0 || source.LatestChainBlock <= 0 || source.LatestChainBlock < blockNumber {
		return true
	}
	return source.LatestChainBlock-blockNumber < threshold
}

func confirmationThreshold(network string) (int64, bool) {
	switch network {
	case "ethereum-mainnet", "bnb-chain":
		return 64, true
	case "base-mainnet", "optimism-mainnet", "arbitrum-one":
		return 120, true
	case "polygon-mainnet":
		return 256, true
	default:
		return 0, false
	}
}

func confirmationSourceStatus(network string, status adapter.SourceStatus) adapter.SourceStatus {
	if _, supported := confirmationThreshold(network); supported && status.LatestChainBlock > 0 {
		return status
	}
	const warning = "Confirmation-backed status is unavailable for this network; observations remain provisional."
	if !strings.Contains(status.Warning, "observations remain provisional") {
		if status.Warning != "" {
			status.Warning += " "
		}
		status.Warning += warning
	}
	return status
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
