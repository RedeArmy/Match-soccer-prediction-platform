package middleware_test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
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
