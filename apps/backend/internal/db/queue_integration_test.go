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
CREATE TABLE %s.acquisition_blobs (response_sha256 TEXT PRIMARY KEY, response_body BYTEA NOT NULL);
CREATE TABLE %s.acquisition_snapshots (id BIGSERIAL PRIMARY KEY, network TEXT NOT NULL, provider TEXT NOT NULL, request_identity TEXT NOT NULL, response_sha256 TEXT NOT NULL REFERENCES %s.acquisition_blobs(response_sha256), retrieved_at TIMESTAMPTZ NOT NULL);
CREATE TABLE %s.acquisition_scopes (id BIGSERIAL PRIMARY KEY, network TEXT NOT NULL, address TEXT NOT NULL, cursor TEXT NOT NULL, retrieved_at TIMESTAMPTZ NOT NULL);
CREATE TABLE %s.acquisition_scope_transfers (scope_id BIGINT NOT NULL REFERENCES %s.acquisition_scopes(id), transfer_id TEXT NOT NULL REFERENCES %s.transfers(id), PRIMARY KEY (scope_id, transfer_id));
CREATE TABLE %s.acquisition_scope_snapshots (scope_id BIGINT NOT NULL REFERENCES %s.acquisition_scopes(id), acquisition_id BIGINT NOT NULL REFERENCES %s.acquisition_snapshots(id), PRIMARY KEY (scope_id, acquisition_id));
CREATE TABLE %s.rule_catalog (LIKE public.rule_catalog INCLUDING ALL);
CREATE TABLE %s.rule_runs (LIKE public.rule_runs INCLUDING ALL);
CREATE TRIGGER acquisition_snapshots_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_blobs_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_blobs FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scopes_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scopes FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scope_transfers_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scope_transfers FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scope_snapshots_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scope_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
SET search_path = %s, public`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)); err != nil {
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
			var snapshots, blobs, links int
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_snapshots`).Scan(&snapshots); err != nil {
				t.Fatal(err)
			}
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_blobs`).Scan(&blobs); err != nil {
				t.Fatal(err)
			}
			if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_scope_transfers WHERE transfer_id = $1`, "ethereum-mainnet:0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa:tx").Scan(&links); err != nil {
				t.Fatal(err)
			}
			// The failed request is retained in its own scope. The three successful
			// provider responses are scoped to their shared page, not each transfer.
			if snapshots != 4 || blobs != 3 || links != 1 {
				t.Fatalf("snapshots = %d, blobs = %d, scope links = %d", snapshots, blobs, links)
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
			if err != nil || len(exported.Transfers) != 3 || len(exported.Snapshots) != 3 || len(exported.Scopes) != 1 || len(exported.ScopeTransfers) != 3 || len(exported.ScopeSnapshots) != 3 || len(exported.RuleRuns) != len(rules.Catalog())-1 {
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

func TestLargeFanDatasetsPersistDeterministicFindings(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test controlled large fan datasets")
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
	database.SQL.SetMaxOpenConns(1)
	database.SQL.SetMaxIdleConns(1)
	schema := "openchain_large_fan_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.rule_runs (LIKE public.rule_runs INCLUDING ALL); SET search_path = %s, public`, schema, schema, schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
	}()

	const network = "controlled-large-fan"
	const hub = "0x00000000000000000000000000000000000000aa"
	now := time.Unix(1_700_000_000, 0).UTC()
	transfers := make([]db.Transfer, 0, 128)
	for index := 0; index < 64; index++ {
		transfers = append(transfers,
			db.Transfer{ID: fmt.Sprintf("in-%03d", index), Network: network, FromAddress: fmt.Sprintf("source-%03d", index), ToAddress: hub, Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, AmountBaseUnits: strconv.Itoa(100 + index), BlockTimestamp: now.Add(time.Duration(index) * time.Minute)},
			db.Transfer{ID: fmt.Sprintf("out-%03d", index), Network: network, FromAddress: hub, ToAddress: fmt.Sprintf("destination-%03d", index), Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, AmountBaseUnits: strconv.Itoa(200 + index), BlockTimestamp: now.Add(48*time.Hour + time.Duration(index)*time.Minute)},
		)
	}
	leads, runs := rules.Evaluate(network, transfers, now.Add(72*time.Hour))
	var fanIn, fanOut *rules.Lead
	for index := range leads {
		switch leads[index].RuleID {
		case "fan-in-consolidation":
			fanIn = &leads[index]
		case "fan-out-dispersion":
			fanOut = &leads[index]
		}
	}
	if fanIn == nil || fanOut == nil || len(fanIn.TransferIDs) != 3 || len(fanOut.TransferIDs) != 3 || len(runs) == 0 || len(runs[0].InputTransferIDs) != len(transfers) {
		t.Fatalf("large fan findings = %#v", leads)
	}
	if err := database.SaveRuleRuns(ctx, runs); err != nil {
		t.Fatal(err)
	}
	var stored int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM rule_runs WHERE network = $1`, network).Scan(&stored); err != nil || stored != len(runs) {
		t.Fatalf("persisted runs = %d want %d err=%v", stored, len(runs), err)
	}
}
