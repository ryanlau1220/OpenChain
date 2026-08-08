package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func setupTestServer() (http.Handler, *Server) {
	evmClient := adapter.NewEVMClient("https://ethereum-sepolia-rpc.publicnode.com")
	labelRegistry := labels.NewRegistry()
	riskEvaluator := risk.NewEvaluator(labelRegistry, nil)

	tracingEngine := tracing.NewEngine(evmClient, nil, nil, labelRegistry, riskEvaluator)

	caseService := cases.NewService()
	wsHub := NewHub()

	server := NewServer(evmClient, labelRegistry, riskEvaluator, tracingEngine, caseService, wsHub)
	return server.Handler(), server
}

// ── REST: Health ───────────────────────────────────────────────────────────────

func TestHealthAPI(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("GET", "/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse health response: %v", err)
	}
	if resp["status"] != "healthy" {
		t.Errorf("expected healthy status, got %s", resp["status"])
	}
}

func TestCORSPreflight(t *testing.T) {
	mux, _ := setupTestServer()

	req := httptest.NewRequest("OPTIONS", "/openchain.v1.CanvasService/ShareCanvas", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for preflight, got %d", w.Code)
	}
	if w.Header().Get("Access-Control-Allow-Origin") == "" {
		t.Error("expected Access-Control-Allow-Origin header on preflight")
	}
	if w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("expected Access-Control-Allow-Headers header on preflight")
	}
}

// ── ConnectRPC: Tracing Service ────────────────────────────────────────────────

func TestConnectTracingService(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectTracingHandler{server: server}

	resp, err := handler.TraceGraph(context.Background(), connect.NewRequest(&pb.TraceGraphRequest{
		SeedAddresses: []string{"0x7a250d5630b4cf539739df2c5dacb4c659f2488d"},
		MaxHops:       2,
		Direction:     pb.TraceDirection_TRACE_DIRECTION_BOTH,
	}))
	if err != nil {
		t.Fatalf("TraceGraph failed: %v", err)
	}
	if resp.Msg.SeedAddress == "" {
		t.Error("expected non-empty seed_address in TraceGraph response")
	}
	if len(resp.Msg.Nodes) == 0 {
		t.Error("expected at least one node in TraceGraph response")
	}
	// Verify seed node is present and marked
	seedFound := false
	for _, n := range resp.Msg.Nodes {
		if n.IsSeed {
			seedFound = true
			break
		}
	}
	if !seedFound {
		t.Error("expected at least one seed node marked IsSeed=true")
	}
}

func TestConnectExpandNode(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectTracingHandler{server: server}

	resp, err := handler.ExpandNode(context.Background(), connect.NewRequest(&pb.ExpandNodeRequest{
		NodeAddress: "0x7a250d5630b4cf539739df2c5dacb4c659f2488d",
		MaxHops:     1,
		Direction:   pb.TraceDirection_TRACE_DIRECTION_BOTH,
	}))
	if err != nil {
		t.Fatalf("ExpandNode failed: %v", err)
	}
	_ = resp.Msg.NewNodes // just ensure the call succeeds
}

// ── ConnectRPC: Lookup Service ─────────────────────────────────────────────────

func TestConnectLookupAddress(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectLookupHandler{server: server}

	resp, err := handler.LookupAddress(context.Background(), connect.NewRequest(&pb.LookupAddressRequest{
		Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
		Network: pb.Network_NETWORK_ETHEREUM_SEPOLIA,
	}))
	if err != nil {
		t.Fatalf("LookupAddress failed: %v", err)
	}
	if resp.Msg.Summary == nil {
		t.Fatal("expected summary in LookupAddress response")
	}
	if resp.Msg.Summary.Address == "" {
		t.Error("expected address in summary")
	}
}

func TestConnectLookupAddressEmpty(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectLookupHandler{server: server}

	_, err := handler.LookupAddress(context.Background(), connect.NewRequest(&pb.LookupAddressRequest{}))
	if err == nil {
		t.Fatal("expected error for empty address, got nil")
	}
}

// ── ConnectRPC: Label Service ──────────────────────────────────────────────────

func TestConnectLabelService(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectLabelHandler{server: server}

	// GetLabels for known seeded address
	gResp, err := handler.GetLabels(context.Background(), connect.NewRequest(&pb.GetLabelsRequest{
		Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
	}))
	if err != nil {
		t.Fatalf("GetLabels failed: %v", err)
	}
	if len(gResp.Msg.Labels) == 0 {
		t.Error("expected at least one label for Uniswap V2 Router address")
	}

	// AddLabel
	aResp, err := handler.AddLabel(context.Background(), connect.NewRequest(&pb.AddLabelRequest{
		Address:  "0x1111111111111111111111111111111111111111",
		Network:  pb.Network_NETWORK_ETHEREUM_SEPOLIA,
		Category: "DeFi",
		Label:    "Test Protocol",
		Source:   "Manual",
	}))
	if err != nil {
		t.Fatalf("AddLabel failed: %v", err)
	}
	if aResp.Msg.Label == nil || aResp.Msg.Label.Label != "Test Protocol" {
		t.Error("expected label with correct name")
	}
}

