package main

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/migrations"
)

// integrationPool is initialised once per test run when the container starts
// successfully. Tests that require it call requireIntegration(t).
var integrationPool *pgxpool.Pool
var integrationDSN string

func TestMain(m *testing.M) {
	pool, dsn, cleanup := mustSetupIntegrationDB()
	if pool != nil {
		integrationPool = pool
		integrationDSN = dsn
	}
	code := m.Run()
	if pool != nil {
		cleanup()
	}
	os.Exit(code)
}

// mustSetupIntegrationDB starts a PostgreSQL test container and runs all
// migrations. Returns (nil, "", noop) if startup fails so unit tests can
// still run in environments without Docker.
func mustSetupIntegrationDB() (*pgxpool.Pool, string, func()) {
	ctx := context.Background()

	container, err := tcpostgres.Run(ctx, "postgres:17-alpine",
		tcpostgres.WithDatabase("quiniela_test"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		log.Printf("validate-params integration: skip (no Docker): %v", err)
		return nil, "", func() {}
	}

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Printf("validate-params integration: connection string: %v", err)
		_ = container.Terminate(ctx)
		return nil, "", func() {}
	}

	if err := database.Migrate(dsn, migrations.FS); err != nil {
		log.Printf("validate-params integration: migrate: %v", err)
		_ = container.Terminate(ctx)
		return nil, "", func() {}
	}

	pool, err := database.NewPool(ctx, database.Config{
		DSN:             dsn,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		log.Printf("validate-params integration: pool: %v", err)
		_ = container.Terminate(ctx)
		return nil, "", func() {}
	}

	cleanup := func() {
		pool.Close()
		if err := container.Terminate(ctx); err != nil {
			log.Printf("validate-params integration: terminate: %v", err)
		}
	}
	return pool, dsn, cleanup
}

// requireIntegration skips the test when the integration container is not
// available (e.g., no Docker in the current environment).
func requireIntegration(t *testing.T) {
	t.Helper()
	if integrationPool == nil {
		t.Skip("integration DB not available")
	}
}

// ── integration tests ─────────────────────────────────────────────────────────

func TestFetchAllParams_ReturnsMigratedRows(t *testing.T) {
	requireIntegration(t)

	params, err := fetchAllParams(context.Background(), integrationPool)
	if err != nil {
		t.Fatalf("fetchAllParams: %v", err)
	}
	if len(params) == 0 {
		t.Fatal("expected system_params rows after migrations, got none")
	}
	for _, p := range params {
		if p.key == "" {
			t.Error("found row with empty key")
		}
	}
}

func TestConnectDatabase_ValidDSN_ReturnsPool(t *testing.T) {
	requireIntegration(t)

	prev := os.Getenv("DATABASE_URL")
	os.Setenv("DATABASE_URL", integrationDSN)
	defer os.Setenv("DATABASE_URL", prev)

	pool, err := connectDatabase()
	if err != nil {
		t.Fatalf("connectDatabase: %v", err)
	}
	pool.Close()
}

func TestRun_ValidDB_PassesValidation(t *testing.T) {
	requireIntegration(t)

	prev := os.Getenv("DATABASE_URL")
	os.Setenv("DATABASE_URL", integrationDSN)
	defer os.Setenv("DATABASE_URL", prev)

	if err := run(); err != nil {
		t.Errorf("run() with migrated DB: %v", err)
	}
}
