package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedMigrationsAreOrdered(t *testing.T) {
	migrations, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for index := 1; index < len(migrations); index++ {
		if migrations[index-1].version >= migrations[index].version {
			t.Fatalf("migrations are not ordered: %#v", migrations)
		}
	}
	for _, migration := range migrations {
		if len(migration.checksum) != 64 {
			t.Fatalf("migration %s has invalid checksum %q", migration.version, migration.checksum)
		}
	}
}

func TestMigrationFreshInstallUpgradeBackupRestore(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test migration install, upgrade, backup, and restore")
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is required for migration backup and restore testing")
	}
	ownerDSN := os.Getenv("MIGRATION_DATABASE_URL")
	if ownerDSN == "" {
		ownerDSN = os.Getenv("DATABASE_URL")
	}
	adminDSN, err := databaseDSN(ownerDSN, "postgres")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := NewDB(DefaultConfig(adminDSN))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	stamp := strconv.FormatInt(time.Now().UnixNano(), 10)
	freshName, upgradeName, restoredName := "openchain_fresh_"+stamp, "openchain_upgrade_"+stamp, "openchain_restore_"+stamp
	for _, name := range []string{freshName, upgradeName, restoredName} {
		name := name
		defer dropTestDatabase(admin, name)
	}
	for _, name := range []string{freshName, upgradeName, restoredName} {
		if _, err := admin.SQL.ExecContext(ctx, "CREATE DATABASE "+name); err != nil {
			t.Fatal(err)
		}
	}

	freshDSN, err := databaseDSN(ownerDSN, freshName)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := NewDB(DefaultConfig(freshDSN))
	if err != nil {
		t.Fatal(err)
	}
	if err := fresh.ApplyMigrations(ctx); err != nil {
		fresh.Close()
		t.Fatal(err)
	}
	assertMigrationLedger(t, ctx, fresh)
	if _, err := fresh.SQL.ExecContext(ctx, `INSERT INTO public.transfers (id, network, transaction_hash, event_id, transfer_kind, from_address, to_address, asset_symbol, asset_kind, asset_contract_address, asset_decimals, amount_base_units, block_number, block_timestamp, source, retrieved_at) VALUES ('migration-backup-test', 'test', '0xtest', 'tx', 'NATIVE', 'from', 'to', 'TEST', 'NATIVE', '', 18, '1', 1, now(), 'test', now())`); err != nil {
		fresh.Close()
		t.Fatal(err)
	}
	backup, err := os.CreateTemp(t.TempDir(), "openchain-migration-*.sql")
	if err != nil {
		fresh.Close()
		t.Fatal(err)
	}
	if err := backup.Close(); err != nil {
		fresh.Close()
		t.Fatal(err)
	}
	if output, err := postgresTool(ctx, "pg_dump", "--dbname="+freshDSN).Output(); err != nil {
		fresh.Close()
		t.Fatalf("backup failed: %v: %s", err, output)
	} else if err := os.WriteFile(backup.Name(), output, 0o600); err != nil {
		fresh.Close()
		t.Fatal(err)
	}
	fresh.Close()
	restoredDSN, err := databaseDSN(ownerDSN, restoredName)
	if err != nil {
		t.Fatal(err)
	}
	backupInput, err := os.Open(backup.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer backupInput.Close()
	restore := postgresTool(ctx, "psql", "--dbname="+restoredDSN, "--set=ON_ERROR_STOP=1")
	restore.Stdin = backupInput
	if output, err := restore.CombinedOutput(); err != nil {
		t.Fatalf("restore failed: %v: %s", err, output)
	}
	restored, err := NewDB(DefaultConfig(restoredDSN))
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	assertMigrationLedger(t, ctx, restored)
	var restoredTransfers int
	if err := restored.SQL.QueryRowContext(ctx, "SELECT count(*) FROM public.transfers WHERE id = 'migration-backup-test'").Scan(&restoredTransfers); err != nil || restoredTransfers != 1 {
		t.Fatalf("restored transfer count = %d, err = %v", restoredTransfers, err)
	}

	upgradeDSN, err := databaseDSN(ownerDSN, upgradeName)
	if err != nil {
		t.Fatal(err)
	}
	upgrade, err := NewDB(DefaultConfig(upgradeDSN))
	if err != nil {
		t.Fatal(err)
	}
	defer upgrade.Close()
	if err := applyLegacyMigrations(ctx, upgrade); err != nil {
		t.Fatal(err)
	}
	if err := upgrade.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	assertMigrationLedger(t, ctx, upgrade)
	if _, err := upgrade.SQL.ExecContext(ctx, "UPDATE public.schema_migrations SET checksum = 'changed' WHERE version = '0001_initial'"); err != nil {
		t.Fatal(err)
	}
	if err := upgrade.ApplyMigrations(ctx); err == nil || !strings.Contains(err.Error(), "migration checksum mismatch") {
		t.Fatalf("checksum drift error = %v", err)
	}
}

