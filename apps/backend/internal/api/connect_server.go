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
	result, err := h.server.tracingEngine.TraceGraph(ctx, address)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.TraceGraphResponse{SeedAddress: result.SeedAddress, Nodes: nodes, Edges: edges, TotalNodes: result.TotalNodes, TotalEdges: result.TotalEdges}), nil
}

func (h *connectTracingHandler) ExpandNode(ctx context.Context, req *connect.Request[pb.ExpandNodeRequest]) (*connect.Response[pb.ExpandNodeResponse], error) {
	address, err := ethereumAddress(req.Msg.GetNodeAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	result, err := h.server.tracingEngine.TraceGraph(ctx, address)
	if err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, err)
	}
	nodes, edges := toGraphProto(result)
	return connect.NewResponse(&pb.ExpandNodeResponse{NewNodes: nodes, NewEdges: edges}), nil
}

func toGraphProto(result *tracing.GraphResult) ([]*pb.GraphNode, []*pb.GraphEdge) {
	nodes := make([]*pb.GraphNode, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodes = append(nodes, &pb.GraphNode{Id: node.ID, Label: node.Label, EntityType: parseEntityType(node.EntityType), IsSeed: node.IsSeed, TotalVolumeWei: node.TotalVolumeWei, InTxCount: node.InTxCount, OutTxCount: node.OutTxCount})
	}
	edges := make([]*pb.GraphEdge, 0, len(result.Edges))
	for _, edge := range result.Edges {
		edges = append(edges, &pb.GraphEdge{Id: edge.ID, Source: edge.Source, Target: edge.Target, ValueWei: edge.ValueWei, ValueFormatted: edge.ValueFormatted, TxCount: edge.TxCount, AssetSymbol: edge.AssetSymbol})
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
		if items := h.server.labels.GetLabels(ctx, address); len(items) > 0 {
			label = items[0].Label
			if strings.EqualFold(items[0].Category, "exchange") {
				entityType = pb.EntityType_ENTITY_TYPE_EXCHANGE
			}
		}
	}
	return connect.NewResponse(&pb.LookupAddressResponse{Summary: &pb.AddressSummary{Address: address, Network: pb.Network_NETWORK_ETHEREUM_SEPOLIA, EntityType: entityType, Label: label, BalanceWei: balance.String(), BalanceFormatted: adapter.FormatWeiToETH(balance), TxCount: txCount}}), nil
}

type connectLabelHandler struct{ server *Server }

func toLabelProto(item labels.LabelItem) *pb.AddressLabel {
	return &pb.AddressLabel{Id: item.ID, Address: item.Address, Category: item.Category, Label: item.Label, Confidence: item.Confidence, EvidenceUrl: item.EvidenceURL, Source: item.Source, CreatedBy: item.CreatedBy, CreatedAt: item.CreatedAt.Unix()}
}

func (h *connectLabelHandler) GetLabels(ctx context.Context, req *connect.Request[pb.GetLabelsRequest]) (*connect.Response[pb.GetLabelsResponse], error) {
	address, err := ethereumAddress(req.Msg.GetAddress())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	items := h.server.labels.GetLabels(ctx, address)
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
	items := h.server.labels.SearchLabels(ctx, req.Msg.GetQuery(), req.Msg.GetCategory(), limit)
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
