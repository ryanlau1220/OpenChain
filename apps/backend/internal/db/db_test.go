package db

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("")
	if config.DSN == "" {
		t.Fatalf("config = %#v", config)
	}
}

func TestSaveGraphIntegration(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test Apache AGE persistence")
	}
	database, err := NewDB(DefaultConfig(os.Getenv("DATABASE_URL")))
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
	schema := "openchain_evidence_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s;
CREATE TABLE %s.transfers (LIKE public.transfers INCLUDING ALL);
CREATE TABLE %s.assets (LIKE public.assets INCLUDING ALL);
CREATE TABLE %s.acquisition_blobs (response_sha256 TEXT PRIMARY KEY, response_body BYTEA NOT NULL);
CREATE TABLE %s.acquisition_snapshots (id BIGSERIAL PRIMARY KEY, network TEXT NOT NULL, provider TEXT NOT NULL, request_identity TEXT NOT NULL, response_sha256 TEXT NOT NULL REFERENCES %s.acquisition_blobs(response_sha256), retrieved_at TIMESTAMPTZ NOT NULL);
CREATE TABLE %s.acquisition_scopes (id BIGSERIAL PRIMARY KEY, network TEXT NOT NULL, address TEXT NOT NULL, cursor TEXT NOT NULL, retrieved_at TIMESTAMPTZ NOT NULL);
CREATE TABLE %s.acquisition_scope_transfers (scope_id BIGINT NOT NULL REFERENCES %s.acquisition_scopes(id), transfer_id TEXT NOT NULL REFERENCES %s.transfers(id), PRIMARY KEY (scope_id, transfer_id));
CREATE TABLE %s.acquisition_scope_snapshots (scope_id BIGINT NOT NULL REFERENCES %s.acquisition_scopes(id), acquisition_id BIGINT NOT NULL REFERENCES %s.acquisition_snapshots(id), PRIMARY KEY (scope_id, acquisition_id));
CREATE TRIGGER acquisition_snapshots_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_blobs_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_blobs FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scopes_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scopes FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scope_transfers_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scope_transfers FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
CREATE TRIGGER acquisition_scope_snapshots_immutable BEFORE UPDATE OR DELETE ON %s.acquisition_scope_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_evidence_mutation();
SET search_path = %s, public`, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema, schema)); err != nil {
		t.Fatal(err)
	}
	from := "0x1000000000000000000000000000000000000001"
	to := "0x2000000000000000000000000000000000000002"
	transfer := Transfer{ID: "ethereum-mainnet:test-graph:tx", Network: "ethereum-mainnet", TransactionHash: "0xtest-graph", EventID: "tx", TransferKind: "NATIVE", FromAddress: from, ToAddress: to, Asset: adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}, AmountBaseUnits: "42", BlockNumber: 1, BlockHash: "0xblock", BlockTimestamp: time.Unix(1, 0), Provisional: false, Source: "test", RetrievedAt: time.Now().UTC()}
	rawResponse := []byte(`{"status":"1","result":["evidence"]}`)
	acquisition := adapter.RawAcquisition{Provider: "test-provider", RequestIdentity: "GET https://provider.test/account?address=test", Response: rawResponse, RetrievedAt: time.Now().UTC()}
	defer func() {
		cleanupGraph(t, database, transfer.ID, transfer.Network, from, to)
		_, _ = database.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
	}()
	scope := AcquisitionScope{Network: transfer.Network, Address: from, Cursor: "page-1", RetrievedAt: acquisition.RetrievedAt}
	if err := database.SaveEvidenceGraph(ctx, scope, []Address{{Network: transfer.Network, Address: from, Label: "From", EntityType: "EOA"}, {Network: transfer.Network, Address: to, Label: "To", EntityType: "EOA"}}, []Transfer{transfer}, []adapter.RawAcquisition{acquisition}); err != nil {
		t.Fatal(err)
	}
	var relationalCount int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM transfers WHERE id = $1`, transfer.ID).Scan(&relationalCount); err != nil {
		t.Fatal(err)
	}
	if relationalCount != 1 || graphFundFlowCount(t, database, transfer.ID) != 1 {
		t.Fatalf("transfer was not persisted to both stores")
	}
	expectedHash := sha256.Sum256(rawResponse)
	var acquisitionID int64
	var responseHash string
	var provisional bool
	if err := database.SQL.QueryRowContext(ctx, `SELECT snapshot.id, snapshot.response_sha256, transfer.provisional FROM acquisition_snapshots snapshot JOIN acquisition_scope_snapshots scope_snapshot ON scope_snapshot.acquisition_id = snapshot.id JOIN acquisition_scope_transfers scope_transfer ON scope_transfer.scope_id = scope_snapshot.scope_id JOIN transfers transfer ON transfer.id = scope_transfer.transfer_id WHERE transfer.id = $1`, transfer.ID).Scan(&acquisitionID, &responseHash, &provisional); err != nil || responseHash != fmt.Sprintf("%x", expectedHash[:]) || provisional {
		t.Fatalf("transfer provenance = id:%d hash:%q provisional:%v err:%v", acquisitionID, responseHash, provisional, err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE acquisition_snapshots SET provider = 'changed' WHERE id = $1`, acquisitionID); err == nil {
		t.Fatal("immutable acquisition snapshot was updated")
	}
	secondScope := AcquisitionScope{Network: transfer.Network, Address: to, Cursor: "page-2", RetrievedAt: acquisition.RetrievedAt.Add(time.Second)}
	if err := database.SaveAcquisitions(ctx, secondScope, []adapter.RawAcquisition{{Provider: acquisition.Provider, RequestIdentity: acquisition.RequestIdentity, Response: rawResponse, RetrievedAt: acquisition.RetrievedAt.Add(time.Second)}}); err != nil {
		t.Fatal(err)
	}
	var blobs, snapshots, scopes int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_blobs`).Scan(&blobs); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_snapshots`).Scan(&snapshots); err != nil {
		t.Fatal(err)
	}
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM acquisition_scopes`).Scan(&scopes); err != nil || blobs != 1 || snapshots != 2 || scopes != 2 {
		t.Fatalf("deduplicated evidence blobs=%d snapshots=%d scopes=%d err=%v", blobs, snapshots, scopes, err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE acquisition_blobs SET response_body = 'changed' WHERE response_sha256 = $1`, responseHash); err == nil {
		t.Fatal("immutable acquisition blob was updated")
	}
	exported, err := database.ExportEvidence(ctx, transfer.Network, []string{transfer.ID})
	if err != nil || len(exported.Transfers) != 1 || len(exported.Snapshots) != 1 || len(exported.Scopes) != 1 || len(exported.ScopeTransfers) != 1 || len(exported.ScopeSnapshots) != 1 || exported.Snapshots[0].Hash != responseHash {
		t.Fatalf("evidence export = %#v err=%v", exported, err)
	}
	var assetCount int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM assets WHERE network = $1 AND contract_address = $2`, transfer.Network, "").Scan(&assetCount); err != nil || assetCount != 1 {
		t.Fatalf("asset was not persisted: count=%d err=%v", assetCount, err)
	}
}

