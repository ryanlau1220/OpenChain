package api

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"net/http"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/rules"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

type connectTracingHandler struct{ server *Server }

func parseEntityType(value string) pb.EntityType {
	switch value {
	case "CONTRACT":
		return pb.EntityType_ENTITY_TYPE_CONTRACT
	case "EXCHANGE":
		return pb.EntityType_ENTITY_TYPE_EXCHANGE
	default:
		return pb.EntityType_ENTITY_TYPE_EOA
	}
}

func graphRanking(value pb.GraphRanking) tracing.Ranking {
	switch value {
	case pb.GraphRanking_GRAPH_RANKING_TOTAL_RAW_AMOUNT:
		return tracing.RankingTotalRawAmount
	case pb.GraphRanking_GRAPH_RANKING_MOST_ACTIVE:
		return tracing.RankingMostActive
	case pb.GraphRanking_GRAPH_RANKING_KNOWN_ENTITY:
		return tracing.RankingKnownEntity
	default:
		return tracing.RankingMostRecent
	}
}

func (h *connectTracingHandler) TraceGraph(ctx context.Context, req *connect.Request[pb.TraceGraphRequest]) (*connect.Response[pb.TraceGraphResponse], error) {
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	address, err := runtime.Chain.NormalizeAddress(req.Msg.GetSeedAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.traceGraph(ctx, req.Msg.GetNetwork(), address, traceDirection(req.Msg.GetDirection()), req.Msg.GetLimit(), req.Msg.GetCursor(), req.Msg.GetMaxCounterparties(), graphRanking(req.Msg.GetRanking()), req.Msg.GetRetry())
	if err != nil {
		if errors.Is(err, errUnsupportedNetwork) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if errors.Is(err, tracing.ErrQueueFull) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("trace queue is full; try again shortly"))
		}
		if errors.Is(err, tracing.ErrClientQueueFull) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("your trace queue is full; wait for a queued trace to start"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.TraceGraphResponse{SeedAddress: result.SeedAddress, Nodes: nodes, Edges: edges, TotalNodes: result.TotalNodes, TotalEdges: result.TotalEdges, NextCursor: result.NextCursor, HasMore: result.HasMore, SourceStatus: toSourceStatus(result.SourceStatus), Pending: result.Pending, Leads: toLeadProto(result.Leads), Coverage: toCoverageProto(result.Coverage)}), nil
}

