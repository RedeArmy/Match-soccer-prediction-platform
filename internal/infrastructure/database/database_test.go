package database_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/testutil"
	"github.com/rede/world-cup-quiniela/migrations"
)

const (
	fmtUnexpectedErr = "unexpected error: %v"
	fmtExpectedErr   = "expected an error, got nil"
	fmtMigrateErr    = "migrate: %v"
	fmtNewPoolErr    = "new pool: %v"
)

// ── NewPool ───────────────────────────────────────────────────────────────────

// TestNewPool_ContextCancelledDuringBackoff covers the select case in NewPool
// where a context is cancelled while waiting in the exponential-backoff loop.
// Port 1 on 127.0.0.1 is always refused, so Ping fails on the first attempt;
// a pre-cancelled context causes the backoff select to fire ctx.Done immediately.
func TestNewPool_ContextCancelledDuringBackoff_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := database.NewPool(ctx, database.Config{
		DSN:             "postgres://u:p@127.0.0.1:1/db",
		MaxOpenConns:    1,
		MaxIdleConns:    0,
		ConnMaxLifetime: time.Minute,
	})
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	if !strings.Contains(err.Error(), "context cancelled during retry") {
		t.Errorf("expected context cancellation message, got: %v", err)
	}
}

// ── Migrate ───────────────────────────────────────────────────────────────────

func TestMigrate_AppliesPendingMigrations(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestMigrate_IdempotentOnSecondCall(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("first migration: %v", err)
	}
	// Second call must return nil (ErrNoChange treated as success).
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("second migration (idempotent): %v", err)
	}
}

func TestMigrate_InvalidDSN_ReturnsError(t *testing.T) {
	if err := database.Migrate("postgres://invalid:5432/nodb?sslmode=disable", migrations.FS); err == nil {
		t.Fatal(fmtExpectedErr)
	}
}

// ── Seed ─────────────────────────────────────────────────────────────────────

func TestSeed_InsertsFixtures(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	// Schema must exist before seeding.
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtMigrateErr, err)
	}

	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	if err := database.Seed(context.Background(), pool); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Verify at least the seeded users and matches were inserted.
	var userCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM users").Scan(&userCount); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if userCount == 0 {
		t.Error("expected seeded users, got 0")
	}

	var matchCount int
	if err := pool.QueryRow(context.Background(), "SELECT COUNT(*) FROM matches").Scan(&matchCount); err != nil {
		t.Fatalf("count matches: %v", err)
	}
	if matchCount == 0 {
		t.Error("expected seeded matches, got 0")
	}
}

func TestSeed_StadiumsTableMissing_ReturnsError(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtMigrateErr, err)
	}

	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	if _, err := pool.Exec(context.Background(), "DROP TABLE IF EXISTS stadiums CASCADE"); err != nil {
		t.Fatalf("drop stadiums: %v", err)
	}

	if err := database.Seed(context.Background(), pool); err == nil {
		t.Fatal("expected error when stadiums table is missing, got nil")
	}
}

// newMigratedPool is a helper that runs migrations + returns a connected pool.
// Each call gets its own Postgres container via SetupPostgres.
func newMigratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := testutil.SetupPostgres(t)
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtMigrateErr, err)
	}
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestSeed_CountriesTableMissing_ReturnsError(t *testing.T) {
	pool := newMigratedPool(t)
	if _, err := pool.Exec(context.Background(), "DROP TABLE IF EXISTS countries CASCADE"); err != nil {
		t.Fatalf("drop countries: %v", err)
	}
	if err := database.Seed(context.Background(), pool); err == nil {
		t.Fatal("expected error when countries table is missing, got nil")
	}
}

func TestSeed_CitiesTableMissing_ReturnsError(t *testing.T) {
	pool := newMigratedPool(t)
	if _, err := pool.Exec(context.Background(), "DROP TABLE IF EXISTS cities CASCADE"); err != nil {
		t.Fatalf("drop cities: %v", err)
	}
	if err := database.Seed(context.Background(), pool); err == nil {
		t.Fatal("expected error when cities table is missing, got nil")
	}
}

func TestSeed_MatchesTableMissing_ReturnsError(t *testing.T) {
	pool := newMigratedPool(t)
	if _, err := pool.Exec(context.Background(), "DROP TABLE IF EXISTS matches CASCADE"); err != nil {
		t.Fatalf("drop matches: %v", err)
	}
	if err := database.Seed(context.Background(), pool); err == nil {
		t.Fatal("expected error when matches table is missing, got nil")
	}
}

// baselineSQLPath resolves migrations/baseline/schema.sql relative to this
// source file so tests work regardless of working directory.
func baselineSQLPath() string {
	_, selfPath, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(selfPath), "..", "..", "..")
	return filepath.Join(root, "migrations", "baseline", "schema.sql")
}

// ── ApplyBaseline ─────────────────────────────────────────────────────────────

