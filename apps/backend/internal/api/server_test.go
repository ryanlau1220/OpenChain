package api

import (
	"context"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
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
const testTransactionHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func setupTestServer() (http.Handler, *Server) {
	registry := labels.NewService(nil)
	server := NewServer(testNetworks(registry), registry, "http://localhost:3000", 30, false, "test-key")
	return server.Handler(), server
}

func testNetworks(registry *labels.Service) map[pb.Network]NetworkRuntime {
	chain := adapter.NewEVMChainAdapter("ethereum-mainnet", "1", "https://api.example", "test-key", nil)
	engine := tracing.NewEngine(chain, nil, registry)
	return map[pb.Network]NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine}}
}

type transactionLookupChain struct{}

func (transactionLookupChain) Network() string { return "ethereum-mainnet" }
func (transactionLookupChain) Capabilities() adapter.NetworkCapabilities {
	return adapter.NetworkCapabilities{NativeTransfers: true, HistoricalPagination: true, Finality: true, EntityClassification: true, ExactRawProvenance: true}
}
func (transactionLookupChain) NormalizeAddress(value string) (string, error) { return value, nil }
func (transactionLookupChain) NormalizeTransactionHash(value string) (string, error) {
	return value, nil
}
func (transactionLookupChain) NativeAsset() adapter.Asset {
	return adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}
func (transactionLookupChain) ActivityLabel() string { return "Outgoing nonce" }
func (transactionLookupChain) GetBalance(context.Context, string) (*big.Int, error) {
	return big.NewInt(0), nil
}
func (transactionLookupChain) GetTxCount(context.Context, string) (uint64, error) { return 0, nil }
func (transactionLookupChain) IsContract(context.Context, string) (bool, error)   { return false, nil }
func (transactionLookupChain) ListTransfers(context.Context, string, uint32, string) (*adapter.TransferPage, error) {
	return nil, nil
}
func (transactionLookupChain) LookupTransaction(context.Context, string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	return &adapter.TransactionItem{Hash: testTransactionHash, From: testAddress, To: testAddress, ValueBaseUnits: "0", BlockNumber: 1}, adapter.SourceStatus{Source: "test"}, nil
}
func (transactionLookupChain) GetContractMetadata(context.Context, string) (*adapter.ContractMetadata, error) {
	return &adapter.ContractMetadata{Category: "EOA"}, nil
}
func (transactionLookupChain) SourceStatus() adapter.SourceStatus {
	return adapter.SourceStatus{Source: "test"}
}

func TestPublicRequestLimitUsesConnectResourceExhausted(t *testing.T) {
	registry := labels.NewService(nil)
	server := httptest.NewServer(NewServer(testNetworks(registry), registry, "http://localhost:3000", 1, false, "test-key").Handler())
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

func TestPublicRequestLimitSharesIPBudgetDuringConcurrentBurst(t *testing.T) {
	registry := labels.NewService(nil)
	server := httptest.NewServer(NewServer(testNetworks(registry), registry, "http://localhost:3000", 2, true, "test-key").Handler())
	defer server.Close()
	client := openchainv1connect.NewTracingServiceClient(server.Client(), server.URL)
	const callers = 8
	codes := make(chan connect.Code, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			request := connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: "invalid", Network: pb.Network_NETWORK_ETHEREUM_MAINNET})
			request.Header().Set("X-Forwarded-For", "198.51.100.7")
			_, err := client.TraceGraph(context.Background(), request)
			codes <- connect.CodeOf(err)
		}()
	}
	workers.Wait()
	close(codes)
	var allowed, limited int
	for code := range codes {
		switch code {
		case connect.CodeInvalidArgument:
			allowed++
		case connect.CodeResourceExhausted:
			limited++
		default:
			t.Fatalf("concurrent request code = %v", code)
		}
	}
	if allowed != 2 || limited != callers-2 {
		t.Fatalf("shared IP budget allowed=%d limited=%d", allowed, limited)
	}
	request := connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: "invalid", Network: pb.Network_NETWORK_ETHEREUM_MAINNET})
	request.Header().Set("X-Forwarded-For", "198.51.100.8")
	if _, err := client.TraceGraph(context.Background(), request); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("separate IP code = %v", connect.CodeOf(err))
	}
}

