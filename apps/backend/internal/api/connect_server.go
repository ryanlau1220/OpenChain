package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"

	"github.com/google/uuid"
)

// ── Tracing Service ────────────────────────────────────────────────────────────

type connectTracingHandler struct{ server *Server }

func parseEntityType(s string) pb.EntityType {
	switch s {
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

func (h *connectTracingHandler) TraceGraph(ctx context.Context, req *connect.Request[pb.TraceGraphRequest]) (*connect.Response[pb.TraceGraphResponse], error) {
	seeds := req.Msg.GetSeedAddresses()
	if len(seeds) == 0 && req.Msg.GetSeedAddress() != "" {
		seeds = []string{req.Msg.GetSeedAddress()}
	}

	gResult, err := h.server.tracingEngine.TraceMultiAddressGraph(
		ctx, seeds, "ETHEREUM_SEPOLIA",
		req.Msg.GetMaxHops(), req.Msg.GetDirection().String(), req.Msg.GetTokens(),
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	nodes := make([]*pb.GraphNode, 0, len(gResult.Nodes))
	for _, n := range gResult.Nodes {
		nodes = append(nodes, &pb.GraphNode{
			Id: n.ID, Label: n.Label, EntityType: parseEntityType(n.EntityType),
			RiskScore: n.RiskScore, Category: n.Category, IsSeed: n.IsSeed,
			TotalVolumeWei: n.TotalVolumeWei, InTxCount: n.InTxCount, OutTxCount: n.OutTxCount,
		})
	}
	edges := make([]*pb.GraphEdge, 0, len(gResult.Edges))
	for _, e := range gResult.Edges {
		edges = append(edges, &pb.GraphEdge{
			Id: e.ID, Source: e.Source, Target: e.Target, ValueWei: e.ValueWei,
			ValueFormatted: e.ValueFormatted, TxCount: e.TxCount, AssetSymbol: e.AssetSymbol,
			FirstTxTimestamp: e.FirstTxTimestamp, LastTxTimestamp: e.LastTxTimestamp,
		})
	}
	return connect.NewResponse(&pb.TraceGraphResponse{
		SeedAddress: gResult.SeedAddress, Nodes: nodes, Edges: edges,
		TotalNodes: gResult.TotalNodes, TotalEdges: gResult.TotalEdges,
	}), nil
}

func (h *connectTracingHandler) ExpandNode(ctx context.Context, req *connect.Request[pb.ExpandNodeRequest]) (*connect.Response[pb.ExpandNodeResponse], error) {
	gResult, err := h.server.tracingEngine.TraceMultiAddressGraph(
		ctx, []string{req.Msg.GetNodeAddress()}, "ETHEREUM_SEPOLIA",
		req.Msg.GetMaxHops(), req.Msg.GetDirection().String(), nil,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	nodes := make([]*pb.GraphNode, 0, len(gResult.Nodes))
	for _, n := range gResult.Nodes {
		nodes = append(nodes, &pb.GraphNode{
			Id: n.ID, Label: n.Label, EntityType: parseEntityType(n.EntityType),
			RiskScore: n.RiskScore, Category: n.Category, IsSeed: n.IsSeed,
			TotalVolumeWei: n.TotalVolumeWei, InTxCount: n.InTxCount, OutTxCount: n.OutTxCount,
		})
	}
	edges := make([]*pb.GraphEdge, 0, len(gResult.Edges))
	for _, e := range gResult.Edges {
		edges = append(edges, &pb.GraphEdge{
			Id: e.ID, Source: e.Source, Target: e.Target, ValueWei: e.ValueWei,
			ValueFormatted: e.ValueFormatted, TxCount: e.TxCount, AssetSymbol: e.AssetSymbol,
			FirstTxTimestamp: e.FirstTxTimestamp, LastTxTimestamp: e.LastTxTimestamp,
		})
	}
	return connect.NewResponse(&pb.ExpandNodeResponse{NewNodes: nodes, NewEdges: edges}), nil
}

func (h *connectTracingHandler) StreamGraphUpdates(
	ctx context.Context,
	req *connect.Request[pb.StreamGraphUpdatesRequest],
	stream *connect.ServerStream[pb.StreamGraphUpdatesResponse],
) error {
	caseID := req.Msg.GetCaseId()
	if caseID == "" {
		caseID = "default-case"
	}

	ch, unsub := h.server.pubSub.Subscribe(caseID)
	defer unsub()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case evt, ok := <-ch:
			if !ok {
				return nil
			}
			res := &pb.StreamGraphUpdatesResponse{
				CaseId:    evt.CaseID,
				EventType: evt.EventType,
			}
			if err := stream.Send(res); err != nil {
				return err
			}
		}
	}
}


// ── Lookup Service ─────────────────────────────────────────────────────────────

type connectLookupHandler struct{ server *Server }

func (h *connectLookupHandler) LookupAddress(ctx context.Context, req *connect.Request[pb.LookupAddressRequest]) (*connect.Response[pb.LookupAddressResponse], error) {
	address := req.Msg.GetAddress()
	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}

	bal, _ := h.server.evm.GetBalance(ctx, address)
	txCount, _ := h.server.evm.GetTxCount(ctx, address)
	isContract, _ := h.server.evm.IsContract(ctx, address)

	entityType := pb.EntityType_ENTITY_TYPE_EOA
	if isContract {
		entityType = pb.EntityType_ENTITY_TYPE_CONTRACT
	}

	lbls := h.server.labels.GetLabels(ctx, address)
	labelStr := shortAddress(address)
	if len(lbls) > 0 {
		labelStr = lbls[0].Label
		lblLower := strings.ToLower(lbls[0].Label)
		catLower := strings.ToLower(lbls[0].Category)
		if catLower == "defi" || catLower == "contract" || strings.Contains(lblLower, "router") || strings.Contains(lblLower, "vault") || strings.Contains(lblLower, "contract") {
			entityType = pb.EntityType_ENTITY_TYPE_CONTRACT
		} else if catLower == "exchange" || strings.Contains(lblLower, "binance") || strings.Contains(lblLower, "exchange") {
			entityType = pb.EntityType_ENTITY_TYPE_EXCHANGE
		}
	}

	if txCount == 0 {
		if ca := h.server.tracingEngine.ChainAdapter(); ca != nil {
			txs, err := ca.GetAccountTransactions(ctx, address, 15)
			if err == nil && len(txs) > 0 {
				txCount = uint64(len(txs))
			}
		}
	}

	summary := &pb.AddressSummary{
		Address:          address,
		Network:          pb.Network_NETWORK_ETHEREUM_SEPOLIA,
		EntityType:       entityType,
		Label:            labelStr,
		BalanceWei:       bal.String(),
		BalanceFormatted: adapter.FormatWeiToETH(bal),
		TxCount:          txCount,
	}

	return connect.NewResponse(&pb.LookupAddressResponse{Summary: summary}), nil
}

func (h *connectLookupHandler) LookupTransaction(_ context.Context, req *connect.Request[pb.LookupTxRequest]) (*connect.Response[pb.LookupTxResponse], error) {
	if req.Msg.GetHash() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("hash is required"))
	}
	return connect.NewResponse(&pb.LookupTxResponse{}), nil
}

