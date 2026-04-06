// Tests for the migrate CLI binary.
//
// All tests are white-box (package main) so they can call run() directly
// without spawning a subprocess. Using run() instead of main() avoids the
// os.Exit calls that would kill the test process on error paths.
//
// A throwaway PostgreSQL container is started per test via testutil.SetupPostgres,
// mirroring the pattern used in internal/infrastructure/database/database_test.go.
// This ensures the migration and seed paths are exercised against a real
// database engine rather than a mock, which is the only reliable way to catch
// schema or SQL errors.
package main

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/testutil"
)

func TestRun_AppliesMigrations(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := run(dsn, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_AppliesMigrationsAndSeeds(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := run(dsn, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_EmptyDSN_ReturnsError(t *testing.T) {
	err := run("", false)
	if err == nil {
		t.Fatal("expected error for empty DSN, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_DSN") {
		t.Errorf("expected error to mention WCQ_DATABASE_DSN, got: %v", err)
	}
}

func TestRun_InvalidDSN_ReturnsError(t *testing.T) {
	if err := run("postgres://invalid:5432/nodb?sslmode=disable", false); err == nil {
		t.Fatal("expected error for unreachable DSN, got nil")
	}
}