// ── ConnectRPC: Risk Service ───────────────────────────────────────────────────

func TestConnectRiskService(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectRiskHandler{server: server}

	resp, err := handler.EvaluateRisk(context.Background(), connect.NewRequest(&pb.EvaluateRiskRequest{
		Address: "0x7a250d5630B4cF539739dF2C5dAcb4c659F2488D",
		Network: pb.Network_NETWORK_ETHEREUM_SEPOLIA,
	}))
	if err != nil {
		t.Fatalf("EvaluateRisk failed: %v", err)
	}
	if resp.Msg.Evaluation == nil {
		t.Fatal("expected evaluation in EvaluateRisk response")
	}
	if resp.Msg.Evaluation.RiskLevel == "" {
		t.Error("expected non-empty risk_level")
	}
}

// ── ConnectRPC: Case Service ───────────────────────────────────────────────────

func TestConnectCaseService(t *testing.T) {
	_, server := setupTestServer()
	handler := &connectCaseHandler{server: server}

	// ListCases — seeded default exists
	listResp, err := handler.ListCases(context.Background(), connect.NewRequest(&pb.ListCasesRequest{}))
	if err != nil {
		t.Fatalf("ListCases failed: %v", err)
	}
	if listResp.Msg.Total == 0 {
		t.Error("expected at least one seeded case")
	}

	// CreateCase
	createResp, err := handler.CreateCase(context.Background(), connect.NewRequest(&pb.CreateCaseRequest{
		Title:       "ConnectRPC Test Case",
		Description: "Created via ConnectRPC handler",
		Tags:        []string{"test", "rpc"},
	}))
	if err != nil {
		t.Fatalf("CreateCase failed: %v", err)
	}
	caseID := createResp.Msg.CaseItem.Id

	// GetCase
	getResp, err := handler.GetCase(context.Background(), connect.NewRequest(&pb.GetCaseRequest{Id: caseID}))
	if err != nil {
		t.Fatalf("GetCase failed: %v", err)
	}
	if getResp.Msg.CaseItem.Title != "ConnectRPC Test Case" {
		t.Errorf("expected title 'ConnectRPC Test Case', got %s", getResp.Msg.CaseItem.Title)
	}

	// ExportReport
	expResp, err := handler.ExportReport(context.Background(), connect.NewRequest(&pb.ExportReportRequest{
		CaseId: caseID,
		Format: "JSON",
	}))
	if err != nil {
		t.Fatalf("ExportReport failed: %v", err)
	}
	if len(expResp.Msg.Content) == 0 {
		t.Error("expected non-empty export content")
	}
}

// ── ConnectRPC: Canvas Service ─────────────────────────────────────────────────

func TestConnectCanvasService(t *testing.T) {
	handler := newCanvasHandler()

	share, err := handler.ShareCanvas(context.Background(), connect.NewRequest(&pb.ShareCanvasRequest{
		Snapshot: &pb.CanvasSnapshot{
			SeedAddress:   "0xabc",
			SeedAddresses: []string{"0xabc"},
			TotalNodes:    3,
			TotalEdges:    2,
		},
	}))
	if err != nil {
		t.Fatalf("ShareCanvas failed: %v", err)
	}
	if share.Msg.ShareId == "" {
		t.Fatal("expected non-empty share_id")
	}
	if share.Msg.ExpiresAt == "" {
		t.Fatal("expected expires_at")
	}

	// Round-trip: fetch the shared canvas back by id
	got, err := handler.GetSharedCanvas(context.Background(), connect.NewRequest(&pb.GetSharedCanvasRequest{
		ShareId: share.Msg.ShareId,
	}))
	if err != nil {
		t.Fatalf("GetSharedCanvas failed: %v", err)
	}
	if got.Msg.Snapshot == nil || got.Msg.Snapshot.SeedAddress != "0xabc" {
		t.Error("expected snapshot round-trip to preserve seed_address")
	}

	_, err = handler.ShareCanvas(context.Background(), connect.NewRequest(&pb.ShareCanvasRequest{}))
	if err == nil {
		t.Error("expected InvalidArgument for nil snapshot")
	}
	_, err = handler.GetSharedCanvas(context.Background(), connect.NewRequest(&pb.GetSharedCanvasRequest{ShareId: "nope"}))
	if err == nil {
		t.Error("expected NotFound for unknown share_id")
	}
}