// ── Label Service ──────────────────────────────────────────────────────────────

type connectLabelHandler struct{ server *Server }

func toLabelPb(l labels.LabelItem) *pb.AddressLabel {
	return &pb.AddressLabel{
		Id: l.ID, Address: l.Address, Category: l.Category, Label: l.Label,
		Confidence: l.Confidence, EvidenceUrl: l.EvidenceURL, Source: l.Source,
		CreatedBy: l.CreatedBy, CreatedAt: l.CreatedAt.Unix(),
	}
}

func (h *connectLabelHandler) AddLabel(ctx context.Context, req *connect.Request[pb.AddLabelRequest]) (*connect.Response[pb.AddLabelResponse], error) {
	item, err := h.server.labels.AddLabel(ctx, labels.LabelItem{
		Address:     req.Msg.GetAddress(),
		Network:     req.Msg.GetNetwork().String(),
		Category:    req.Msg.GetCategory(),
		Label:       req.Msg.GetLabel(),
		Confidence:  req.Msg.GetConfidence(),
		EvidenceURL: req.Msg.GetEvidenceUrl(),
		Source:      req.Msg.GetSource(),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.AddLabelResponse{Label: toLabelPb(item)}), nil
}

func (h *connectLabelHandler) GetLabels(ctx context.Context, req *connect.Request[pb.GetLabelsRequest]) (*connect.Response[pb.GetLabelsResponse], error) {
	lbls := h.server.labels.GetLabels(ctx, req.Msg.GetAddress())
	out := make([]*pb.AddressLabel, 0, len(lbls))
	for _, l := range lbls {
		out = append(out, toLabelPb(l))
	}
	return connect.NewResponse(&pb.GetLabelsResponse{Labels: out}), nil
}

func (h *connectLabelHandler) SearchLabels(ctx context.Context, req *connect.Request[pb.SearchLabelsRequest]) (*connect.Response[pb.SearchLabelsResponse], error) {
	lbls := h.server.labels.SearchLabels(ctx, req.Msg.GetQuery(), req.Msg.GetCategory(), int(req.Msg.GetLimit()))
	out := make([]*pb.AddressLabel, 0, len(lbls))
	for _, l := range lbls {
		out = append(out, toLabelPb(l))
	}
	return connect.NewResponse(&pb.SearchLabelsResponse{Labels: out}), nil
}

// ── Risk Service ───────────────────────────────────────────────────────────────

type connectRiskHandler struct{ server *Server }

func (h *connectRiskHandler) EvaluateRisk(ctx context.Context, req *connect.Request[pb.EvaluateRiskRequest]) (*connect.Response[pb.EvaluateRiskResponse], error) {
	address := req.Msg.GetAddress()
	if address == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("address is required"))
	}
	isContract, _ := h.server.evm.IsContract(ctx, address)
	txCount, _ := h.server.evm.GetTxCount(ctx, address)
	eval := h.server.riskEvaluator.EvaluateAddress(ctx, address, "ETHEREUM_SEPOLIA", isContract, txCount)

	flags := make([]*pb.RiskFlag, 0, len(eval.Flags))
	for _, f := range eval.Flags {
		flags = append(flags, &pb.RiskFlag{
			RuleId: f.RuleID, RuleName: f.RuleName, Severity: f.Severity,
			ScoreImpact: f.ScoreImpact, Description: f.Description, EvidenceDetail: f.EvidenceDetail,
		})
	}
	return connect.NewResponse(&pb.EvaluateRiskResponse{
		Evaluation: &pb.RiskEvaluation{
			Address: eval.Address, TotalScore: eval.TotalScore, RiskLevel: eval.RiskLevel,
			Flags: flags, EvaluatedAt: eval.EvaluatedAt.Unix(),
		},
	}), nil
}

