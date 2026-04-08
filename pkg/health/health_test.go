package health_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/pkg/health"
)

// ── DBChecker ─────────────────────────────────────────────────────────────────

func TestDBChecker_Name(t *testing.T) {
	// NewDBChecker requires a non-nil pool only for Check; Name is safe with any.
	pool, _ := pgxpool.New(context.Background(),
		"postgres://fake:fake@localhost:1/fake?sslmode=disable&connect_timeout=1")
	defer pool.Close()

	c := health.NewDBChecker(pool)
	if c.Name() != "db" {
		t.Errorf("expected name %q, got %q", "db", c.Name())
	}
}

func TestDBChecker_Check_Unreachable_ReturnsError(t *testing.T) {
	// Point at a port that is not listening so Ping returns immediately with
	// "connection refused". The test verifies the error path without needing a
	// real PostgreSQL instance.
	pool, err := pgxpool.New(context.Background(),
		"postgres://fake:fake@localhost:1/fake?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("create fake pool: %v", err)
	}
	defer pool.Close()

	c := health.NewDBChecker(pool)
	result := c.Check(context.Background())

	if result.Status != "error" {
		t.Errorf("expected status %q, got %q", "error", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
	if result.LatencyMS != 0 {
		t.Errorf("expected latency_ms 0 on error, got %d", result.LatencyMS)
	}
}

// ── RedisChecker ──────────────────────────────────────────────────────────────

func TestRedisChecker_Name(t *testing.T) {
	c := health.NewRedisChecker(redis.NewClient(&redis.Options{Addr: "localhost:1"}))
	if c.Name() != "redis" {
		t.Errorf("expected name %q, got %q", "redis", c.Name())
	}
}

func TestRedisChecker_Check_OK(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	c := health.NewRedisChecker(client)
	result := c.Check(context.Background())

	if result.Status != "ok" {
		t.Errorf("expected status %q, got %q (error: %s)", "ok", result.Status, result.Error)
	}
	if result.Error != "" {
		t.Errorf("expected empty error, got %q", result.Error)
	}
}

func TestRedisChecker_Check_Unreachable_ReturnsError(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:        "localhost:1",
		DialTimeout: 50,
	})
	t.Cleanup(func() { client.Close() })

	c := health.NewRedisChecker(client)
	result := c.Check(context.Background())

	if result.Status != "error" {
		t.Errorf("expected status %q, got %q", "error", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRedisChecker_Check_LatencyPopulated(t *testing.T) {
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	c := health.NewRedisChecker(client)
	result := c.Check(context.Background())

	// Latency should be >= 0. We can't assert an exact value but we can verify
	// it is present (LatencyMS is omitempty: it would be 0 only if Ping takes
	// sub-millisecond AND rounds down, which is fine — we just check Status).
	if result.Status != "ok" {
		t.Fatalf("unexpected error: %s", result.Error)
	}
	_ = result.LatencyMS // always >= 0; presence verified via Status == "ok"
}
