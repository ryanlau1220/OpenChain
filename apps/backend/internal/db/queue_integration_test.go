package db_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func TestQueueIntegrationReturnsCompletedTrace(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test the trace worker")
	}
	const (
		from = "0x1000000000000000000000000000000000000001"
		to   = "0x2000000000000000000000000000000000000002"
		hash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	)
	trueBlocks := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/export":
			if request.URL.Query().Get("addrs") != from {
				t.Errorf("address = %q", request.URL.Query().Get("addrs"))
			}
			_, _ = writer.Write([]byte(`{"data":[{"hash":"` + hash + `","blockNumber":10,"timestamp":100,"from":"` + from + `","to":"` + to + `","value":"42"}]}`))
		case "/status":
			_, _ = writer.Write([]byte(`{"data":[{}]}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer trueBlocks.Close()

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
	if _, err := database.SQL.ExecContext(ctx, fmt.Sprintf(`CREATE SCHEMA %s; CREATE TABLE %s.trace_jobs (LIKE public.trace_jobs INCLUDING ALL); SET search_path = %s, public`, schema, schema, schema)); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = database.SQL.ExecContext(context.Background(), `DELETE FROM trace_jobs WHERE network = 'ethereum-mainnet' AND address = $1`, from)
		_, _ = database.SQL.ExecContext(context.Background(), fmt.Sprintf(`DROP SCHEMA %s CASCADE`, schema))
		_, _ = database.SQL.ExecContext(context.Background(), `DELETE FROM transfers WHERE id = $1`, "ethereum-mainnet:"+hash+":0")
	}()

	client, err := adapter.NewTrueBlocksClient(trueBlocks.URL)
	if err != nil {
		t.Fatal(err)
	}
	queue := tracing.NewQueue(tracing.NewEngine(nil, client, database, labels.NewService(database)), database)
	workerContext, stopWorker := context.WithCancel(context.Background())
	queue.Start(workerContext)
	defer func() {
		stopWorker()
		queue.Wait()
	}()

	pending, err := queue.TraceGraph(ctx, from, tracing.DirectionBoth, 1, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !pending.Pending || len(pending.Nodes) != 1 {
		t.Fatalf("pending trace = %#v", pending)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		result, err := queue.TraceGraph(ctx, from, tracing.DirectionBoth, 1, "", false)
		if err != nil {
			t.Fatal(err)
		}
		if !result.Pending {
			if len(result.Nodes) != 2 || len(result.Edges) != 1 || result.Edges[0].TransactionHash != hash {
				t.Fatalf("completed trace = %#v", result)
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("trace job was not completed")
}