func TestTraceJobIntegration(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test durable trace jobs")
	}
	database, err := NewDB(DefaultConfig(os.Getenv("DATABASE_URL")))
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
	schema := "openchain_jobs_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.trace_jobs (LIKE public.trace_jobs INCLUDING ALL); SET search_path = %s, public`, schema, schema, schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
	}()
	query := TraceJobQuery{Network: "test", Address: "trace-job-test-address", Direction: "both", Limit: 1}

	queued, err := database.EnqueueTraceJob(ctx, query, false, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	baseQuery := query
	baseQuery.Network = "base-mainnet"
	if _, err := database.EnqueueTraceJob(ctx, baseQuery, false, 3, 3); err != nil {
		t.Fatal(err)
	}
	duplicate, err := database.EnqueueTraceJob(ctx, query, false, 3, 3)
	if err != nil {
		t.Fatal(err)
	}
	if queued.ID != duplicate.ID || duplicate.Status != "queued" {
		t.Fatalf("duplicate enqueue = %#v, want existing queued job %#v", duplicate, queued)
	}
	selected := query
	selected.CounterpartyLimit = 5
	selected.Ranking = "most_active"
	distinct, err := database.EnqueueTraceJob(ctx, selected, false, 3, 3)
	if err != nil || distinct.ID == queued.ID {
		t.Fatalf("graph controls must identify a distinct job: %#v, err=%v", distinct, err)
	}

	claimed, err := database.ClaimTraceJob(ctx, query.Network, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if claimed == nil || claimed.ID != queued.ID || claimed.Status != "running" {
		t.Fatalf("claimed = %#v, want running job %#v", claimed, queued)
	}
	if err := database.RequeueTraceJob(ctx, claimed.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err = database.ClaimTraceJob(ctx, query.Network, time.Minute)
	if err != nil || claimed == nil || claimed.ID != queued.ID || claimed.Status != "running" {
		t.Fatalf("reclaimed = %#v, err = %v", claimed, err)
	}
	other, err := database.ClaimTraceJob(ctx, query.Network, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if other != nil {
		t.Fatalf("claimed a job from another network: %#v", other)
	}
	otherNetwork, err := database.ClaimTraceJob(ctx, baseQuery.Network, time.Minute)
	if err != nil || otherNetwork == nil || otherNetwork.ID == queued.ID {
		t.Fatalf("concurrent per-network claim = %#v, err = %v", otherNetwork, err)
	}

	result := []byte(`{"seed_address":"trace-job-test-address","nodes":[],"edges":[]}`)
	if err := database.CompleteTraceJob(ctx, claimed.ID, result); err != nil {
		t.Fatal(err)
	}
	if err := database.CompleteTraceJob(ctx, otherNetwork.ID, result); err != nil {
		t.Fatal(err)
	}
	completed, err := database.EnqueueTraceJob(ctx, query, false, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	var completedResult map[string]any
	if err := json.Unmarshal(completed.Result, &completedResult); err != nil || completed.Status != "succeeded" || completedResult["seed_address"] != query.Address {
		t.Fatalf("completed job = %#v", completed)
	}
	stored, err := database.TraceJob(ctx, query)
	if err != nil || stored.ID != completed.ID || stored.Status != "succeeded" {
		t.Fatalf("stored job = %#v, err = %v", stored, err)
	}
	if _, err := database.SQL.ExecContext(ctx, `UPDATE trace_jobs SET status = 'failed', result_json = NULL WHERE id = $1`, completed.ID); err != nil {
		t.Fatal(err)
	}
	failed, err := database.EnqueueTraceJob(ctx, query, false, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Status != "failed" {
		t.Fatalf("polling a failed job requeued it: %#v", failed)
	}
	retried, err := database.EnqueueTraceJob(ctx, query, true, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if retried.Status != "queued" {
		t.Fatalf("explicit retry = %#v", retried)
	}
	if _, err := database.EnqueueTraceJob(ctx, TraceJobQuery{Network: "test", Address: "other-trace-job", Direction: "both", Limit: 1}, false, 1, 2); !errors.Is(err, ErrTraceQueueFull) {
		t.Fatalf("queue capacity error = %v", err)
	}
}

func TestConcurrentTraceJobsDeduplicateAndRespectCapacity(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test concurrent durable trace jobs")
	}
	root, err := NewDB(DefaultConfig(os.Getenv("DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := root.InitSchema(ctx); err != nil {
		t.Fatal(err)
	}
	schema := "openchain_concurrent_jobs_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if _, err := root.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.trace_jobs (LIKE public.trace_jobs INCLUDING ALL)`, schema, schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = root.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
	}()
	parsed, err := url.Parse(root.DSN)
	if err != nil {
		t.Fatal(err)
	}
	values := parsed.Query()
	values.Set("search_path", schema+",public")
	parsed.RawQuery = values.Encode()
	database, err := NewDB(DefaultConfig(parsed.String()))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	query := TraceJobQuery{Network: "ethereum-mainnet", Address: "concurrent-trace-job", Direction: "both", Limit: 1}
	const callers = 8
	start := make(chan struct{})
	jobs := make(chan *TraceJob, callers)
	errs := make(chan error, callers)
	var workers sync.WaitGroup
	for range callers {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			job, err := database.EnqueueTraceJob(ctx, query, false, callers, callers)
			if err != nil {
				errs <- err
				return
			}
			jobs <- job
		}()
	}
	close(start)
	workers.Wait()
	close(jobs)
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	var jobID int64
	for job := range jobs {
		if jobID == 0 {
			jobID = job.ID
		}
		if job.ID != jobID || job.Status != "queued" {
			t.Fatalf("concurrent enqueue returned %#v, want queued job %d", job, jobID)
		}
	}
	if jobID == 0 {
		t.Fatal("concurrent enqueue returned no job")
	}
	var count int
	if err := database.SQL.QueryRowContext(ctx, `SELECT count(*) FROM trace_jobs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("deduplicated trace jobs = %d err=%v", count, err)
	}
	if _, err := database.SQL.ExecContext(ctx, `DELETE FROM trace_jobs`); err != nil {
		t.Fatal(err)
	}

	start = make(chan struct{})
	errs = make(chan error, 2)
	for _, address := range []string{"capacity-a", "capacity-b"} {
		workers.Add(1)
		go func(address string) {
			defer workers.Done()
			<-start
			_, err := database.EnqueueTraceJob(ctx, TraceJobQuery{Network: query.Network, Address: address, Direction: query.Direction, Limit: query.Limit}, false, 1, 2)
			errs <- err
		}(address)
	}
	close(start)
	workers.Wait()
	close(errs)
	var accepted, rejected int
	for err := range errs {
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrTraceQueueFull) {
			rejected++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || rejected != 1 {
		t.Fatalf("queue capacity accepted=%d rejected=%d", accepted, rejected)
	}
	if _, err := database.SQL.ExecContext(ctx, `DELETE FROM trace_jobs`); err != nil {
		t.Fatal(err)
	}
	for _, address := range []string{"client-capacity-a", "client-capacity-b"} {
		if _, err := database.EnqueueTraceJob(ctx, TraceJobQuery{Network: query.Network, Address: address, Direction: query.Direction, Limit: query.Limit, ClientKey: "client-a"}, false, callers, 2); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := database.EnqueueTraceJob(ctx, TraceJobQuery{Network: query.Network, Address: "client-capacity-c", Direction: query.Direction, Limit: query.Limit, ClientKey: "client-a"}, false, callers, 2); !errors.Is(err, ErrTraceClientQueueFull) {
		t.Fatalf("client queue capacity error = %v", err)
	}
	if _, err := database.EnqueueTraceJob(ctx, TraceJobQuery{Network: query.Network, Address: "client-capacity-c", Direction: query.Direction, Limit: query.Limit, ClientKey: "client-b"}, false, callers, 2); err != nil {
		t.Fatalf("separate client queue budget = %v", err)
	}
}

func graphFundFlowCount(t *testing.T, database *DB, id string) int {
	t.Helper()
	ctx := context.Background()
	tx, err := database.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = ag_catalog, "$user", public`); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(map[string]string{"id": id})
	if err != nil {
		t.Fatal(err)
	}
	var count string
	err = tx.QueryRowContext(ctx, `SELECT * FROM cypher('openchain', $$ MATCH ()-[flow:FundFlow {id: $id}]->() RETURN count(flow) $$, $1) AS (count agtype)`, string(encoded)).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	value, err := strconv.Atoi(count)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func cleanupGraph(t *testing.T, database *DB, flowID, network, from, to string) {
	t.Helper()
	ctx := context.Background()
	tx, err := database.SQL.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "LOAD 'age'"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, `SET LOCAL search_path = ag_catalog, "$user", public`); err != nil {
		t.Fatal(err)
	}
	for _, query := range []struct {
		query  string
		params map[string]string
	}{
		{`SELECT * FROM cypher('openchain', $$ MATCH ()-[flow:FundFlow {id: $id}]->() DELETE flow RETURN count(flow) $$, $1) AS (result agtype)`, map[string]string{"id": flowID}},
		{`SELECT * FROM cypher('openchain', $$ MATCH (address:Address {network: $network, address: $address}) DETACH DELETE address RETURN count(address) $$, $1) AS (result agtype)`, map[string]string{"network": network, "address": from}},
		{`SELECT * FROM cypher('openchain', $$ MATCH (address:Address {network: $network, address: $address}) DETACH DELETE address RETURN count(address) $$, $1) AS (result agtype)`, map[string]string{"network": network, "address": to}},
	} {
		if err := execCypher(ctx, tx, query.query, query.params); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