func databaseDSN(value, database string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("parse database URL")
	}
	parsed.Path = "/" + database
	return parsed.String(), nil
}

func applyLegacyMigrations(ctx context.Context, database *DB) error {
	migrations, err := embeddedMigrations()
	if err != nil {
		return err
	}
	tx, err := database.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, migration := range migrations[:2] {
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, "CREATE TABLE public.schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())"); err != nil {
		return err
	}
	for _, migration := range migrations[:2] {
		if _, err := tx.ExecContext(ctx, "INSERT INTO public.schema_migrations (version) VALUES ($1)", migration.version); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func assertMigrationLedger(t *testing.T, ctx context.Context, database *DB) {
	t.Helper()
	migrations, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := database.SQL.QueryContext(ctx, "SELECT version, checksum FROM public.schema_migrations")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	applied := make(map[string]string)
	for rows.Next() {
		var version, checksum string
		if err := rows.Scan(&version, &checksum); err != nil {
			t.Fatal(err)
		}
		applied[version] = checksum
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := verifyMigrationLedger(migrations, applied); err != nil || len(applied) != len(migrations) {
		t.Fatalf("migration ledger = %#v, err = %v", applied, err)
	}
}

func dropTestDatabase(admin *DB, name string) {
	_, _ = admin.SQL.Exec("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND pid <> pg_backend_pid()", name)
	_, _ = admin.SQL.Exec("DROP DATABASE IF EXISTS " + name)
}

func postgresTool(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, "docker", append([]string{"exec", "-i", "openchain-postgres"}, args...)...)
}

func TestRuntimeRoleCanReadAGEWithoutSuperuserLoad(t *testing.T) {
	if os.Getenv("OPENCHAIN_DB_INTEGRATION_TEST") != "1" {
		t.Skip("set OPENCHAIN_DB_INTEGRATION_TEST=1 to test runtime database role")
	}
	owner, err := NewDB(DefaultConfig(os.Getenv("DATABASE_URL")))
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := owner.ApplyMigrations(ctx); err != nil {
		t.Fatal(err)
	}
	runtimeURL, err := url.Parse(owner.DSN)
	if err != nil {
		t.Fatal(err)
	}
	role := "openchain_runtime_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	runtimeURL.User = url.UserPassword(role, "runtime-test-password")
	if err := owner.ProvisionRuntimeRole(ctx, runtimeURL.String()); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = owner.SQL.ExecContext(context.Background(), fmt.Sprintf("DROP OWNED BY %q; DROP ROLE %q", role, role))
	}()
	runtime, err := NewDB(DefaultConfig(runtimeURL.String()))
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	address := "runtime-role-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	if err := runtime.SaveEvidenceGraph(ctx, AcquisitionScope{}, []Address{{Network: "runtime-test", Address: address}}, nil, nil); err != nil {
		t.Fatalf("restricted runtime AGE write: %v", err)
	}
	if _, err := runtime.GraphNeighbors(ctx, "runtime-test", address, "both", 1); err != nil {
		t.Fatalf("restricted runtime AGE query: %v", err)
	}
	cleanupGraph(t, owner, "", "runtime-test", address, "")
}