func (h *connectTracingHandler) GetTraceStatus(ctx context.Context, req *connect.Request[pb.TraceStatusRequest]) (*connect.Response[pb.TraceGraphResponse], error) {
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	address, err := runtime.Chain.NormalizeAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.traceStatus(ctx, req.Msg.GetNetwork(), address, traceDirection(req.Msg.GetDirection()), req.Msg.GetLimit(), req.Msg.GetCursor(), req.Msg.GetMaxCounterparties(), graphRanking(req.Msg.GetRanking()))
	if err != nil {
		if errors.Is(err, tracing.ErrTraceNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, errors.New("trace job was not found"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.TraceGraphResponse{SeedAddress: result.SeedAddress, Nodes: nodes, Edges: edges, TotalNodes: result.TotalNodes, TotalEdges: result.TotalEdges, NextCursor: result.NextCursor, HasMore: result.HasMore, SourceStatus: toSourceStatus(result.SourceStatus), Pending: result.Pending, Leads: toLeadProto(result.Leads), Coverage: toCoverageProto(result.Coverage)}), nil
}

func (h *connectTracingHandler) ExpandNode(ctx context.Context, req *connect.Request[pb.ExpandNodeRequest]) (*connect.Response[pb.ExpandNodeResponse], error) {
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	address, err := runtime.Chain.NormalizeAddress(req.Msg.GetNodeAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.traceGraph(ctx, req.Msg.GetNetwork(), address, traceDirection(req.Msg.GetDirection()), req.Msg.GetLimit(), req.Msg.GetCursor(), req.Msg.GetMaxCounterparties(), graphRanking(req.Msg.GetRanking()), req.Msg.GetRetry())
	if err != nil {
		if errors.Is(err, errUnsupportedNetwork) {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		if errors.Is(err, tracing.ErrQueueFull) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("trace queue is full; try again shortly"))
		}
		if errors.Is(err, tracing.ErrClientQueueFull) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("your trace queue is full; wait for a queued trace to start"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.ExpandNodeResponse{NewNodes: nodes, NewEdges: edges, NextCursor: result.NextCursor, HasMore: result.HasMore, SourceStatus: toSourceStatus(result.SourceStatus), Pending: result.Pending, Leads: toLeadProto(result.Leads), Coverage: toCoverageProto(result.Coverage)}), nil
}

func toGraphProto(result *tracing.GraphResult) ([]*pb.GraphNode, []*pb.GraphEdge) {
	nodes := make([]*pb.GraphNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodeLabels := make([]*pb.AddressLabel, 0, len(node.Labels))
		for _, label := range node.Labels {
			nodeLabels = append(nodeLabels, toLabelProto(label))
		}
		volumes := make([]*pb.AssetVolume, 0, len(node.TotalVolumeByAsset))
		for _, volume := range node.TotalVolumeByAsset {
			volumes = append(volumes, &pb.AssetVolume{Asset: &pb.Asset{Kind: volume.Asset.Kind, ContractAddress: volume.Asset.ContractAddress, Symbol: volume.Asset.Symbol, Decimals: volume.Asset.Decimals}, AmountBaseUnits: volume.AmountBaseUnits})
		}
		nodes = append(nodes, &pb.GraphNode{Id: node.ID, Label: node.Label, EntityType: parseEntityType(node.EntityType), IsSeed: node.IsSeed, TotalVolumeByAsset: volumes, InTxCount: node.InTxCount, OutTxCount: node.OutTxCount, Labels: nodeLabels})
	}
	edges := make([]*pb.GraphEdge, 0, len(result.Edges))
	for _, edge := range result.Edges {
		edges = append(edges, &pb.GraphEdge{Id: edge.ID, Source: edge.Source, Target: edge.Target, AmountBaseUnits: edge.AmountBaseUnits, AmountFormatted: edge.AmountFormatted, TxCount: edge.TxCount, Asset: &pb.Asset{Kind: edge.Asset.Kind, ContractAddress: edge.Asset.ContractAddress, Symbol: edge.Asset.Symbol, Decimals: edge.Asset.Decimals}, EventId: edge.EventID, BlockNumber: edge.BlockNumber, BlockHash: edge.BlockHash, TransactionHash: edge.TransactionHash, TransferKind: edge.TransferKind, SourceName: edge.SourceName, RetrievedAt: edge.RetrievedAt, FirstTxTimestamp: edge.Timestamp, LastTxTimestamp: edge.Timestamp, Provisional: edge.Provisional})
	}
	return nodes, edges
}

func toCoverageProto(coverage tracing.TraceCoverage) *pb.TraceCoverage {
	return &pb.TraceCoverage{RequestedPageSize: coverage.RequestedPageSize, ObservedTransferCount: coverage.ObservedTransferCount, GraphTransferCount: coverage.GraphTransferCount, ConfirmationBackedTransferCount: coverage.ConfirmationBackedTransferCount, ProvisionalTransferCount: coverage.ProvisionalTransferCount, Cursor: coverage.Cursor, HasMore: coverage.HasMore, ProviderComplete: coverage.ProviderComplete, Limitation: coverage.Limitation}
}

func toLeadProto(leads []rules.Lead) []*pb.InvestigationLead {
	result := make([]*pb.InvestigationLead, 0, len(leads))
	for _, lead := range leads {
		result = append(result, &pb.InvestigationLead{Id: lead.ID, RuleId: lead.RuleID, RuleVersion: lead.RuleVersion, Title: lead.Title, SubjectAddress: lead.SubjectAddress, TransferIds: lead.TransferIDs, Rationale: lead.Rationale, Limitations: lead.Limitations, ParametersJson: string(lead.Parameters)})
	}
	return result
}

type connectLookupHandler struct{ server *Server }

func (h *connectLookupHandler) LookupAddress(ctx context.Context, req *connect.Request[pb.LookupAddressRequest]) (*connect.Response[pb.LookupAddressResponse], error) {
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	address, err := runtime.Chain.NormalizeAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	summary := &pb.AddressSummary{Address: address, Network: req.Msg.GetNetwork(), EntityType: pb.EntityType_ENTITY_TYPE_UNSPECIFIED, Label: shortAddress(address)}
	fieldStatuses := make([]*pb.LookupFieldStatus, 0, 3)

	balanceAvailable := false
	var txCount uint64
	if value, callErr := runtime.Chain.GetBalance(ctx, address); callErr == nil {
		summary.BalanceBaseUnits = value.String()
		summary.BalanceFormatted = adapter.FormatAmount(value, runtime.Chain.NativeAsset())
		balanceAvailable = true
	}
	fieldStatuses = append(fieldStatuses, lookupFieldStatus(pb.LookupField_LOOKUP_FIELD_BALANCE, balanceAvailable, "Balance data is temporarily unavailable."))

	if runtime.Chain.ActivityLabel() != "" {
		activityAvailable := false
		if value, callErr := runtime.Chain.GetTxCount(ctx, address); callErr == nil {
			txCount = value
			activityAvailable = true
		}
		summary.TxCount = txCount
		fieldStatuses = append(fieldStatuses, lookupFieldStatus(pb.LookupField_LOOKUP_FIELD_ACTIVITY, activityAvailable, "Activity data is temporarily unavailable."))
	}

	entityAvailable := false
	if value, callErr := runtime.Chain.IsContract(ctx, address); callErr == nil {
		entityAvailable = true
		if value {
			summary.EntityType = pb.EntityType_ENTITY_TYPE_CONTRACT
		} else {
			summary.EntityType = pb.EntityType_ENTITY_TYPE_EOA
		}
	}
	if h.server.labels != nil {
		if items, labelErr := h.server.labels.GetLabels(ctx, runtime.Engine.Network(), address); labelErr == nil && len(items) > 0 {
			entityAvailable = true
			summary.Label = items[0].Label
			if items[0].Category == labels.CategoryExchange {
				summary.EntityType = pb.EntityType_ENTITY_TYPE_EXCHANGE
			}
		}
	}
	fieldStatuses = append(fieldStatuses, lookupFieldStatus(pb.LookupField_LOOKUP_FIELD_ENTITY_TYPE, entityAvailable, "Entity type data is temporarily unavailable."))
	return connect.NewResponse(&pb.LookupAddressResponse{Summary: summary, SourceStatus: toSourceStatus(runtime.Engine.SourceStatus(ctx)), FieldStatuses: fieldStatuses}), nil
}

func (h *connectLookupHandler) LookupTransaction(ctx context.Context, req *connect.Request[pb.LookupTransactionRequest]) (*connect.Response[pb.LookupTransactionResponse], error) {
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	hash, err := runtime.Chain.NormalizeTransactionHash(req.Msg.GetHash())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	transaction, source, err := runtime.Engine.LookupTransaction(ctx, hash)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	return connect.NewResponse(&pb.LookupTransactionResponse{Transaction: &pb.TransactionItem{Hash: transaction.Hash, BlockNumber: uint64(transaction.BlockNumber), Timestamp: transaction.Timestamp.Unix(), FromAddress: transaction.From, ToAddress: transaction.To, ValueBaseUnits: transaction.ValueBaseUnits, ValueFormatted: adapter.FormatAmount(bigInt(transaction.ValueBaseUnits), runtime.Chain.NativeAsset()), Status: pb.TransactionStatus_TRANSACTION_STATUS_UNKNOWN}, SourceStatus: toSourceStatus(source), FieldStatuses: []*pb.LookupFieldStatus{lookupFieldStatus(pb.LookupField_LOOKUP_FIELD_TRANSACTION_STATUS, false, "Transaction execution status is unavailable from the configured source.")}}), nil
}

func lookupFieldStatus(field pb.LookupField, available bool, warning string) *pb.LookupFieldStatus {
	if available {
		warning = ""
	}
	return &pb.LookupFieldStatus{Field: field, Available: available, Warning: warning}
}

func bigInt(value string) *big.Int {
	number, ok := new(big.Int).SetString(value, 10)
	if !ok {
		return big.NewInt(0)
	}
	return number
}

func traceDirection(direction pb.TraceDirection) tracing.Direction {
	if direction == pb.TraceDirection_TRACE_DIRECTION_INBOUND {
		return tracing.DirectionInbound
	}
	if direction == pb.TraceDirection_TRACE_DIRECTION_OUTBOUND {
		return tracing.DirectionOutbound
	}
	return tracing.DirectionBoth
}

func toSourceStatus(status adapter.SourceStatus) *pb.SourceStatus {
	return &pb.SourceStatus{Source: status.Source, RetrievedAt: status.RetrievedAt.Unix(), IndexedUpToBlock: status.IndexedUpToBlock, LatestChainBlock: status.LatestChainBlock, IsComplete: status.IsComplete, Warning: status.Warning}
}

type connectLabelHandler struct{ server *Server }

func toLabelProto(item labels.LabelItem) *pb.AddressLabel {
	return &pb.AddressLabel{Id: item.ID, Address: item.Address, Network: protoNetwork(item.Network), Category: string(item.Category), Label: item.Label, Confidence: item.Confidence, EvidenceUrl: item.EvidenceURL, Source: item.Source, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt.Unix(), TrustTier: pb.TrustTier(item.TrustTier), SourceVersion: item.SourceVersion, Visibility: pb.LabelVisibility_LABEL_VISIBILITY_PUBLIC}
}

func protoNetwork(network string) pb.Network {
	switch network {
	case "base-mainnet":
		return pb.Network_NETWORK_BASE_MAINNET
	case "solana-mainnet":
		return pb.Network_NETWORK_SOLANA_MAINNET
	case "tron-mainnet":
		return pb.Network_NETWORK_TRON_MAINNET
	case "polygon-mainnet":
		return pb.Network_NETWORK_POLYGON_MAINNET
	case "arbitrum-one":
		return pb.Network_NETWORK_ARBITRUM_ONE
	case "optimism-mainnet":
		return pb.Network_NETWORK_OPTIMISM_MAINNET
	case "bnb-chain":
		return pb.Network_NETWORK_BNB_CHAIN
	case "ton-mainnet":
		return pb.Network_NETWORK_TON_MAINNET
	case "cardano-mainnet":
		return pb.Network_NETWORK_CARDANO_MAINNET
	}
	return pb.Network_NETWORK_ETHEREUM_MAINNET
}

func (h *connectLabelHandler) GetLabels(ctx context.Context, req *connect.Request[pb.GetLabelsRequest]) (*connect.Response[pb.GetLabelsResponse], error) {
	if h.server.labels == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("curated labels are unavailable"))
	}
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	address, err := runtime.Chain.NormalizeAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, err := h.server.labels.GetLabels(ctx, runtime.Engine.Network(), address)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	result := make([]*pb.AddressLabel, 0, len(items))
	for _, item := range items {
		result = append(result, toLabelProto(item))
	}
	return connect.NewResponse(&pb.GetLabelsResponse{Labels: result}), nil
}

func (h *connectLabelHandler) SearchLabels(ctx context.Context, req *connect.Request[pb.SearchLabelsRequest]) (*connect.Response[pb.SearchLabelsResponse], error) {
	if len(req.Msg.GetQuery()) > 100 || len(req.Msg.GetCategory()) > 64 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("label search is too long"))
	}
	if req.Msg.GetCategory() != "" && !labels.ValidCategory(labels.EntityCategory(req.Msg.GetCategory())) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported entity category"))
	}
	limit := int(req.Msg.GetLimit())
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if h.server.labels == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("curated labels are unavailable"))
	}
	runtime, err := h.server.network(req.Msg.GetNetwork())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items, err := h.server.labels.SearchLabels(ctx, runtime.Engine.Network(), req.Msg.GetQuery(), req.Msg.GetCategory(), limit)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	result := make([]*pb.AddressLabel, 0, len(items))
	for _, item := range items {
		result = append(result, toLabelProto(item))
	}
	return connect.NewResponse(&pb.SearchLabelsResponse{Labels: result}), nil
}