func TestTraceStatusPollingDoesNotConsumePublicRequestBudget(t *testing.T) {
	registry := labels.NewService(nil)
	server := httptest.NewServer(NewServer(testNetworks(registry), registry, "http://localhost:3000", 1, false, "test-key").Handler())
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
			Network      string                      `json:"network"`
			Capabilities adapter.NetworkCapabilities `json:"capabilities"`
			Providers    []struct {
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
	if len(body.Networks) != 1 || body.Networks[0].Network != "ethereum-mainnet" || !body.Networks[0].Capabilities.NativeTransfers || !body.Networks[0].Capabilities.TokenTransfers || !body.Networks[0].Capabilities.InternalTransfers || !body.Networks[0].Capabilities.HistoricalPagination || !body.Networks[0].Capabilities.Finality || !body.Networks[0].Capabilities.EntityClassification || !body.Networks[0].Capabilities.ExactRawProvenance || !body.Networks[0].Capabilities.BridgeEvidence || body.Networks[0].Capabilities.TransactionSuccess || len(body.Networks[0].Providers) != 1 || body.Networks[0].Providers[0].MaxConcurrent != 1 || body.Networks[0].Providers[0].RequestsPerSecond != 5 {
		t.Fatalf("network health = %#v", body.Networks)
	}
}

func TestHealthAlertsExposeQueueAndProviderThresholds(t *testing.T) {
	registry := labels.NewService(nil)
	runtimes := testNetworks(registry)
	runtime := runtimes[pb.Network_NETWORK_ETHEREUM_MAINNET]
	runtime.Queue = tracing.NewQueue(runtime.Engine, nil, 10, 1)
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
	if len(alerts) != 3 || alerts[0].Code != "trace_queue_full" || alerts[0].Severity != "critical" || alerts[0].Network != "ethereum-mainnet" || alerts[1].Code != "trace_jobs_failed" || alerts[2].Code != "provider_unhealthy" {
		t.Fatalf("alerts = %#v", alerts)
	}
}

func TestHealthAlertsUsePerNetworkQueueCapacity(t *testing.T) {
	registry := labels.NewService(nil)
	runtimes := testNetworks(registry)
	ethereum := runtimes[pb.Network_NETWORK_ETHEREUM_MAINNET]
	ethereum.Queue = tracing.NewQueue(ethereum.Engine, nil, 10, 1)
	runtimes[pb.Network_NETWORK_ETHEREUM_MAINNET] = ethereum
	base := runtimes[pb.Network_NETWORK_BASE_MAINNET]
	base.Queue = tracing.NewQueue(base.Engine, nil, 10, 1)
	runtimes[pb.Network_NETWORK_BASE_MAINNET] = base
	alerts := healthAlerts([]healthNetwork{
		{Network: "ethereum-mainnet", Queue: tracing.Stats{Enabled: true, Queued: 10}},
		{Network: "base-mainnet", Queue: tracing.Stats{Enabled: true, Queued: 5}},
	}, runtimes)
	if len(alerts) != 1 || alerts[0].Code != "trace_queue_full" || alerts[0].Network != "ethereum-mainnet" {
		t.Fatalf("per-network queue alerts = %#v", alerts)
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

func TestLookupMarksUnavailableFieldsInsteadOfReturningZeroFacts(t *testing.T) {
	_, server := setupTestServer()
	response, err := (&connectLookupHandler{server: server}).LookupAddress(context.Background(), connect.NewRequest(&pb.LookupAddressRequest{Address: testAddress, Network: pb.Network_NETWORK_ETHEREUM_MAINNET}))
	if err != nil {
		t.Fatal(err)
	}
	summary := response.Msg.GetSummary()
	if summary.GetBalanceBaseUnits() != "" || summary.GetBalanceFormatted() != "" || summary.GetEntityType() != pb.EntityType_ENTITY_TYPE_UNSPECIFIED {
		t.Fatalf("unavailable fields became facts: %#v", summary)
	}
	statuses := map[pb.LookupField]*pb.LookupFieldStatus{}
	for _, status := range response.Msg.GetFieldStatuses() {
		statuses[status.GetField()] = status
	}
	for _, field := range []pb.LookupField{pb.LookupField_LOOKUP_FIELD_BALANCE, pb.LookupField_LOOKUP_FIELD_ACTIVITY, pb.LookupField_LOOKUP_FIELD_ENTITY_TYPE} {
		status := statuses[field]
		if status == nil || status.GetAvailable() || status.GetWarning() == "" {
			t.Fatalf("field %v status = %#v", field, status)
		}
	}
}

func TestLookupTransactionMarksExecutionStatusUnknown(t *testing.T) {
	chain := transactionLookupChain{}
	registry := labels.NewService(nil)
	server := NewServer(map[pb.Network]NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: tracing.NewEngine(chain, nil, registry)}}, registry, "http://localhost:3000", 30, false, "test-key")
	response, err := (&connectLookupHandler{server: server}).LookupTransaction(context.Background(), connect.NewRequest(&pb.LookupTransactionRequest{Hash: testTransactionHash, Network: pb.Network_NETWORK_ETHEREUM_MAINNET}))
	if err != nil {
		t.Fatal(err)
	}
	if response.Msg.GetTransaction().GetStatus() != pb.TransactionStatus_TRANSACTION_STATUS_UNKNOWN {
		t.Fatalf("transaction status = %v", response.Msg.GetTransaction().GetStatus())
	}
	statuses := response.Msg.GetFieldStatuses()
	if len(statuses) != 1 || statuses[0].GetField() != pb.LookupField_LOOKUP_FIELD_TRANSACTION_STATUS || statuses[0].GetAvailable() || statuses[0].GetWarning() == "" {
		t.Fatalf("transaction field statuses = %#v", statuses)
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
	nodes, _ := toGraphProto(&tracing.GraphResult{Nodes: []tracing.GraphNode{{ID: testAddress, Labels: []labels.LabelItem{{ID: "router", Address: testAddress, Network: "ethereum-mainnet", Label: "Uniswap V2 Router", Category: labels.CategoryDeFiService, EvidenceURL: "https://example.test/proof", Source: "test source", SourceVersion: "v1", Visibility: "public", TrustTier: 1}}}}})
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

func TestGraphProtoCarriesRetrievedScopeCoverage(t *testing.T) {
	coverage := toCoverageProto(tracing.TraceCoverage{RequestedPageSize: 50, ObservedTransferCount: 50, GraphTransferCount: 10, ConfirmationBackedTransferCount: 8, ProvisionalTransferCount: 2, Cursor: "page-2", HasMore: true, ProviderComplete: false, Limitation: "retrieved provider page only"})
	if coverage.GetRequestedPageSize() != 50 || coverage.GetObservedTransferCount() != 50 || coverage.GetGraphTransferCount() != 10 || coverage.GetConfirmationBackedTransferCount() != 8 || coverage.GetProvisionalTransferCount() != 2 || !coverage.GetHasMore() || coverage.GetProviderComplete() || coverage.GetCursor() != "page-2" || coverage.GetLimitation() == "" {
		t.Fatalf("coverage = %#v", coverage)
	}
}

func TestGraphProtoCarriesBlockHash(t *testing.T) {
	_, edges := toGraphProto(&tracing.GraphResult{Edges: []tracing.GraphEdge{{ID: "block", BlockHash: "0xblock"}}})
	if len(edges) != 1 || edges[0].GetBlockHash() != "0xblock" {
		t.Fatalf("edge block hash = %#v", edges)
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
	server := NewServer(map[pb.Network]NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine}}, service, "http://localhost:3000", 30, false, "test-key")
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
