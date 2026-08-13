package db_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1/openchainv1connect"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/api"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/rules"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func TestQueueIntegrationTraceFindingAndEvidenceExport(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test the trace worker")
	}
	const (
		from = "0x1000000000000000000000000000000000000001"
		toA  = "0x2000000000000000000000000000000000000002"
		toB  = "0x3000000000000000000000000000000000000003"
		toC  = "0x4000000000000000000000000000000000000004"
	)
	var txListAttempts atomic.Int32
	etherscan := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("module") != "account" {
			http.NotFound(writer, request)
			return
		}
		if request.URL.Query().Get("address") != from {
			t.Errorf("address = %q", request.URL.Query().Get("address"))
		}
		if request.URL.Query().Get("action") != "txlist" {
			_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[]}`))
			return
		}
		if txListAttempts.Add(1) == 1 {
			writer.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = writer.Write([]byte(`{"status":"1","message":"OK","result":[` +
			`{"hash":"0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","blockNumber":"10","timeStamp":"100","from":"` + from + `","to":"` + toA + `","value":"42"},` +
			`{"hash":"0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","blockNumber":"11","timeStamp":"3700","from":"` + from + `","to":"` + toB + `","value":"43"},` +
			`{"hash":"0xcccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","blockNumber":"12","timeStamp":"7300","from":"` + from + `","to":"` + toC + `","value":"44"}]}`))
	}))
	defer etherscan.Close()

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
	database.SQL.SetMaxOpenConns(1)
	database.SQL.SetMaxIdleConns(1)
	schema := "openchain_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s;
CREATE TABLE %s.trace_jobs (LIKE public.trace_jobs INCLUDING ALL);
CREATE TABLE %s.transfers (LIKE public.transfers INCLUDING ALL);
CREATE TABLE %s.assets (LIKE public.assets INCLUDING ALL);
CREATE TABLE %s.acquisition_snapshots (LIKE public.acquisition_snapshots INCLUDING ALL);
CREATE TABLE %s.transfer_acquisitions (transfer_id TEXT NOT NULL REFERENCES %s.transfers(id), acquisition_id BIGINT NOT NULL REFERENCES %s.acquisition_snapshots(id), PRIMARY KEY (transfer_id, acquisition_id));
CREATE TABLE %s.rule_catalog (LIKE public.rule_catalog INCLUDING ALL);
CREATE TABLE %s.rule_runs (LIKE public.rule_runs INCLUDING ALL);
CREATE TRIGGER acquisition_snapshots_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
SET search_path = %s, public`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
	}()
	if err := database.ImportRuleCatalog(ctx, rules.CatalogEntries()); err != nil {
		t.Fatal(err)
	}

	chain := adapter.NewEVMChainAdapter("ethereum-mainnet", "1", etherscan.URL, "test-key", nil)
	registry := labels.NewService(database)
	engine := tracing.NewEngine(chain, database, registry)
	queue := tracing.NewQueue(engine, database, 2, 1)
	workerContext, stopWorker := context.WithCancel(context.Background())
	queue.Start(workerContext)
	defer func() {
		stopWorker()
		queue.Wait()
	}()
	server := httptest.NewServer(api.NewServer(map[pb.Network]api.NetworkRuntime{
		pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine, Queue: queue},
	}, registry, "http://localhost:3000", 100, false, "test-key").Handler())
	defer server.Close()
	tracingClient := openchainv1connect.NewTracingServiceClient(server.Client(), server.URL)
	evidenceClient := openchainv1connect.NewEvidenceServiceClient(server.Client(), server.URL)

	// The Etherscan adapter divides a requested graph page among native,
	// internal, and token sources. Seven yields three native observations.
	pending, err := tracingClient.TraceGraph(ctx, connect.NewRequest(&pb.TraceGraphRequest{SeedAddress: from, Network: pb.Network_NETWORK_ETHEREUM_MAINNET, Limit: 7, Retry: true}))
	if err != nil {
		t.Fatal(err)
	}
	if !pending.Msg.GetPending() || len(pending.Msg.GetNodes()) != 1 {
		t.Fatalf("pending trace = %#v", pending.Msg)
	}

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		result, err := tracingClient.GetTraceStatus(ctx, connect.NewRequest(&pb.TraceStatusRequest{Address: from, Network: pb.Network_NETWORK_ETHEREUM_MAINNET, Limit: 7}))
		if err != nil {
			t.Fatal(err)
		}
		if !result.Msg.GetPending() {
			if len(result.Msg.GetNodes()) != 4 || len(result.Msg.GetEdges()) != 3 || txListAttempts.Load() != 2 {
				t.Fatalf("completed trace = %#v", result.Msg)
			}
			if len(result.Msg.GetLeads()) != 1 || result.Msg.GetLeads()[0].GetRuleId() != "fan-out-dispersion" {
				t.Fatalf("deterministic finding = %#v", result.Msg.GetLeads())
			}
			var snapshots, links int
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_snapshots`).Scan(&snapshots); err != nil {
				t.Fatal(err)
			}
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM transfer_acquisitions WHERE transfer_id = $1`, "ethereum-mainnet:0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:tx").Scan(&links); err != nil {
				t.Fatal(err)
			}
			// The failed first provider request is preserved separately from the
			// three successful source snapshots linked to each transfer.
			if snapshots != 4 || links != 3 {
				t.Fatalf("snapshots = %d, links = %d", snapshots, links)
			}
			var ruleRuns int
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM rule_runs WHERE network = $1`, "ethereum-mainnet").Scan(&ruleRuns); err != nil || ruleRuns != len(rules.Catalog())-1 {
				t.Fatalf("rule runs = %d err = %v", ruleRuns, err)
			}
			var version string
			var parameters, inputIDs, ruleResult []byte
			var startedAt, completedAt time.Time
			if err := database.SQL.QueryRowContext(ctx, `SELECT rule_version, parameters, input_transfer_ids, result, started_at, completed_at FROM rule_runs WHERE rule_id = $1`, "fan-out-dispersion").Scan(&version, &parameters, &inputIDs, &ruleResult, &startedAt, &completedAt); err != nil || version != "1.0.0" || !json.Valid(parameters) || !json.Valid(inputIDs) || !json.Valid(ruleResult) || startedAt.IsZero() || completedAt.IsZero() {
				t.Fatalf("rule run provenance version=%q parameters=%q inputs=%q result=%q started=%v completed=%v err=%v", version, parameters, inputIDs, ruleResult, startedAt, completedAt, err)
			}
			transferIDs := make([]string, 0, len(result.Msg.GetEdges()))
			for _, edge := range result.Msg.GetEdges() {
				transferIDs = append(transferIDs, edge.GetId())
			}
			exported, err := database.ExportEvidence(ctx, "ethereum-mainnet", transferIDs)
			if err != nil || len(exported.Transfers) != 3 || len(exported.Snapshots) != 3 || len(exported.Provenance) != 9 || len(exported.RuleRuns) != len(rules.Catalog())-1 {
				t.Fatalf("evidence export = %#v err=%v", exported, err)
			}
			packageResponse, err := evidenceClient.ExportEvidencePackage(ctx, connect.NewRequest(&pb.ExportEvidencePackageRequest{Network: pb.Network_NETWORK_ETHEREUM_MAINNET, TransferIds: transferIDs, CaseJson: `{"version":1,"title":"Integration case"}`}))
			if err != nil || !json.Valid([]byte(packageResponse.Msg.GetPackageJson())) {
				t.Fatalf("frozen package err=%v package=%s", err, packageResponse.Msg.GetPackageJson())
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("trace job was not completed")
}
