package database_test

import (
	"context"
	"testing"
	"time"

	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/migrations"
)

const (
	fmtUnexpectedErr  = "unexpected error: %v"
	fmtExpectedErr    = "expected an error, got nil"
	dbImage           = "postgres:17-alpine"
	dbName            = "quiniela_test"
	dbUser            = "test"
	dbPassword        = "test"
)

// setupPostgres starts a throwaway PostgreSQL container and returns its DSN.
// The container is terminated automatically when the test finishes.
func setupPostgres(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, dbImage,
		tcpostgres.WithDatabase(dbName),
		tcpostgres.WithUsername(dbUser),
		tcpostgres.WithPassword(dbPassword),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	})

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("failed to get connection string: %v", err)
	}
	return dsn
}

// ── Migrate ───────────────────────────────────────────────────────────────────

func TestMigrate_AppliesPendingMigrations(t *testing.T) {
	dsn := setupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestMigrate_IdempotentOnSecondCall(t *testing.T) {
	dsn := setupPostgres(t)

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
	dsn := setupPostgres(t)

	// Schema must exist before seeding.
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("new pool: %v", err)
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

func TestSeed_IdempotentOnSecondCall(t *testing.T) {
	dsn := setupPostgres(t)

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    5,
		MaxIdleConns:    2,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	ctx := context.Background()
	if err := database.Seed(ctx, pool); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	// ON CONFLICT DO NOTHING — second call must not error or duplicate rows.
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