// ── Case Service ───────────────────────────────────────────────────────────────

type connectCaseHandler struct{ server *Server }

func toPbCase(c *cases.InvestigationCase) *pb.InvestigationCase {
	nodes := make([]*pb.CaseNodeItem, 0, len(c.Nodes))
	for _, n := range c.Nodes {
		nodes = append(nodes, &pb.CaseNodeItem{Address: n.Address, Label: n.Label, Notes: n.Notes})
	}
	edges := make([]*pb.CaseEdgeItem, 0, len(c.Edges))
	for _, e := range c.Edges {
		edges = append(edges, &pb.CaseEdgeItem{
			SourceAddress: e.SourceAddress, TargetAddress: e.TargetAddress,
			TxHash: e.TxHash, Notes: e.Notes,
		})
	}
	return &pb.InvestigationCase{
		Id: c.ID, Title: c.Title, Description: c.Description, Status: c.Status,
		Tags: c.Tags, Nodes: nodes, Edges: edges,
		CreatedBy: c.CreatedBy, CreatedAt: c.CreatedAt.Unix(), UpdatedAt: c.UpdatedAt.Unix(),
	}
}

func (h *connectCaseHandler) CreateCase(ctx context.Context, req *connect.Request[pb.CreateCaseRequest]) (*connect.Response[pb.CreateCaseResponse], error) {
	c, err := h.server.caseService.CreateCase(ctx, req.Msg.GetTitle(), req.Msg.GetDescription(), req.Msg.GetTags())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&pb.CreateCaseResponse{CaseItem: toPbCase(c)}), nil
}

