// Tests for the migrate CLI binary.
//
// Most tests call run() directly rather than main() to avoid os.Exit — the
// package-main white-box pattern. The one exception is TestMain_ErrorExitsWithCode1,
// which replaces osExit so that the error path in main() can be exercised
// without terminating the test process. flag.CommandLine is reset before each
// main() call to prevent "flag redefined" panics when main() registers its
// flags on subsequent calls.
//
// A throwaway PostgreSQL container is started per test via testutil.SetupPostgres,
// mirroring the pattern used in internal/infrastructure/database/database_test.go.
// This ensures the migration and seed paths are exercised against a real
// database engine rather than a mock, which is the only reliable way to catch
// schema or SQL errors.
package main

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/testutil"
)

// moduleBaselinePath resolves migrations/baseline/schema.sql relative to this
// source file so tests work regardless of the working directory at test time.
func moduleBaselinePath() string {
	_, selfPath, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(selfPath), "..", "..")
	return filepath.Join(root, "migrations", "baseline", "schema.sql")
}

func TestRun_AppliesMigrations(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := run(dsn, false, false, defaultBaselinePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_AppliesMigrationsAndSeeds(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := run(dsn, true, false, defaultBaselinePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_EmptyDSN_ReturnsError(t *testing.T) {
	err := run("", false, false, defaultBaselinePath)
	if err == nil {
		t.Fatal("expected error for empty DSN, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_DSN") {
		t.Errorf("expected error to mention WCQ_DATABASE_DSN, got: %v", err)
	}
}

func TestRun_InvalidDSN_ReturnsError(t *testing.T) {
	if err := run("postgres://invalid:5432/nodb?sslmode=disable", false, false, defaultBaselinePath); err == nil {
		t.Fatal("expected error for unreachable DSN, got nil")
	}
}

func TestRun_Fresh_AppliesBaseline(t *testing.T) {
	baselinePath := moduleBaselinePath()
	if _, err := os.Stat(baselinePath); os.IsNotExist(err) {
		t.Skip("baseline schema.sql not present; run make schema-dump first")
	}
	dsn := testutil.SetupPostgres(t)

	if err := run(dsn, false, true, baselinePath); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRun_Fresh_MissingBaseline_ReturnsError(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	err := run(dsn, false, true, "/nonexistent/schema.sql")
	if err == nil {
		t.Fatal("expected error for missing baseline file, got nil")
	}
	if !strings.Contains(err.Error(), "migrate fresh") {
		t.Errorf("expected error to mention 'migrate fresh', got: %v", err)
	}
}

// TestMain_ErrorExitsWithCode1 exercises the main() error path by replacing
// osExit with a recorder so the process is not terminated, and resetting
// flag.CommandLine to avoid "flag redefined" panics on repeated calls.
func TestMain_ErrorExitsWithCode1(t *testing.T) {
	// Reset flag state so main() can re-register its flags without panicking.
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ContinueOnError)

	// Capture the exit code instead of terminating the test process.
	origExit := osExit
	var code int
	osExit = func(c int) { code = c }
	defer func() { osExit = origExit }()

	// Clear WCQ_DATABASE_DSN so run() fails immediately (no DB needed).
	prev := os.Getenv("WCQ_DATABASE_DSN")
	os.Unsetenv("WCQ_DATABASE_DSN")
	defer os.Setenv("WCQ_DATABASE_DSN", prev)

	// Suppress flag.Parse parsing test-runner flags by clearing non-binary args.
	origArgs := os.Args
	os.Args = os.Args[:1]
	defer func() { os.Args = origArgs }()

	main()

	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
}
