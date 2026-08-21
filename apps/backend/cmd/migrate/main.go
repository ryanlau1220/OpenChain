package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/openchain/openchain/apps/backend/internal/db"
)

func main() {
	runtimeDSN := os.Getenv("DATABASE_URL")
	ownerDSN := os.Getenv("MIGRATION_DATABASE_URL")
	if ownerDSN == "" {
		ownerDSN = runtimeDSN
	}
	if ownerDSN == "" {
		log.Fatal("MIGRATION_DATABASE_URL or DATABASE_URL is required")
	}
	database, err := db.NewDB(db.DefaultConfig(ownerDSN))
	if err != nil {
		log.Fatalf("database unavailable: %v", err)
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.ApplyMigrations(ctx); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
	if runtimeDSN != "" {
		if err := database.ProvisionRuntimeRole(ctx, runtimeDSN); err != nil {
			log.Fatalf("provision runtime role: %v", err)
		}
	}
	log.Print("OpenChain database migrations are current")
}
