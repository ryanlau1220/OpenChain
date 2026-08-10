package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pb "github.com/openchain/openchain/apps/backend/gen/proto/openchain/v1"
	"github.com/openchain/openchain/apps/backend/internal/adapter"
	"github.com/openchain/openchain/apps/backend/internal/api"
	"github.com/openchain/openchain/apps/backend/internal/config"
	"github.com/openchain/openchain/apps/backend/internal/db"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func main() {
	cfg := config.LoadConfig()
	if err := cfg.Validate(); err != nil {
		log.Fatal(err)
	}
	log.Printf("Starting OpenChain backend on port %s", cfg.Port)
	database, err := db.NewDB(db.DefaultConfig(cfg.DatabaseURL))
	if err != nil {
		log.Fatalf("Database unavailable: %v", err)
	}
	defer func(connection *db.DB) { _ = connection.Close() }(database)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	err = database.InitSchema(ctx)
	cancel()
	if err != nil {
		_ = database.Close()
		log.Fatalf("Database schema unavailable: %v", err)
	}

	registry := labels.NewService(database)
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	err = registry.ImportSeed(ctx)
	cancel()
	if err != nil {
		log.Fatalf("Curated label import failed: %v", err)
	}
	runtimes := make(map[pb.Network]api.NetworkRuntime, 4)
	queues := make([]*tracing.Queue, 0, 4)
	for _, network := range []struct {
		id                    pb.Network
		name, chainID, rpcURL string
	}{
		{pb.Network_NETWORK_ETHEREUM_MAINNET, "ethereum-mainnet", "1", cfg.EthereumMainnetRPCURL},
		{pb.Network_NETWORK_BASE_MAINNET, "base-mainnet", "8453", cfg.BaseMainnetRPCURL},
	} {
		evmClient := adapter.NewEVMClient(network.rpcURL)
		var chainAdapter adapter.ChainAdapter
		if network.id == pb.Network_NETWORK_BASE_MAINNET {
			chainAdapter = adapter.NewBlockscoutChainAdapter(network.name, adapter.BlockscoutBaseAPIURL, cfg.BlockscoutAPIKey, evmClient)
		} else {
			chainAdapter = adapter.NewEVMChainAdapter(network.name, network.chainID, adapter.EtherscanAPIURL, cfg.EtherscanAPIKey, evmClient)
		}
		engine := tracing.NewEngine(chainAdapter, database, registry)
		queue := tracing.NewQueue(engine, database, cfg.MaxQueuedTraceJobs)
		runtimes[network.id] = api.NetworkRuntime{Chain: chainAdapter, Engine: engine, Queue: queue}
		queues = append(queues, queue)
	}
	for _, network := range []struct {
		id   pb.Network
		name string
	}{
		{pb.Network_NETWORK_SOLANA_MAINNET, "solana-mainnet"},
		{pb.Network_NETWORK_TRON_MAINNET, "tron-mainnet"},
	} {
		var chainAdapter adapter.ChainAdapter
		if network.id == pb.Network_NETWORK_SOLANA_MAINNET {
			chainAdapter = adapter.NewSolanaAdapter(network.name, cfg.SolanaMainnetRPCURL)
		} else {
			chainAdapter = adapter.NewTronAdapter(network.name, adapter.TronGridAPIURL, cfg.TronGridAPIKey)
		}
		engine := tracing.NewEngine(chainAdapter, database, registry)
		queue := tracing.NewQueue(engine, database, cfg.MaxQueuedTraceJobs)
		runtimes[network.id] = api.NetworkRuntime{Chain: chainAdapter, Engine: engine, Queue: queue}
		queues = append(queues, queue)
	}
	workerContext, stopWorker := context.WithCancel(context.Background())
	for _, queue := range queues {
		queue.Start(workerContext)
	}
	server := api.NewServer(runtimes, registry, cfg.WebOrigin, cfg.PublicRequestsPerMinute, cfg.TrustProxy)
	httpServer := &http.Server{Addr: ":" + cfg.Port, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()
	log.Printf("OpenChain API server listening at http://localhost:%s", cfg.Port)
	<-stop
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}
	stopWorker()
	for _, queue := range queues {
		queue.Wait()
	}
	fmt.Println("Server stopped cleanly.")
}
