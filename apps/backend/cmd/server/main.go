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
	evmClient := adapter.NewEVMClient(cfg.EthereumMainnetRPCURL)
	chainAdapter := adapter.NewEVMChainAdapter(adapter.EtherscanAPIURL, cfg.EtherscanAPIKey, evmClient)

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
	engine := tracing.NewEngine(chainAdapter, database, registry)
	queue := tracing.NewQueue(engine, database, cfg.MaxQueuedTraceJobs)
	workerContext, stopWorker := context.WithCancel(context.Background())
	queue.Start(workerContext)
	server := api.NewServer(evmClient, registry, engine, queue, cfg.WebOrigin, cfg.PublicRequestsPerMinute, cfg.TrustProxy)
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
	queue.Wait()
	fmt.Println("Server stopped cleanly.")
}
