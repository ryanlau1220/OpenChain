package main

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/api"
	"github.com/openchain/openchain/apps/backend/internal/config"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

// loadTestNetwork keeps temporary queue rows isolated from every real network.
const loadTestNetwork = "openchain-load-test"

type stubChain struct{ delay time.Duration }

func (c *stubChain) Network() string { return loadTestNetwork }

func (c *stubChain) NativeAsset() adapter.Asset {
	return adapter.Asset{Kind: "NATIVE", Symbol: "ETH", Decimals: 18}
}

func (c *stubChain) ActivityLabel() string { return "Outgoing nonce" }

func (c *stubChain) NormalizeAddress(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 42 || !strings.HasPrefix(value, "0x") {
		return "", fmt.Errorf("invalid EVM address")
	}
	if _, ok := new(big.Int).SetString(value[2:], 16); !ok {
		return "", fmt.Errorf("invalid EVM address")
	}
	return value, nil
}

func (c *stubChain) NormalizeTransactionHash(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) != 66 || !strings.HasPrefix(value, "0x") {
		return "", fmt.Errorf("invalid EVM transaction hash")
	}
	if _, ok := new(big.Int).SetString(value[2:], 16); !ok {
		return "", fmt.Errorf("invalid EVM transaction hash")
	}
	return value, nil
}

func (c *stubChain) GetBalance(context.Context, string) (*big.Int, error) {
	return big.NewInt(1_000_000_000_000_000_000), nil
}

func (c *stubChain) GetTxCount(context.Context, string) (uint64, error) { return 1, nil }

func (c *stubChain) IsContract(context.Context, string) (bool, error) { return false, nil }

func (c *stubChain) GetContractMetadata(context.Context, string) (*adapter.ContractMetadata, error) {
	return &adapter.ContractMetadata{}, nil
}

func (c *stubChain) SourceStatus() adapter.SourceStatus {
	return adapter.SourceStatus{Source: "load-test-stub", LatestChainBlock: 100, IsComplete: true}
}

func (c *stubChain) LookupTransaction(ctx context.Context, hash string) (*adapter.TransactionItem, adapter.SourceStatus, error) {
	if _, err := c.NormalizeTransactionHash(hash); err != nil {
		return nil, adapter.SourceStatus{}, err
	}
	return &adapter.TransactionItem{Hash: hash, ValueBaseUnits: "1", AssetSymbol: "ETH", Timestamp: time.Unix(1, 0)}, c.SourceStatus(), nil
}

func (c *stubChain) ListTransfers(ctx context.Context, address string, _ uint32, _ string) (*adapter.TransferPage, error) {
	if err := wait(ctx, c.delay); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	return &adapter.TransferPage{
		Transfers: []adapter.TransferItem{{
			Hash:            "0x" + strings.Repeat("1", 64),
			EventID:         "stub:0",
			TransferKind:    "NATIVE",
			From:            address,
			To:              "0x0000000000000000000000000000000000000001",
			AmountBaseUnits: "1000000000000000",
			Asset:           c.NativeAsset(),
			BlockNumber:     1,
			BlockHash:       "0x" + strings.Repeat("2", 64),
			Timestamp:       now,
		}},
		SourceStatus: adapter.SourceStatus{Source: "load-test-stub", RetrievedAt: now, LatestChainBlock: 100, IsComplete: true},
	}, nil
}

func wait(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func positiveEnv(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func clearLoadTestJobs(ctx context.Context, database *db.DB) error {
	_, err := database.SQL.ExecContext(ctx, `DELETE FROM trace_jobs WHERE network = $1`, loadTestNetwork)
	return err
}

func main() {
	cfg := config.LoadConfig()
	database, err := db.NewDB(db.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		log.Fatal(err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := database.SQL.PingContext(ctx); err != nil {
		cancel()
		log.Fatal(err)
	}
	if err := clearLoadTestJobs(ctx, database); err != nil {
		cancel()
		log.Fatal(err)
	}
	cancel()
	chain := &stubChain{delay: time.Duration(positiveEnv("OPENCHAIN_LOAD_TEST_PROVIDER_DELAY_MS", 250)) * time.Millisecond}
	engine := tracing.NewEngine(chain, nil, nil)
	queue := tracing.NewQueue(engine, database, positiveEnv("OPENCHAIN_LOAD_TEST_MAX_QUEUED_JOBS_PER_NETWORK", 4), positiveEnv("OPENCHAIN_LOAD_TEST_MAX_QUEUED_JOBS_PER_CLIENT_PER_NETWORK", 2))
	workerContext, stopWorker := context.WithCancel(context.Background())
	queue.Start(workerContext)
	server := api.NewServer(map[pb.Network]api.NetworkRuntime{pb.Network_NETWORK_ETHEREUM_MAINNET: {Chain: chain, Engine: engine, Queue: queue}}, nil, cfg.WebOrigin, positiveEnv("OPENCHAIN_LOAD_TEST_REQUESTS_PER_MINUTE", 8), true, "openchain-load-test")
	port := os.Getenv("OPENCHAIN_LOAD_TEST_PORT")
	if port == "" {
		port = "18091"
	}
	httpServer := &http.Server{Addr: "127.0.0.1:" + port, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	signalContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	<-signalContext.Done()
	stop()
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutdown)
	stopWorker()
	queue.Wait()
	if err := clearLoadTestJobs(shutdown, database); err != nil {
		log.Printf("clear load-test queue jobs: %v", err)
	}
}
