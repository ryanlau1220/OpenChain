package api

import (
	"context"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/labels"
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

func (h *connectTracingHandler) TraceGraph(ctx context.Context, req *connect.Request[pb.TraceGraphRequest]) (*connect.Response[pb.TraceGraphResponse], error) {
	address, err := ethereumAddress(req.Msg.GetSeedAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.traceGraph(ctx, address, traceDirection(req.Msg.GetDirection()), req.Msg.GetLimit(), req.Msg.GetCursor(), req.Msg.GetRetry())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.TraceGraphResponse{SeedAddress: result.SeedAddress, Nodes: nodes, Edges: edges, TotalNodes: result.TotalNodes, TotalEdges: result.TotalEdges, NextCursor: result.NextCursor, HasMore: result.HasMore, SourceStatus: toSourceStatus(result.SourceStatus), Pending: result.Pending}), nil
}

func (h *connectTracingHandler) ExpandNode(ctx context.Context, req *connect.Request[pb.ExpandNodeRequest]) (*connect.Response[pb.ExpandNodeResponse], error) {
	address, err := ethereumAddress(req.Msg.GetNodeAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.traceGraph(ctx, address, traceDirection(req.Msg.GetDirection()), req.Msg.GetLimit(), req.Msg.GetCursor(), req.Msg.GetRetry())
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.ExpandNodeResponse{NewNodes: nodes, NewEdges: edges, NextCursor: result.NextCursor, HasMore: result.HasMore, SourceStatus: toSourceStatus(result.SourceStatus), Pending: result.Pending}), nil
}

func toGraphProto(result *tracing.GraphResult) ([]*pb.GraphNode, []*pb.GraphEdge) {
	nodes := make([]*pb.GraphNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodeLabels := make([]*pb.AddressLabel, 0, len(node.Labels))
		for _, label := range node.Labels {
			nodeLabels = append(nodeLabels, toLabelProto(label))
		}
		nodes = append(nodes, &pb.GraphNode{Id: node.ID, Label: node.Label, EntityType: parseEntityType(node.EntityType), IsSeed: node.IsSeed, TotalVolumeWei: node.TotalVolumeWei, InTxCount: node.InTxCount, OutTxCount: node.OutTxCount, Labels: nodeLabels})
	}
	edges := make([]*pb.GraphEdge, 0, len(result.Edges))
	for _, edge := range result.Edges {
		edges = append(edges, &pb.GraphEdge{Id: edge.ID, Source: edge.Source, Target: edge.Target, ValueWei: edge.ValueWei, ValueFormatted: edge.ValueFormatted, TxCount: edge.TxCount, AssetSymbol: edge.AssetSymbol, EventIndex: edge.EventIndex, BlockNumber: edge.BlockNumber, TransactionHash: edge.TransactionHash, FirstTxTimestamp: edge.Timestamp, LastTxTimestamp: edge.Timestamp})
	}
	return nodes, edges
}

type connectLookupHandler struct{ server *Server }

func (h *connectLookupHandler) LookupAddress(ctx context.Context, req *connect.Request[pb.LookupAddressRequest]) (*connect.Response[pb.LookupAddressResponse], error) {
	address, err := ethereumAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	balance := big.NewInt(0)
	var txCount uint64
	var isContract bool
	if h.server.evm != nil {
		if value, callErr := h.server.evm.GetBalance(ctx, address); callErr == nil {
			balance = value
		}
		if value, callErr := h.server.evm.GetTxCount(ctx, address); callErr == nil {
			txCount = value
		}
		if value, callErr := h.server.evm.IsContract(ctx, address); callErr == nil {
			isContract = value
		}
	}
	entityType, label := pb.EntityType_ENTITY_TYPE_EOA, shortAddress(address)
	if isContract {
		entityType = pb.EntityType_ENTITY_TYPE_CONTRACT
	}
	if h.server.labels != nil {
		if items, labelErr := h.server.labels.GetLabels(ctx, address); labelErr == nil && len(items) > 0 {
			label = items[0].Label
			if strings.EqualFold(items[0].Category, "exchange") {
				entityType = pb.EntityType_ENTITY_TYPE_EXCHANGE
			}
		}
	}
	return connect.NewResponse(&pb.LookupAddressResponse{Summary: &pb.AddressSummary{Address: address, Network: pb.Network_NETWORK_ETHEREUM_MAINNET, EntityType: entityType, Label: label, BalanceWei: balance.String(), BalanceFormatted: adapter.FormatWeiToETH(balance), TxCount: txCount}, SourceStatus: toSourceStatus(h.server.tracingEngine.SourceStatus(ctx))}), nil
}

func (h *connectLookupHandler) LookupTransaction(ctx context.Context, req *connect.Request[pb.LookupTransactionRequest]) (*connect.Response[pb.LookupTransactionResponse], error) {
	hash, err := transactionHash(req.Msg.GetHash())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	transaction, source, err := h.server.tracingEngine.LookupTransaction(ctx, hash)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	return connect.NewResponse(&pb.LookupTransactionResponse{Transaction: &pb.TransactionItem{Hash: transaction.Hash, BlockNumber: uint64(transaction.BlockNumber), Timestamp: transaction.Timestamp.Unix(), FromAddress: transaction.From, ToAddress: transaction.To, ValueWei: transaction.ValueWei, ValueFormatted: adapter.FormatWeiToETH(bigInt(transaction.ValueWei)), StatusSuccess: true}, SourceStatus: toSourceStatus(source)}), nil
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
	return &pb.AddressLabel{Id: item.ID, Address: item.Address, Network: pb.Network_NETWORK_ETHEREUM_MAINNET, Category: item.Category, Label: item.Label, Confidence: item.Confidence, EvidenceUrl: item.EvidenceURL, Source: item.Source, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt.Unix(), TrustTier: pb.TrustTier(item.TrustTier), SourceVersion: item.SourceVersion, Visibility: pb.LabelVisibility_LABEL_VISIBILITY_PUBLIC}
}

func (h *connectLabelHandler) GetLabels(ctx context.Context, req *connect.Request[pb.GetLabelsRequest]) (*connect.Response[pb.GetLabelsResponse], error) {
	address, err := ethereumAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if h.server.labels == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("curated labels are unavailable"))
	}
	items, err := h.server.labels.GetLabels(ctx, address)
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
	limit := int(req.Msg.GetLimit())
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	if h.server.labels == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("curated labels are unavailable"))
	}
	items, err := h.server.labels.SearchLabels(ctx, req.Msg.GetQuery(), req.Msg.GetCategory(), limit)
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
	path, handler := openchainv1connect.NewTracingServiceHandler(&connectTracingHandler{server: s})
	mount(path, handler)
	path, handler = openchainv1connect.NewLookupServiceHandler(&connectLookupHandler{server: s})
	mount(path, handler)
	path, handler = openchainv1connect.NewLabelServiceHandler(&connectLabelHandler{server: s})
	mount(path, handler)
}
