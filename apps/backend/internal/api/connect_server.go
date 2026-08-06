package api

import (
	"context"
	"net/http"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
)

type ConnectTracingHandler struct {
	server *Server
}

func parseEntityType(entityStr string) pb.EntityType {
	switch entityStr {
	case "CONTRACT":
		return pb.EntityType_ENTITY_TYPE_CONTRACT
	case "EXCHANGE":
		return pb.EntityType_ENTITY_TYPE_EXCHANGE
	case "MIXER":
		return pb.EntityType_ENTITY_TYPE_MIXER
	default:
		return pb.EntityType_ENTITY_TYPE_EOA
	}
}

func (h *ConnectTracingHandler) TraceGraph(ctx context.Context, req *connect.Request[pb.TraceGraphRequest]) (*connect.Response[pb.TraceGraphResponse], error) {
	seeds := req.Msg.GetSeedAddresses()
	if len(seeds) == 0 && req.Msg.GetSeedAddress() != "" {
		seeds = []string{req.Msg.GetSeedAddress()}
	}

	gResult, err := h.server.tracingEngine.TraceMultiAddressGraph(
		ctx,
		seeds,
		"ETHEREUM_SEPOLIA",
		req.Msg.GetMaxHops(),
		req.Msg.GetDirection().String(),
		req.Msg.GetTokens(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	nodes := make([]*pb.GraphNode, 0, len(gResult.Nodes))
	for _, n := range gResult.Nodes {
		nodes = append(nodes, &pb.GraphNode{
			Id:             n.ID,
			Label:          n.Label,
			EntityType:     parseEntityType(n.EntityType),
			RiskScore:      n.RiskScore,
			Category:       n.Category,
			IsSeed:         n.IsSeed,
			TotalVolumeWei: n.TotalVolumeWei,
			InTxCount:      n.InTxCount,
			OutTxCount:     n.OutTxCount,
		})
	}

	edges := make([]*pb.GraphEdge, 0, len(gResult.Edges))
	for _, e := range gResult.Edges {
		edges = append(edges, &pb.GraphEdge{
			Id:               e.ID,
			Source:           e.Source,
			Target:           e.Target,
			ValueWei:         e.ValueWei,
			ValueFormatted:   e.ValueFormatted,
			TxCount:          e.TxCount,
			AssetSymbol:      e.AssetSymbol,
			FirstTxTimestamp: e.FirstTxTimestamp,
			LastTxTimestamp:  e.LastTxTimestamp,
		})
	}

	resp := &pb.TraceGraphResponse{
		SeedAddress: gResult.SeedAddress,
		Nodes:       nodes,
		Edges:       edges,
		TotalNodes:  gResult.TotalNodes,
		TotalEdges:  gResult.TotalEdges,
	}

	return connect.NewResponse(resp), nil
}

func (h *ConnectTracingHandler) ExpandNode(ctx context.Context, req *connect.Request[pb.ExpandNodeRequest]) (*connect.Response[pb.ExpandNodeResponse], error) {
	gResult, err := h.server.tracingEngine.TraceMultiAddressGraph(
		ctx,
		[]string{req.Msg.GetNodeAddress()},
		"ETHEREUM_SEPOLIA",
		req.Msg.GetMaxHops(),
		req.Msg.GetDirection().String(),
		nil,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	nodes := make([]*pb.GraphNode, 0, len(gResult.Nodes))
	for _, n := range gResult.Nodes {
		nodes = append(nodes, &pb.GraphNode{
			Id:             n.ID,
			Label:          n.Label,
			EntityType:     parseEntityType(n.EntityType),
			RiskScore:      n.RiskScore,
			Category:       n.Category,
			IsSeed:         n.IsSeed,
			TotalVolumeWei: n.TotalVolumeWei,
			InTxCount:      n.InTxCount,
			OutTxCount:     n.OutTxCount,
		})
	}

	edges := make([]*pb.GraphEdge, 0, len(gResult.Edges))
	for _, e := range gResult.Edges {
		edges = append(edges, &pb.GraphEdge{
			Id:               e.ID,
			Source:           e.Source,
			Target:           e.Target,
			ValueWei:         e.ValueWei,
			ValueFormatted:   e.ValueFormatted,
			TxCount:          e.TxCount,
			AssetSymbol:      e.AssetSymbol,
			FirstTxTimestamp: e.FirstTxTimestamp,
			LastTxTimestamp:  e.LastTxTimestamp,
		})
	}

	return connect.NewResponse(&pb.ExpandNodeResponse{
		NewNodes: nodes,
		NewEdges: edges,
	}), nil
}

func (s *Server) RegisterConnectRPC(mux *http.ServeMux) {
	tracingHandler := &ConnectTracingHandler{server: s}
	path, handler := openchainv1connect.NewTracingServiceHandler(tracingHandler)
	mux.Handle(path, withLogging(handler.ServeHTTP))
}
