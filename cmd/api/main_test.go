// Tests for the api CLI binary.
//
// All tests are white-box (package main) so they can call setupDB() directly
// without spawning a subprocess or intercepting os.Exit. The full main()
// lifecycle (signal handling, HTTP server) is intentionally excluded — it
// is covered by the api package tests (internal/api/server_test.go) and
// end-to-end smoke tests at the infrastructure layer.
//
// A throwaway PostgreSQL container is started per test via testutil.SetupPostgres,
// mirroring the pattern used in internal/infrastructure/database/database_test.go.
package main

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/testutil"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/logger"
)

func newTestLogger(t *testing.T) *zap.Logger {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "debug", Encoding: "console"})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestSetupDB_EmptyDSN_ReturnsNilPool(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{}

	pool, err := setupDB(context.Background(), cfg, log)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool != nil {
		pool.Close()
		t.Fatal("expected nil pool for empty DSN, got non-nil")
	}
}

func TestSetupDB_ValidDSN_MigratesAndReturnsPool(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	log := newTestLogger(t)
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			DSN:             dsn,
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
		},
	}

	pool, err := setupDB(context.Background(), cfg, log)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pool == nil {
		t.Fatal("expected non-nil pool for valid DSN, got nil")
	}
	pool.Close()
}

func TestSetupDB_InvalidDSN_ReturnsError(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{
		Database: config.DatabaseConfig{
			DSN:             "postgres://invalid:5432/nodb?sslmode=disable",
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
		},
	}

	_, err := setupDB(context.Background(), cfg, log)
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
}

// ── setupEventBus ─────────────────────────────────────────────────────────────

func TestSetupEventBus_InMemoryDriver_ReturnsBusAndNoopCleanup(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{EventBus: config.EventBusConfig{Driver: "in_memory"}}

	bus, cleanup, err := setupEventBus(context.Background(), cfg, log)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
	// cleanup must not panic for the in-memory driver (no-op)
	cleanup()
}

func TestSetupEventBus_UnknownDriver_FallsBackToInMemory(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{EventBus: config.EventBusConfig{Driver: "kafka"}}

	bus, cleanup, err := setupEventBus(context.Background(), cfg, log)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus == nil {
		t.Fatal("expected non-nil bus after fallback")
	}
	cleanup()
}

func TestSetupEventBus_RedisDriver_InvalidAddr_ReturnsError(t *testing.T) {
	log := newTestLogger(t)
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: "redis"},
		Redis:    config.RedisConfig{Addr: "localhost:0", Password: "", DB: 0},
	}

	bus, cleanup, err := setupEventBus(context.Background(), cfg, log)

	if err == nil {
		t.Fatal("expected error for unreachable Redis, got nil")
	}
	if bus != nil {
		t.Fatal("expected nil bus on connection failure")
	}
	// cleanup must not panic even when construction failed
	cleanup()
}

func TestSetupEventBus_RedisDriver_ValidAddr_ReturnsBusAndCleanup(t *testing.T) {
	mr := miniredis.RunT(t)
	log := newTestLogger(t)
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: "redis"},
		Redis:    config.RedisConfig{Addr: mr.Addr(), Password: "", DB: 0},
	}

	bus, cleanup, err := setupEventBus(context.Background(), cfg, log)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
	// cleanup must stop goroutines and close the client without panicking
	cleanup()
}