func TestApplyBaseline_CreatesSchema(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	ddl := `CREATE TABLE IF NOT EXISTS baseline_probe (id SERIAL PRIMARY KEY);`
	if err := database.ApplyBaseline(context.Background(), pool, ddl); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var exists bool
	err = pool.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'baseline_probe')`).
		Scan(&exists)
	if err != nil {
		t.Fatalf("query table existence: %v", err)
	}
	if !exists {
		t.Error("expected baseline_probe table to exist after ApplyBaseline")
	}
}

func TestApplyBaseline_InvalidSQL_ReturnsError(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	if err := database.ApplyBaseline(context.Background(), pool, "NOT VALID SQL !!!"); err == nil {
		t.Fatal(fmtExpectedErr)
	}
}

// ── MarkMigrationsApplied ────────────────────────────────────────────────────

func TestMarkMigrationsApplied_InsertsVersionRows(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := database.MarkMigrationsApplied(ctx, pool, migrations.FS); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations").Scan(&count); err != nil {
		t.Fatalf("count schema_migrations: %v", err)
	}
	if count == 0 {
		t.Error("expected at least one row in schema_migrations after MarkMigrationsApplied")
	}
}

func TestMarkMigrationsApplied_Idempotent(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := database.MarkMigrationsApplied(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := database.MarkMigrationsApplied(ctx, pool, migrations.FS); err != nil {
		t.Fatalf("second call (idempotent): %v", err)
	}
}

func TestMarkMigrationsApplied_DirtyFlagFalse(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := database.MarkMigrationsApplied(ctx, pool, migrations.FS); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var dirtyCount int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM schema_migrations WHERE dirty = true").Scan(&dirtyCount); err != nil {
		t.Fatalf("count dirty rows: %v", err)
	}
	if dirtyCount != 0 {
		t.Errorf("expected 0 dirty rows, got %d", dirtyCount)
	}
}

// ── MigrateFresh ─────────────────────────────────────────────────────────────

func TestMigrateFresh_FullBootstrap(t *testing.T) {
	baselineFile := baselineSQLPath()
	if _, err := os.Stat(baselineFile); os.IsNotExist(err) {
		t.Skip("baseline schema.sql not present; run cmd/genschema first")
	}

	dsn := testutil.SetupPostgres(t)
	ctx := context.Background()

	if err := database.MigrateFresh(ctx, dsn, baselineFile, migrations.FS); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Subsequent Migrate must be a no-op (all migrations already marked applied).
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("Migrate after MigrateFresh should be no-op: %v", err)
	}

	// Verify a known table from the baseline exists.
	pool, err := database.NewPool(ctx, database.Config{
		DSN: dsn, MaxOpenConns: 3, MaxIdleConns: 1, ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	var exists bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_name = 'system_params')`).
		Scan(&exists); err != nil {
		t.Fatalf("query table existence: %v", err)
	}
	if !exists {
		t.Error("expected system_params table after MigrateFresh")
	}
}

func TestMigrateFresh_MissingBaselineFile_ReturnsError(t *testing.T) {
	// MigrateFresh reads the baseline file before dialling the database, so no
	// real connection is needed here. Any DSN is accepted; the function returns
	// on the os.ReadFile error before NewPool is ever called.
	err := database.MigrateFresh(context.Background(), "postgres://x:x@localhost/x", "/nonexistent/schema.sql", migrations.FS)
	if err == nil {
		t.Fatal(fmtExpectedErr)
	}
}

func TestMigrateFresh_InvalidDSN_ReturnsError(t *testing.T) {
	// Write a minimal valid SQL to a temp file so we get past the file read step.
	f, err := os.CreateTemp(t.TempDir(), "schema-*.sql")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := f.WriteString("SELECT 1;"); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	f.Close()

	// Bound the dial so this test never consumes more than 5 s of the package's
	// 60 s budget, even when testcontainer-based tests run first and leave only
	// a thin slice of time. 5 s is enough to exercise at least one retry-plus-
	// backoff cycle (1 s sleep) before ctx.Done() fires; it is far shorter than
	// the OS-level TCP SYN-retry window that would otherwise block indefinitely.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = database.MigrateFresh(ctx, "postgres://invalid:5432/nodb?sslmode=disable", f.Name(), migrations.FS)
	if err == nil {
		t.Fatal(fmtExpectedErr)
	}
}

func TestSeed_IdempotentOnSecondCall(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtMigrateErr, err)
	}

	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf(fmtNewPoolErr, err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := database.Seed(ctx, pool); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// ON CONFLICT DO NOTHING - second call must not error or duplicate rows.
	if err := database.Seed(ctx, pool); err != nil {
		t.Fatalf("second seed (idempotent): %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	// Exactly the 3 seeded users, not 6 (no duplicates).
	if count != 3 {
		t.Errorf("expected 3 users after idempotent seed, got %d", count)
	}
}
