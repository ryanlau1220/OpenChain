package db

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strconv"
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