func (s *Server) RegisterConnectRPC(mux *http.ServeMux) {
	mount := func(path string, handler http.Handler) { mux.Handle(path, withLogging(handler.ServeHTTP)) }
	options := []connect.HandlerOption{connect.WithInterceptors(connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, request connect.AnyRequest) (connect.AnyResponse, error) {
			if rateLimitProcedure(request.Spec().Procedure) && !s.requestLimiter.Allow(clientKey(ctx)) {
				err := connect.NewError(connect.CodeResourceExhausted, errors.New("request limit reached; try again in one minute"))
				err.Meta().Set("Retry-After", "60")
				return nil, err
			}
			return next(ctx, request)
		}
	})), connect.WithReadMaxBytes(1 << 20)}
	path, handler := openchainv1connect.NewTracingServiceHandler(&connectTracingHandler{server: s}, options...)
	mount(path, handler)
	path, handler = openchainv1connect.NewLookupServiceHandler(&connectLookupHandler{server: s}, options...)
	mount(path, handler)
	path, handler = openchainv1connect.NewLabelServiceHandler(&connectLabelHandler{server: s}, options...)
	mount(path, handler)
	path, handler = openchainv1connect.NewEvidenceServiceHandler(&connectEvidenceHandler{server: s}, options...)
	mount(path, handler)
}

func rateLimitProcedure(procedure string) bool {
	switch procedure {
	case openchainv1connect.TracingServiceTraceGraphProcedure,
		openchainv1connect.TracingServiceExpandNodeProcedure,
		openchainv1connect.LookupServiceLookupAddressProcedure,
		openchainv1connect.LookupServiceLookupTransactionProcedure,
		openchainv1connect.EvidenceServiceExportEvidencePackageProcedure:
		return true
	default:
		return false
	}
}
