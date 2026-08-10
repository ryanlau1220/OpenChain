package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

const testAddress = "0x7a250d5630b4cf539739df2c5dacb4c659f2488d"

func setupTestServer() (http.Handler, *Server) {
	registry := labels.NewService(nil)
	engine := tracing.NewEngine(nil, nil, registry)
	return NewServer(nil, registry, engine, nil, "http://localhost:3000", 30, false).Handler(), NewServer(nil, registry, engine, nil, "http://localhost:3000", 30, false)
}

func TestPublicRequestLimitUsesConnectResourceExhausted(t *testing.T) {
	registry := labels.NewService(nil)
	engine := tracing.NewEngine(nil, nil, registry)
	server := httptest.NewServer(NewServer(nil, registry, engine, nil, "http://localhost:3000", 1, false).Handler())
	defer server.Close()
	client := openchainv1connect.NewTracingServiceClient(server.Client(), server.URL)
	request := connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress})
	if _, err := client.TraceGraph(context.Background(), request); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("first code = %v", connect.CodeOf(err))
	}
	if _, err := client.TraceGraph(context.Background(), request); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("second code = %v", connect.CodeOf(err))
	}
}

func TestHealthAPI(t *testing.T) {
	handler, _ := setupTestServer()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	var body struct {
		Status string `json:"status"`
		Queue  struct {
			Enabled bool `json:"enabled"`
		} `json:"queue"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "healthy" {
		t.Fatalf("status = %q", body.Status)
	}
	if body.Queue.Enabled {
		t.Fatal("test server unexpectedly has a queue")
	}
}

func TestCORSOnlyAllowsConfiguredOrigin(t *testing.T) {
	handler, _ := setupTestServer()
	for _, origin := range []string{"http://localhost:3000", "https://untrusted.example"} {
		request := httptest.NewRequest(http.MethodOptions, "/openchain.v1.LookupService/LookupAddress", nil)
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusNoContent {
			t.Fatalf("status = %d", response.Code)
		}
		if origin == "http://localhost:3000" && response.Header().Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("configured origin not allowed")
		}
		if origin != "http://localhost:3000" && response.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Fatal("untrusted origin allowed")
		}
	}
}

func TestLookupRejectsInvalidAddress(t *testing.T) {
	_, server := setupTestServer()
	_, err := (&connectLookupHandler{server: server}).LookupAddress(context.Background(), connect.NewRequest(&pb.LookupAddressRequest{Address: "invalid"}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}

func TestTraceWithoutDataSourceIsUnavailable(t *testing.T) {
	_, server := setupTestServer()
	response, err := (&connectTracingHandler{server: server}).TraceGraph(context.Background(), connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress}))
	_ = response
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}

func TestGraphProtoCarriesDirectLabelEvidence(t *testing.T) {
	nodes, _ := toGraphProto(&tracing.GraphResult{Nodes: []tracing.GraphNode{{ID: testAddress, Labels: []labels.LabelItem{{ID: "router", Address: testAddress, Network: "ethereum-mainnet", Label: "Uniswap V2 Router", Category: "DeFi", EvidenceURL: "https://example.test/proof", Source: "test source", SourceVersion: "v1", Visibility: "public", TrustTier: 1}}}}})
	if len(nodes) != 1 || len(nodes[0].GetLabels()) != 1 || nodes[0].GetLabels()[0].GetEvidenceUrl() != "https://example.test/proof" || nodes[0].GetLabels()[0].GetVisibility() != pb.LabelVisibility_LABEL_VISIBILITY_PUBLIC {
		t.Fatalf("graph labels = %#v", nodes)
	}
}

func TestCuratedLabelsReachLookupAndLabelAPI(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test curated-label API flow")
	}
	database, err := db.NewDB(db.DefaultConfig(os.Getenv("DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := database.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	service := labels.NewService(database)
	if err := service.ImportSeed(ctx); err != nil {
		t.Fatal(err)
	}
	engine := tracing.NewEngine(nil, database, service)
	server := NewServer(nil, service, engine, nil, "http://localhost:3000", 30, false)
	labelsResponse, err := (&connectLabelHandler{server: server}).GetLabels(ctx, connect.NewRequest(&pb.GetLabelsRequest{Address: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET}))
	if err != nil {
		t.Fatal(err)
	}
	if len(labelsResponse.Msg.Labels) != 1 || labelsResponse.Msg.Labels[0].GetEvidenceUrl() == "" || labelsResponse.Msg.Labels[0].GetSourceVersion() == "" || labelsResponse.Msg.Labels[0].GetVisibility() != pb.LabelVisibility_LABEL_VISIBILITY_PUBLIC {
		t.Fatalf("label response = %#v", labelsResponse.Msg.Labels)
	}
	lookupResponse, err := (&connectLookupHandler{server: server}).LookupAddress(ctx, connect.NewRequest(&pb.LookupAddressRequest{Address: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET}))
	if err != nil {
		t.Fatal(err)
	}
	if lookupResponse.Msg.Summary.GetLabel() != "Uniswap V2 Router" {
		t.Fatalf("lookup label = %q", lookupResponse.Msg.Summary.GetLabel())
	}
}
