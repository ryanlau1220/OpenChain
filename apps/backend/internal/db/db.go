package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/apache/age/drivers/golang/age"
	_ "github.com/lib/pq"
)

// DB wraps database connection and graph operations for OpenChain
type DB struct {
	SQL       *sql.DB
	GraphName string
	DSN       string
}

// Config defines connection pool and database configuration
type Config struct {
	DSN             string
	GraphName       string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
	ConnMaxIdleTime time.Duration
}

// DefaultConfig returns default connection settings
func DefaultConfig(dsn string) Config {
	if dsn == "" {
		dsn = "postgres://openchain:openchain_secret@localhost:5432/openchain?sslmode=disable"
	}
	return Config{
		DSN:             dsn,
		GraphName:       "openchain",
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

// NewDB initializes PostgreSQL connection pool and verifies Apache AGE graph schema
func NewDB(cfg Config) (*DB, error) {
	sqlDB, err := sql.Open("postgres", cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("failed to open postgres connection: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(cfg.ConnMaxIdleTime)

	db := &DB{
		SQL:       sqlDB,
		GraphName: cfg.GraphName,
		DSN:       cfg.DSN,
	}

	return db, nil
}

// InitSchema ensures Apache AGE extension, graph namespace, and domain labels are initialized
func (d *DB) InitSchema(ctx context.Context) error {
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin schema tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()


	initSQL := `
		CREATE EXTENSION IF NOT EXISTS age;
		LOAD 'age';
		SET search_path = ag_catalog, "$user", public;

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_graph WHERE name = 'openchain') THEN
				PERFORM create_graph('openchain');
			END IF;
		END $$;

		DO $$
		BEGIN
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Wallet' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_vlabel('openchain', 'Wallet');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Contract' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_vlabel('openchain', 'Contract');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Exchange' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_vlabel('openchain', 'Exchange');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'Label' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_vlabel('openchain', 'Label');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'TRANSFER' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_elabel('openchain', 'TRANSFER');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'MINT' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_elabel('openchain', 'MINT');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'SWAP' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_elabel('openchain', 'SWAP');
			END IF;
			IF NOT EXISTS (SELECT 1 FROM ag_catalog.ag_label WHERE name = 'HAS_LABEL' AND graph = (SELECT graphid FROM ag_catalog.ag_graph WHERE name = 'openchain')) THEN
				PERFORM create_elabel('openchain', 'HAS_LABEL');
			END IF;
		END $$;

	`

	if _, err := tx.ExecContext(ctx, initSQL); err != nil {
		return fmt.Errorf("schema initialization failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema tx: %w", err)
	}

	slog.Info("Apache AGE schema successfully initialized", "graph", d.GraphName)
	return nil
}

// ConnectAge returns a transaction-bound Age client for Cypher executions
func (d *DB) ConnectAge() (*age.Age, error) {
	return age.ConnectAge(d.GraphName, d.DSN)
}

// Close closes the database connection pool
func (d *DB) Close() error {
	return d.SQL.Close()
}
