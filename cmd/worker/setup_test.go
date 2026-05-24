package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/config"
)

// redisOnlyCfg returns a Config with a reachable Redis (backed by miniredis)
// and a deliberately invalid database DSN. It is used by tests that need
// setupEventBus to succeed (to cover its success path) while keeping setupDB
// in an error state (to avoid needing a real Postgres instance).
func redisOnlyCfg(t *testing.T) *config.Config {
	t.Helper()
	mr := miniredis.RunT(t)
	return &config.Config{
		EventBus: config.EventBusConfig{Driver: driverRedis},
		Redis:    config.RedisConfig{Addr: mr.Addr()},
		Database: config.DatabaseConfig{DSN: "://invalid-dsn"},
	}
}

// ── setupDB ───────────────────────────────────────────────────────────────────

func TestSetupDB_EmptyDSN_ReturnsError(t *testing.T) {
	cfg := &config.Config{}
	_, err := setupDB(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for empty DSN, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_DATABASE_DSN") {
		t.Errorf("expected error to reference WCQ_DATABASE_DSN, got: %v", err)
	}
}

// ── setupEventBus ─────────────────────────────────────────────────────────────

func TestSetupEventBus_InMemoryDriver_ReturnsError(t *testing.T) {
	cfg := &config.Config{EventBus: config.EventBusConfig{Driver: "in_memory"}}
	_, _, err := setupEventBus(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for in_memory driver, got nil")
	}
	if !strings.Contains(err.Error(), driverRedis) {
		t.Errorf("expected error to mention %s, got: %v", driverRedis, err)
	}
}

func TestSetupEventBus_EmptyDriver_ReturnsError(t *testing.T) {
	cfg := &config.Config{EventBus: config.EventBusConfig{Driver: ""}}
	_, _, err := setupEventBus(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for empty driver, got nil")
	}
}

func TestSetupEventBus_UnknownDriver_ReturnsError(t *testing.T) {
	cfg := &config.Config{EventBus: config.EventBusConfig{Driver: "kafka"}}
	_, _, err := setupEventBus(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for unknown driver, got nil")
	}
	if !strings.Contains(err.Error(), "kafka") {
		t.Errorf("expected error to echo the bad driver value, got: %v", err)
	}
}

func TestSetupEventBus_RedisUnreachable_ReturnsError(t *testing.T) {
	// Port 1 is reserved and will produce an immediate "connection refused"
	// from the OS without blocking - no timeout required.
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: driverRedis},
		Redis:    config.RedisConfig{Addr: "localhost:1"},
	}
	_, _, err := setupEventBus(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for unreachable Redis, got nil")
	}
}

func TestSetupDB_InvalidDSN_ReturnsError(t *testing.T) {
	// A DSN with an unrecognised scheme fails during pgxpool.ParseConfig,
	// before any network connection is attempted.
	cfg := &config.Config{
		Database: config.DatabaseConfig{DSN: "://not-a-valid-dsn"},
	}
	_, err := setupDB(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
}

// ── setupMetrics ──────────────────────────────────────────────────────────────

func TestSetupMetrics_Enabled_LogsAndReturnsHandler(t *testing.T) {
	cfg := &config.Config{Metrics: config.MetricsConfig{Enabled: true, Namespace: "test_setup_worker"}}

	_, handler, shutdown, err := setupMetrics(cfg, zap.NewNop())

	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if handler == nil {
		t.Fatal("expected non-nil handler when metrics are enabled")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := shutdown(ctx); err != nil {
		t.Fatalf("shutdown(enabled): %v", err)
	}
}

func TestSetupEventBus_RedisReachable_ReturnsCleanBus(t *testing.T) {
	// miniredis starts an in-process Redis server so we can exercise the full
	// success path of setupEventBus without a real Redis instance.
	mr := miniredis.RunT(t)

	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: driverRedis},
		Redis:    config.RedisConfig{Addr: mr.Addr()},
	}
	bus, cleanup, err := setupEventBus(context.Background(), cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bus == nil {
		t.Fatal("expected non-nil bus")
	}
	// cleanup must not panic.
	cleanup()
}
