package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DB wraps database connection and graph operations for OpenChain
type DB struct {
	SQL *sql.DB
	DSN string
}

// Config defines connection pool and database configuration
type Config struct {
	DSN             string
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
		MaxOpenConns:    25,
		MaxIdleConns:    10,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
	}
}

// NewDB initializes a PostgreSQL connection pool.
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
		SQL: sqlDB,
		DSN: cfg.DSN,
	}

	return db, nil
}

type migration struct {
	version string
	sql     string
}

// ApplyMigrations applies each embedded migration exactly once. Only the
// migration process should call this; the runtime API account has no DDL rights.
func (d *DB) ApplyMigrations(ctx context.Context) error {
	if err := d.requireAgePreload(ctx); err != nil {
		return err
	}
	migrations, err := embeddedMigrations()
	if err != nil {
		return err
	}
	tx, err := d.SQL.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin schema tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(739201)"); err != nil {
		return fmt.Errorf("lock migrations: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
	)`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	applied := make(map[string]struct{}, len(migrations))
	rows, err := tx.QueryContext(ctx, "SELECT version FROM public.schema_migrations")
	if err != nil {
		return fmt.Errorf("read migration ledger: %w", err)
	}
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			rows.Close()
			return fmt.Errorf("scan migration ledger: %w", err)
		}
		applied[version] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close migration ledger: %w", err)
	}
	for _, migration := range migrations {
		if _, found := applied[migration.version]; found {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", migration.version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO public.schema_migrations (version) VALUES ($1)", migration.version); err != nil {
			return fmt.Errorf("record migration %s: %w", migration.version, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit schema tx: %w", err)
	}

	slog.Info("database migrations are current", "graph", graphName)
	return nil
}

func embeddedMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		contents, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: strings.TrimSuffix(entry.Name(), ".sql"), sql: string(contents)})
	}
	sort.Slice(migrations, func(left, right int) bool { return migrations[left].version < migrations[right].version })
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no database migrations embedded")
	}
	return migrations, nil
}

func (d *DB) requireAgePreload(ctx context.Context) error {
	var libraries string
	if err := d.SQL.QueryRowContext(ctx, "SHOW shared_preload_libraries").Scan(&libraries); err != nil {
		return fmt.Errorf("read Apache AGE preload setting: %w", err)
	}
	for _, library := range strings.Split(libraries, ",") {
		if strings.TrimSpace(library) == "age" {
			return nil
		}
	}
	return fmt.Errorf("Apache AGE must be in shared_preload_libraries")
}

// ProvisionRuntimeRole gives the application connection only the permissions
// needed to read and write OpenChain data. Its DSN must differ from the owner
// DSN in production; using one account remains allowed for local development.
func (d *DB) ProvisionRuntimeRole(ctx context.Context, runtimeDSN string) error {
	if runtimeDSN == "" {
		return fmt.Errorf("runtime database URL is required")
	}
	runtimeURL, err := url.Parse(runtimeDSN)
	if err != nil || runtimeURL.User == nil || runtimeURL.User.Username() == "" {
		return fmt.Errorf("parse runtime database URL")
	}
	role := runtimeURL.User.Username()
	password, hasPassword := runtimeURL.User.Password()
	if !hasPassword || password == "" {
		return fmt.Errorf("runtime database URL requires a password")
	}
	ownerURL, err := url.Parse(d.DSN)
	if err == nil && ownerURL.User != nil && ownerURL.User.Username() == role {
		return nil
	}
	var exists bool
	if err := d.SQL.QueryRowContext(ctx, "SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)", role).Scan(&exists); err != nil {
		return fmt.Errorf("check runtime role: %w", err)
	}
	action := "CREATE ROLE"
	if exists {
		action = "ALTER ROLE"
	}
	var statement string
	if err := d.SQL.QueryRowContext(ctx, `SELECT format($1::text || ' %I LOGIN PASSWORD %L NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT', $2::text, $3::text)`, action, role, password).Scan(&statement); err != nil {
		return fmt.Errorf("prepare runtime role: %w", err)
	}
	if _, err := d.SQL.ExecContext(ctx, statement); err != nil {
		return fmt.Errorf("configure runtime role: %w", err)
	}
	if err := d.grantRuntimeRole(ctx, role, runtimeURL.Path); err != nil {
		return err
	}
	return nil
}

func (d *DB) grantRuntimeRole(ctx context.Context, role, databasePath string) error {
	databaseName := strings.TrimPrefix(databasePath, "/")
	if databaseName == "" {
		return fmt.Errorf("runtime database URL requires a database name")
	}
	statements := []string{
		"GRANT CONNECT ON DATABASE %I TO %I",
		"GRANT USAGE ON SCHEMA public, ag_catalog TO %I",
		"GRANT USAGE, CREATE ON SCHEMA openchain TO %I",
		"GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public, openchain TO %I",
		"GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public, openchain TO %I",
		"GRANT SELECT ON ag_catalog.ag_graph TO %I",
		"GRANT EXECUTE ON ALL FUNCTIONS IN SCHEMA ag_catalog TO %I",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO %I",
		"ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO %I",
	}
	for index, template := range statements {
		arguments := []any{template, role}
		if index == 0 {
			arguments = []any{template, databaseName, role}
		}
		var statement string
		if err := d.SQL.QueryRowContext(ctx, "SELECT format("+placeholders(len(arguments))+")", arguments...).Scan(&statement); err != nil {
			return fmt.Errorf("prepare runtime grant: %w", err)
		}
		if _, err := d.SQL.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("grant runtime role: %w", err)
		}
	}
	return nil
}

func placeholders(count int) string {
	values := make([]string, count)
	for index := range values {
		values[index] = fmt.Sprintf("$%d::text", index+1)
	}
	return strings.Join(values, ", ")
}

// Close closes the database connection pool
func (d *DB) Close() error {
	return d.SQL.Close()
}
