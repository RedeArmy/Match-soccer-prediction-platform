package middleware_test

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

func newRedisRateStore(t *testing.T, burst int) (*miniredis.Miniredis, *middleware.RedisRateStore) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	store := middleware.NewRedisRateStore(rc, 10.0, burst, zaptest.NewLogger(t))
	return mr, store
}

// TestRedisRateStore_WithinBurst_Allows verifies that requests up to the burst
// cap are accepted (allowed=true, retryAfter=0).
func TestRedisRateStore_WithinBurst_Allows(t *testing.T) {
	_, store := newRedisRateStore(t, 5)
	ctx := context.Background()

	for i := range 5 {
		allowed, retryAfter := store.Allow(ctx, "user:test")
		if !allowed {
			t.Fatalf("request %d: expected allowed=true, got false", i+1)
		}
		if retryAfter != 0 {
			t.Fatalf("request %d: expected retryAfter=0, got %d", i+1, retryAfter)
		}
	}
}

// TestRedisRateStore_ExceedBurst_Blocks verifies that the (burst+1)th request
// in the same second is rejected with allowed=false and retryAfter=1.
func TestRedisRateStore_ExceedBurst_Blocks(t *testing.T) {
	_, store := newRedisRateStore(t, 3)
	ctx := context.Background()

	for range 3 {
		store.Allow(ctx, "user:over") //nolint:errcheck
	}

	allowed, retryAfter := store.Allow(ctx, "user:over")
	if allowed {
		t.Fatal("expected allowed=false after burst exceeded, got true")
	}
	if retryAfter != 1 {
		t.Fatalf("expected retryAfter=1, got %d", retryAfter)
	}
}

// TestRedisRateStore_PerKey_IsolatesCounters verifies that two distinct keys
// have independent counters — draining one does not affect the other.
func TestRedisRateStore_PerKey_IsolatesCounters(t *testing.T) {
	_, store := newRedisRateStore(t, 1)
	ctx := context.Background()

	// Drain key "a".
	store.Allow(ctx, "user:a") //nolint:errcheck
	allowedA, _ := store.Allow(ctx, "user:a")
	if allowedA {
		t.Fatal("user:a should be blocked after burst exhausted")
	}

	// Key "b" must still have its own fresh window.
	allowedB, _ := store.Allow(ctx, "user:b")
	if !allowedB {
		t.Fatal("user:b should be allowed (independent counter)")
	}
}

// TestRedisRateStore_FailOpen_OnRedisError verifies that a Redis connectivity
// failure causes the store to fail open (allowed=true) rather than blocking traffic.
func TestRedisRateStore_FailOpen_OnRedisError(t *testing.T) {
	mr, store := newRedisRateStore(t, 5)
	// Close the server to force INCR failures.
	mr.Close()

	allowed, retryAfter := store.Allow(context.Background(), "user:err")
	if !allowed {
		t.Fatal("expected fail-open (allowed=true) on Redis error, got false")
	}
	if retryAfter != 0 {
		t.Fatalf("expected retryAfter=0 on fail-open, got %d", retryAfter)
	}
}

// ── RegisterMetrics ───────────────────────────────────────────────────────────

// TestRedisRateStore_RegisterMetrics_NoopMeter_ReturnsNil verifies that calling
// RegisterMetrics with a noop meter succeeds and does not panic.
func TestRedisRateStore_RegisterMetrics_NoopMeter_ReturnsNil(t *testing.T) {
	_, store := newRedisRateStore(t, 5)
	meter := metricnoop.NewMeterProvider().Meter("test")
	if err := store.RegisterMetrics(meter); err != nil {
		t.Fatalf("RegisterMetrics with noop meter: %v", err)
	}
}

// TestRedisRateStore_RegisterMetrics_RealMeter_RegistersCounter verifies that
// RegisterMetrics with a real SDK meter creates the wcq_rate_limit_fail_open_total
// counter without error.
func TestRedisRateStore_RegisterMetrics_RealMeter_RegistersCounter(t *testing.T) {
	_, store := newRedisRateStore(t, 5)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	if err := store.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

// TestRedisRateStore_FailOpen_IncrementsCounter verifies that a Redis INCR
// failure increments the wcq_rate_limit_fail_open_total counter exactly once.
func TestRedisRateStore_FailOpen_IncrementsCounter(t *testing.T) {
	mr, store := newRedisRateStore(t, 5)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := store.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	// Force INCR failures by closing the Redis server.
	mr.Close()

	allowed, _ := store.Allow(context.Background(), "user:counter")
	if !allowed {
		t.Fatal("expected fail-open (allowed=true) on Redis error")
	}

	// Collect metrics and verify the counter was incremented.
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	total := findInt64SumValue(t, rm, "wcq_rate_limit_fail_open_total")
	if total != 1 {
		t.Errorf("expected wcq_rate_limit_fail_open_total=1, got %d", total)
	}
}

// TestRedisRateStore_RegisterMetrics_MeterError_ReturnsWrappedError verifies that
// when the meter's Int64Counter registration fails, RegisterMetrics returns an
// error that wraps the underlying cause.
func TestRedisRateStore_RegisterMetrics_MeterError_ReturnsWrappedError(t *testing.T) {
	_, store := newRedisRateStore(t, 5)
	m := &failCounterMeter{} // zero-value noop.Meter is a valid embed base
	err := store.RegisterMetrics(m)
	if err == nil {
		t.Fatal("expected error from RegisterMetrics when counter creation fails, got nil")
	}
	if !errors.Is(err, errCounterFailed) {
		t.Errorf("expected err to wrap errCounterFailed, got: %v", err)
	}
}

// failCounterMeter wraps a noop Meter and overrides Int64Counter to inject an
// error, allowing tests to exercise the RegisterMetrics error path.
type failCounterMeter struct {
	metricnoop.Meter
}

var errCounterFailed = errors.New("injected counter error")

func (failCounterMeter) Int64Counter(_ string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errCounterFailed
}

// findInt64SumValue returns the cumulative value of an Int64Counter (Sum) metric
// by name from a ResourceMetrics snapshot.
func findInt64SumValue(t *testing.T, rm metricdata.ResourceMetrics, name string) int64 {
	t.Helper()
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name != name {
				continue
			}
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || len(sum.DataPoints) == 0 {
				t.Errorf("metric %q found but has unexpected data type or no data points", name)
				return 0
			}
			var total int64
			for _, dp := range sum.DataPoints {
				total += dp.Value
			}
			return total
		}
	}
	t.Errorf("metric %q not found in collected metrics", name)
	return 0
}

// TestRedisRateStore_RateLimitByUserID_Integration verifies that RedisRateStore
// integrates correctly with the RateLimitByUserID middleware.
func TestRedisRateStore_RateLimitByUserID_Integration(t *testing.T) {
	_, store := newRedisRateStore(t, 1)
	handler := middleware.RateLimitByUserID(store, zaptest.NewLogger(t))(passthroughHandler)

	// First request within burst — should pass.
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, limiterRequest("user:mid"))
	if w1.Code != 200 {
		t.Fatalf("first request: expected 200, got %d", w1.Code)
	}

	// Second request exceeds burst — should be throttled.
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, limiterRequest("user:mid"))
	if w2.Code != 429 {
		t.Fatalf("second request: expected 429, got %d", w2.Code)
	}
}
