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
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/rules"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

const testAddress = "0x7a250d5630b4cf539739df2c5dacb4c659f2488d"

func setupTestServer() (http.Handler, *Server) {
	registry := labels.NewService(nil)
	server := NewServer(testNetworks(registry), registry, "http://localhost:3000", 30, false)
	return server.Handler(), server
}

func testNetworks(registry *labels.Service) map[pb.Network]NetworkRuntime {
	chain := adapter.NewEVMChainAdapter("ethereum-mainnet", "1", "https://api.example", "test-key", nil)
	engine := tracing.NewEngine(chain, nil, registry)
	return map[pb.Network]NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine}}
}

func TestPublicRequestLimitUsesConnectResourceExhausted(t *testing.T) {
	registry := labels.NewService(nil)
	server := httptest.NewServer(NewServer(testNetworks(registry), registry, "http://localhost:3000", 1, false).Handler())
	defer server.Close()
	client := openchainv1connect.NewTracingServiceClient(server.Client(), server.URL)
	request := connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET})
	if _, err := client.TraceGraph(context.Background(), request); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("first code = %v", connect.CodeOf(err))
	}
	if _, err := client.TraceGraph(context.Background(), request); connect.CodeOf(err) != connect.CodeResourceExhausted {
		t.Fatalf("second code = %v", connect.CodeOf(err))
	}
}

func TestTraceStatusPollingDoesNotConsumePublicRequestBudget(t *testing.T) {
	registry := labels.NewService(nil)
	server := httptest.NewServer(NewServer(testNetworks(registry), registry, "http://localhost:3000", 1, false).Handler())
	defer server.Close()
	client := openchainv1connect.NewTracingServiceClient(server.Client(), server.URL)
	request := connect.NewRequest(&pb.TraceStatusRequest{Address: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET, Limit: 25})
	for range 3 {
		if _, err := client.GetTraceStatus(context.Background(), request); connect.CodeOf(err) != connect.CodeNotFound {
			t.Fatalf("status polling code = %v", connect.CodeOf(err))
		}
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
		Networks []struct {
			Network   string `json:"network"`
			Providers []struct {
				MaxConcurrent     int `json:"max_concurrent"`
				RequestsPerSecond int `json:"requests_per_second"`
			} `json:"providers"`
		} `json:"networks"`
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
	if len(body.Networks) != 1 || body.Networks[0].Network != "ethereum-mainnet" || len(body.Networks[0].Providers) != 1 || body.Networks[0].Providers[0].MaxConcurrent != 1 || body.Networks[0].Providers[0].RequestsPerSecond != 5 {
		t.Fatalf("network health = %#v", body.Networks)
	}
}

func TestHealthAlertsExposeQueueAndProviderThresholds(t *testing.T) {
	registry := labels.NewService(nil)
	runtimes := testNetworks(registry)
	runtime := runtimes[pb.Network_NETWORK_ETHEREUM_MAINNET]
	runtime.Queue = tracing.NewQueue(runtime.Engine, nil, 10)
	runtimes[pb.Network_NETWORK_ETHEREUM_MAINNET] = runtime
	failedAt := time.Now().UTC()
	alerts := healthAlerts([]healthNetwork{{
		Network: "ethereum-mainnet",
		Queue:   tracing.Stats{Enabled: true, Queued: 10, Failed: 1},
		Providers: []adapter.ProviderHealth{{
			Provider:      "test-provider",
			LastFailureAt: &failedAt,
		}},
	}}, runtimes)
	if len(alerts) != 3 || alerts[0].Code != "trace_queue_full" || alerts[0].Severity != "critical" || alerts[1].Code != "trace_jobs_failed" || alerts[2].Code != "provider_unhealthy" {
		t.Fatalf("alerts = %#v", alerts)
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
	response, err := (&connectTracingHandler{server: server}).TraceGraph(context.Background(), connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET}))
	_ = response
	if connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}

func TestTraceRejectsUnspecifiedNetwork(t *testing.T) {
	_, server := setupTestServer()
	_, err := (&connectTracingHandler{server: server}).TraceGraph(context.Background(), connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: testAddress}))
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("code = %v", connect.CodeOf(err))
	}
}

func TestGraphProtoCarriesDirectLabelEvidence(t *testing.T) {
	nodes, _ := toGraphProto(&tracing.GraphResult{Nodes: []tracing.GraphNode{{ID: testAddress, Labels: []labels.LabelItem{{ID: "router", Address: testAddress, Network: "ethereum-mainnet", Label: "Uniswap V2 Router", Category: "DeFi", EvidenceURL: "https://example.test/proof", Source: "test source", SourceVersion: "v1", Visibility: "public", TrustTier: 1}}}}})
	if len(nodes) != 1 || len(nodes[0].GetLabels()) != 1 || nodes[0].GetLabels()[0].GetEvidenceUrl() != "https://example.test/proof" || nodes[0].GetLabels()[0].GetVisibility() != pb.LabelVisibility_LABEL_VISIBILITY_PUBLIC {
		t.Fatalf("graph labels = %#v", nodes)
	}
}

func TestTraceProtoCarriesNeutralInvestigationLead(t *testing.T) {
	lead := rules.Lead{ID: "fan-in-consolidation:test", RuleID: "fan-in-consolidation", RuleVersion: "1.0.0", Title: "Fan-in / consolidation lead", SubjectAddress: testAddress, TransferIDs: []string{"transfer-1"}, Rationale: "Observed transfers.", Limitations: "This is an investigative lead.", Parameters: []byte(`{"window_seconds":86400}`)}
	converted := toLeadProto([]rules.Lead{lead})
	if len(converted) != 1 || converted[0].GetRuleVersion() != "1.0.0" || converted[0].GetParametersJson() != `{"window_seconds":86400}` || converted[0].GetLimitations() == "" {
		t.Fatalf("lead proto = %#v", converted)
	}
}

func TestGraphProtoCarriesFinalityState(t *testing.T) {
	_, edges := toGraphProto(&tracing.GraphResult{Edges: []tracing.GraphEdge{{ID: "provisional", Provisional: true}}})
	if len(edges) != 1 || !edges[0].GetProvisional() {
		t.Fatalf("graph finality = %#v", edges)
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
	chain := adapter.NewEVMChainAdapter("ethereum-mainnet", "1", "https://api.example", "test-key", nil)
	engine := tracing.NewEngine(chain, database, service)
	server := NewServer(map[pb.Network]NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine}}, service, "http://localhost:3000", 30, false)
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