func (h *connectCaseHandler) GetCase(ctx context.Context, req *connect.Request[pb.GetCaseRequest]) (*connect.Response[pb.GetCaseResponse], error) {
	c, err := h.server.caseService.GetCase(ctx, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&pb.GetCaseResponse{CaseItem: toPbCase(c)}), nil
}

func (h *connectCaseHandler) ListCases(ctx context.Context, _ *connect.Request[pb.ListCasesRequest]) (*connect.Response[pb.ListCasesResponse], error) {
	list := h.server.caseService.ListCases(ctx)
	out := make([]*pb.InvestigationCase, 0, len(list))
	for i := range list {
		out = append(out, toPbCase(&list[i]))
	}
	return connect.NewResponse(&pb.ListCasesResponse{Cases: out, Total: uint32(len(out))}), nil
}

func (h *connectCaseHandler) ExportReport(ctx context.Context, req *connect.Request[pb.ExportReportRequest]) (*connect.Response[pb.ExportReportResponse], error) {
	filename, content, mime, err := h.server.caseService.ExportReport(ctx, req.Msg.GetCaseId(), req.Msg.GetFormat())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&pb.ExportReportResponse{Filename: filename, Content: content, MimeType: mime}), nil
}

// ── Canvas Service ─────────────────────────────────────────────────────────────

type canvasShareEntry struct {
	snapshot  *pb.CanvasSnapshot
	expiresAt time.Time
}

type connectCanvasHandler struct {
	mu    sync.RWMutex
	store map[string]canvasShareEntry
}

func newCanvasHandler() *connectCanvasHandler {
	return &connectCanvasHandler{store: make(map[string]canvasShareEntry)}
}

func (h *connectCanvasHandler) ShareCanvas(_ context.Context, req *connect.Request[pb.ShareCanvasRequest]) (*connect.Response[pb.ShareCanvasResponse], error) {
	if req.Msg.Snapshot == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("snapshot is required"))
	}
	shareID := fmt.Sprintf("share-%s", uuid.New().String()[:8])
	expiresAt := time.Now().Add(7 * 24 * time.Hour)

	h.mu.Lock()
	h.store[shareID] = canvasShareEntry{snapshot: req.Msg.Snapshot, expiresAt: expiresAt}
	h.mu.Unlock()

	return connect.NewResponse(&pb.ShareCanvasResponse{
		ShareId:   shareID,
		ExpiresAt: expiresAt.Format(time.RFC3339),
	}), nil
}

func (h *connectCanvasHandler) GetSharedCanvas(_ context.Context, req *connect.Request[pb.GetSharedCanvasRequest]) (*connect.Response[pb.GetSharedCanvasResponse], error) {
	shareID := req.Msg.GetShareId()
	if shareID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("share_id is required"))
	}

	h.mu.RLock()
	entry, ok := h.store[shareID]
	h.mu.RUnlock()

	if !ok || time.Now().After(entry.expiresAt) {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("share link invalid or expired"))
	}

	return connect.NewResponse(&pb.GetSharedCanvasResponse{
		Snapshot:  entry.snapshot,
		ShareId:   shareID,
		ExpiresAt: entry.expiresAt.Format(time.RFC3339),
	}), nil
}

// ── Registration ───────────────────────────────────────────────────────────────

var canvasHandler = newCanvasHandler() // singleton — survives per-request handler re-creation

func (s *Server) RegisterConnectRPC(mux *http.ServeMux) {
	mount := func(path string, handler http.Handler) {
		mux.Handle(path, withLogging(handler.ServeHTTP))
	}
	p, h := openchainv1connect.NewTracingServiceHandler(&connectTracingHandler{s})
	mount(p, h)
	p, h = openchainv1connect.NewLookupServiceHandler(&connectLookupHandler{s})
	mount(p, h)
	p, h = openchainv1connect.NewLabelServiceHandler(&connectLabelHandler{s})
	mount(p, h)
	p, h = openchainv1connect.NewRiskServiceHandler(&connectRiskHandler{s})
	mount(p, h)
	p, h = openchainv1connect.NewCaseServiceHandler(&connectCaseHandler{s})
	mount(p, h)
	p, h = openchainv1connect.NewCanvasServiceHandler(canvasHandler)
	mount(p, h)
}
