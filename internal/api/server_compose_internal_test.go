// Package api provides internal white-box tests for unexported helpers in the
// composition root.  These tests must live in package api (not api_test) so
// they can reach private functions that are intentionally not part of the
// public API surface.
package api

import (
	"errors"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// errFailMeter is a noop Meter that forces Int64Counter to return an error.
// Used to exercise the RegisterMetrics error path in buildIPRateStore.
type errFailMeter struct{ metricnoop.Meter }

var errForcedCounter = errors.New("injected counter failure")

func (errFailMeter) Int64Counter(_ string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errForcedCounter
}

// ── buildIPRateStore ──────────────────────────────────────────────────────────

// TestBuildIPRateStore_NoRedis_ReturnsLimiterStore verifies that when no Redis
// client is configured, buildIPRateStore returns an in-process LimiterStore so
// the server starts without requiring a Redis connection.
func TestBuildIPRateStore_NoRedis_ReturnsLimiterStore(t *testing.T) {
	s := &Server{log: zaptest.NewLogger(t)} // redisClient == nil
	meter := metricnoop.NewMeterProvider().Meter("test")
	store := s.buildIPRateStore(meter, 10.0, 20)
	if _, ok := store.(*middleware.LimiterStore); !ok {
		t.Errorf("expected *middleware.LimiterStore when Redis is not configured, got %T", store)
	}
}

// TestBuildIPRateStore_WithRedis_ReturnsRedisRateStore verifies that when a
// Redis client is available, buildIPRateStore returns a RedisRateStore so
// IP rate limits are enforced cluster-wide across replicas.
func TestBuildIPRateStore_WithRedis_ReturnsRedisRateStore(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
	meter := metricnoop.NewMeterProvider().Meter("test")
	store := s.buildIPRateStore(meter, 10.0, 20)
	if _, ok := store.(*middleware.RedisRateStore); !ok {
		t.Errorf("expected *middleware.RedisRateStore when Redis is configured, got %T", store)
	}
}

// TestBuildIPRateStore_WithRedis_RegisterMetricsError_StillReturnsStore verifies
// the fail-soft behaviour: when OTel counter registration fails, buildIPRateStore
// logs a warning but still returns a valid RedisRateStore so the server can
// continue to serve traffic without metrics.
func TestBuildIPRateStore_WithRedis_RegisterMetricsError_StillReturnsStore(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
	// errFailMeter forces Int64Counter to return an error, exercising the
	// s.log.Warn("RedisRateStore.RegisterMetrics failed...") branch.
	store := s.buildIPRateStore(errFailMeter{}, 5.0, 10)
	if store == nil {
		t.Fatal("buildIPRateStore must not return nil when RegisterMetrics fails")
	}
	if _, ok := store.(*middleware.RedisRateStore); !ok {
		t.Errorf("expected *middleware.RedisRateStore despite metric error, got %T", store)
	}
}

// TestBuildIPRateStore_AllowsRequests verifies that the store returned by
// buildIPRateStore (regardless of Redis availability) implements IPAllower
// and allows requests within the configured burst.
func TestBuildIPRateStore_AllowsRequests(t *testing.T) {
	t.Run("in_process", func(t *testing.T) {
		s := &Server{log: zaptest.NewLogger(t)}
		store := s.buildIPRateStore(metricnoop.NewMeterProvider().Meter("test"), 100.0, 10)
		allowed, _ := store.Allow(t.Context(), "ip:global:1.2.3.4")
		if !allowed {
			t.Error("expected Allow=true for first request within burst, got false")
		}
	})

	t.Run("redis_backed", func(t *testing.T) {
		mr := miniredis.RunT(t)
		rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
		t.Cleanup(func() { _ = rc.Close() })

		s := &Server{log: zaptest.NewLogger(t), redisClient: rc}
		store := s.buildIPRateStore(metricnoop.NewMeterProvider().Meter("test"), 100.0, 10)
		allowed, _ := store.Allow(t.Context(), "ip:global:5.6.7.8")
		if !allowed {
			t.Error("expected Allow=true for first request within burst, got false")
		}
	})
}

// ── buildPayoutEncrypter ──────────────────────────────────────────────────────

func TestBuildPayoutEncrypter_EmptyKey_ReturnsNoop(t *testing.T) {
	enc := buildPayoutEncrypter("", zaptest.NewLogger(t))
	if enc.IsEnabled() {
		t.Error("expected Noop encrypter for empty key: IsEnabled must be false")
	}
}

func TestBuildPayoutEncrypter_ValidKey_ReturnsEnabledEncrypter(t *testing.T) {
	// 64 hex chars = 32 bytes = valid AES-256 key.
	validKey := strings.Repeat("ab", 32)
	enc := buildPayoutEncrypter(validKey, zaptest.NewLogger(t))
	if !enc.IsEnabled() {
		t.Error("expected AES-GCM encrypter for valid 32-byte key: IsEnabled must be true")
	}
}
