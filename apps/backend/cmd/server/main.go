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
	"github.com/openchain/openchain/apps/backend/internal/cases"
	"github.com/openchain/openchain/apps/backend/internal/config"
	"github.com/openchain/openchain/apps/backend/internal/labels"
	"github.com/openchain/openchain/apps/backend/internal/risk"
	"github.com/openchain/openchain/apps/backend/internal/tracing"
)

func main() {
	cfg := config.LoadConfig()

	log.Printf("Starting OpenChain Backend Server on port %s...", cfg.Port)
	log.Printf("Connecting to EVM Testnet RPC: %s", cfg.EthSepoliaRPCURL)

	evmClient := adapter.NewEVMClient(cfg.EthSepoliaRPCURL)
	labelRegistry := labels.NewRegistry()
	riskEvaluator := risk.NewEvaluator(labelRegistry)
	tracingEngine := tracing.NewEngine(evmClient, labelRegistry, riskEvaluator)
	caseService := cases.NewService()
	wsHub := api.NewHub()

	server := api.NewServer(evmClient, labelRegistry, riskEvaluator, tracingEngine, caseService, wsHub)

	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      server.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	stopChan := make(chan os.Signal, 1)
	signal.Notify(stopChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	log.Printf("OpenChain API server listening at http://localhost:%s", cfg.Port)
	<-stopChan
	log.Println("Shutting down server gracefully...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("Server forced shutdown: %v", err)
	}

	fmt.Println("Server stopped cleanly.")
}
