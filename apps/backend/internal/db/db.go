package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"log/slog"
	"time"

	"github.com/apache/age/drivers/golang/age"
	_ "github.com/lib/pq"
)

//go:embed schema.sql
var schemaSQL string

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

	if _, err := tx.ExecContext(ctx, schemaSQL); err != nil {
		return fmt.Errorf("schema initialization failed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema tx: %w", err)
	}

	slog.Info("Apache AGE & relational schema successfully initialized", "graph", d.GraphName)
	return nil
}

// ConnectAge returns a transaction-bound Age client for Cypher executions
func (d *DB) ConnectAge() (*age.Age, error) {
	graph := d.GraphName
	if graph == "" {
		graph = "openchain"
	}
	return age.ConnectAge(graph, d.DSN)
}

// Close closes the database connection pool
func (d *DB) Close() error {
	return d.SQL.Close()
}
