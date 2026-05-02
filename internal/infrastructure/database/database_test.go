package database_test

import (
	"context"
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
