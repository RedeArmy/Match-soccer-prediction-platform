package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
	metricnoop "go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func newIPLimiter(t *testing.T, limit int, windowSec int64) (*miniredis.Miniredis, *middleware.IPRateLimiter) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	l := middleware.NewIPRateLimiter(rc, clock.Real{}, zaptest.NewLogger(t), "ip_gl", limit, windowSec)
	return mr, l
}

func newIPLimiterWithClock(t *testing.T, limit int, windowSec int64, clk clock.Nower) (*miniredis.Miniredis, *middleware.IPRateLimiter) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	l := middleware.NewIPRateLimiter(rc, clk, zaptest.NewLogger(t), "ip_gl", limit, windowSec)
	return mr, l
}

func requestWithIP(ip string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	ctx := repository.ContextWithClientIP(r.Context(), ip)
	return r.WithContext(ctx)
}

// ── Allow: core token behaviour ───────────────────────────────────────────────

// TestIPRateLimiter_AllowsUpToLimit verifies that exactly `limit` requests from
// the same IP are permitted; the (limit+1)th is blocked.
func TestIPRateLimiter_AllowsUpToLimit(t *testing.T) {
	const limit = 5
	_, l := newIPLimiter(t, limit, 10)
	ctx := context.Background()

	for i := range limit {
		allowed, _ := l.Allow(ctx, "1.2.3.4")
		if !allowed {
			t.Fatalf("request %d/%d: expected allowed=true, got false", i+1, limit)
		}
	}

	allowed, _ := l.Allow(ctx, "1.2.3.4")
	if allowed {
		t.Fatal("request limit+1: expected allowed=false, got true")
	}
}

// TestIPRateLimiter_BlockedRequestHasRetryAfter verifies that a blocked Allow
// returns retryAfterSecs > 0.
func TestIPRateLimiter_BlockedRequestHasRetryAfter(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	ctx := context.Background()

	l.Allow(ctx, "1.2.3.4") //nolint:errcheck // exhaust the single slot

	_, retry := l.Allow(ctx, "1.2.3.4")
	if retry <= 0 {
		t.Errorf("expected retryAfterSecs > 0, got %d", retry)
	}
}

// TestIPRateLimiter_IndependentPerIP verifies that IP A and IP B have isolated
// counters: exhausting A does not affect B.
func TestIPRateLimiter_IndependentPerIP(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	ctx := context.Background()

	l.Allow(ctx, "10.0.0.1")               // exhaust IP A
	l.Allow(ctx, "10.0.0.1")               // blocked
	allowed, _ := l.Allow(ctx, "10.0.0.2") // IP B has its own bucket
	if !allowed {
		t.Error("IP B should be allowed even though IP A is exhausted")
	}
}

// TestIPRateLimiter_FailOpenOnRedisError verifies that a Redis failure causes
// Allow to return (true, 0) — fail-open — rather than blocking traffic.
func TestIPRateLimiter_FailOpenOnRedisError(t *testing.T) {
	mr, l := newIPLimiter(t, 3, 10)
	mr.Close() // force all Redis commands to fail

	allowed, retry := l.Allow(context.Background(), "1.2.3.4")
	if !allowed {
		t.Fatal("expected fail-open (allowed=true) on Redis error, got false")
	}
	if retry != 0 {
		t.Errorf("expected retryAfterSecs=0 on fail-open, got %d", retry)
	}
}

// TestIPRateLimiter_NilRedis_UsesFallback verifies that a nil Redis client
// falls through to the in-process LimiterStore without panicking.
func TestIPRateLimiter_NilRedis_UsesFallback(t *testing.T) {
	l := middleware.NewIPRateLimiter(nil, clock.Real{}, zaptest.NewLogger(t), "ip_gl", 3, 10)
	ctx := context.Background()

	for i := range 3 {
		allowed, _ := l.Allow(ctx, "9.9.9.9")
		if !allowed {
			t.Fatalf("fallback request %d: expected allowed=true", i+1)
		}
	}
}

// TestIPRateLimiter_WindowBoundary verifies that the counter resets when the
// fixed window rolls over. Uses clock.Frozen to advance time without sleeping.
func TestIPRateLimiter_WindowBoundary(t *testing.T) {
	const (
		limit     = 2
		windowSec = 10
	)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	frozen := &mutableClock{t: t0}

	_, l := newIPLimiterWithClock(t, limit, windowSec, frozen)
	ctx := context.Background()

	// Exhaust the window.
	l.Allow(ctx, "5.5.5.5")
	l.Allow(ctx, "5.5.5.5")
	blocked, _ := l.Allow(ctx, "5.5.5.5")
	if blocked {
		t.Fatal("expected blocked=false before window rolls; got true")
	}

	// Advance past the window boundary.
	frozen.advance(time.Duration(windowSec+1) * time.Second)

	// New window — counter resets.
	allowed, _ := l.Allow(ctx, "5.5.5.5")
	if !allowed {
		t.Fatal("expected allowed=true after window rolled over, got false")
	}
}

// mutableClock implements clock.Nower with an advanceable time value.
// Not safe for concurrent use — intended for single-goroutine tests only.
type mutableClock struct{ t time.Time }

func (c *mutableClock) Now() time.Time          { return c.t }
func (c *mutableClock) advance(d time.Duration) { c.t = c.t.Add(d) }

// ── Middleware HTTP wrapper ────────────────────────────────────────────────────

// TestIPRateLimiter_Middleware_AllowsAndSetsHeader verifies that requests within
// the limit pass through and X-RateLimit-Limit is present.
func TestIPRateLimiter_Middleware_AllowsAndSetsHeader(t *testing.T) {
	const limit = 5
	_, l := newIPLimiter(t, limit, 10)
	handler := l.Middleware("global")(passthroughHandler)

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, requestWithIP("2.3.4.5"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if got := w.Header().Get("X-RateLimit-Limit"); got != strconv.Itoa(limit) {
		t.Errorf("X-RateLimit-Limit: want %d, got %q", limit, got)
	}
}

// TestIPRateLimiter_Middleware_Returns429WhenLimitExceeded verifies that the
// (limit+1)th request receives HTTP 429, a Retry-After header, and the correct
// JSON error body.
func TestIPRateLimiter_Middleware_Returns429WhenLimitExceeded(t *testing.T) {
	_, l := newIPLimiter(t, 2, 10)
	handler := l.Middleware("global")(passthroughHandler)

	// Exhaust the limit.
	for range 2 {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, requestWithIP("3.4.5.6"))
	}

	// Next request must be throttled.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, requestWithIP("3.4.5.6"))

	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response, got empty string")
	}
	if w.Header().Get("X-RateLimit-Limit") == "" {
		t.Error("expected X-RateLimit-Limit header on 429 response")
	}
}

// TestIPRateLimiter_Middleware_NoIPInContext verifies that when no IP is in the
// request context the request passes through without consuming a token.
func TestIPRateLimiter_Middleware_NoIPInContext(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	handler := l.Middleware("global")(passthroughHandler)

	// Plain request with no IP in context.
	r := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 when no IP in context, got %d", w.Code)
	}
}

// TestIPRateLimiter_Configure verifies that Configure updates the limit so the
// new value is respected on subsequent Allow calls.
func TestIPRateLimiter_Configure_UpdatesLimit(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	ctx := context.Background()

	// Original limit = 1: first request allowed, second blocked.
	l.Allow(ctx, "7.7.7.7")
	allowed, _ := l.Allow(ctx, "7.7.7.7")
	if allowed {
		t.Fatal("expected blocked before Configure")
	}

	// Reconfigure with limit = 100 and a fresh Redis key (different IP).
	l.Configure(100, 10)
	for i := range 100 {
		a, _ := l.Allow(ctx, "8.8.8.8")
		if !a {
			t.Fatalf("request %d blocked after Configure(100)", i+1)
		}
	}
}

// ── RegisterMetrics ───────────────────────────────────────────────────────────

// TestIPRateLimiter_RegisterMetrics_Succeeds verifies that RegisterMetrics with
// a real SDK meter creates both OTel counters without error.
func TestIPRateLimiter_RegisterMetrics_Succeeds(t *testing.T) {
	_, l := newIPLimiter(t, 5, 10)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	if err := l.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}
}

// TestIPRateLimiter_RegisterMetrics_FailOpen_IncrementsCounter verifies that a
// Redis failure increments wcq_ip_rate_limit_fail_open_total exactly once.
func TestIPRateLimiter_RegisterMetrics_FailOpen_IncrementsCounter(t *testing.T) {
	mr, l := newIPLimiter(t, 5, 10)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := l.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	mr.Close()
	l.Allow(context.Background(), "1.1.1.1")

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	total := findInt64SumValue(t, rm, "wcq_ip_rate_limit_fail_open_total")
	if total != 1 {
		t.Errorf("expected wcq_ip_rate_limit_fail_open_total=1, got %d", total)
	}
}

// TestIPRateLimiter_Middleware_BlockedTotal_IncrementedWithLayer verifies that
// a blocked request increments wcq_ip_rate_limit_blocked_total with the correct
// layer attribute.
func TestIPRateLimiter_Middleware_BlockedTotal_IncrementedWithLayer(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)

	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	if err := l.RegisterMetrics(mp.Meter("test")); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	handler := l.Middleware("global")(passthroughHandler)

	// First request exhausts the limit.
	handler.ServeHTTP(httptest.NewRecorder(), requestWithIP("4.5.6.7"))
	// Second request is blocked — should increment the counter.
	handler.ServeHTTP(httptest.NewRecorder(), requestWithIP("4.5.6.7"))

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	total := findInt64SumValue(t, rm, "wcq_ip_rate_limit_blocked_total")
	if total != 1 {
		t.Errorf("expected wcq_ip_rate_limit_blocked_total=1, got %d", total)
	}
}

// ── safeIPPrefix ──────────────────────────────────────────────────────────────

// TestSafeIPPrefix_IPv4_RedactsLastTwoOctets is a white-box test of the
// unexported safeIPPrefix helper via the Middleware log output (tested
// indirectly through Allow which returns without panicking).
// The actual redaction format is verified via a package-internal test in
// rate_limit_internal_test.go.
func TestSafeIPPrefix_DoesNotPanicOnVariousInputs(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	ctx := context.Background()

	// None of these should panic regardless of IP format.
	for _, ip := range []string{
		"203.0.113.42", // valid IPv4
		"::1",          // IPv6 loopback
		"2001:db8::1",  // IPv6
		"not-an-ip",    // malformed
		"",             // empty (handled by Middleware, not Allow directly)
	} {
		if ip != "" {
			l.Allow(ctx, ip) //nolint:errcheck // testing panic safety, not correctness
		}
	}
}

// TestIPRateLimiter_Middleware_BlockedIPv6_RedactsIP exercises the
// "[ipv6-redacted]" branch of safeIPPrefix. Exhausting the limit from an IPv6
// address forces the blocked-request Warn log to call safeIPPrefix with a
// non-IPv4 string, covering the return "[ipv6-redacted]" branch.
func TestIPRateLimiter_Middleware_BlockedIPv6_RedactsIP(t *testing.T) {
	_, l := newIPLimiter(t, 1, 10)
	handler := l.Middleware("global")(passthroughHandler)

	// First request exhausts the single-slot window for this IPv6 address.
	handler.ServeHTTP(httptest.NewRecorder(), requestWithIP("2001:db8::1"))
	// Second request is blocked; safeIPPrefix is called with the IPv6 address
	// inside the Warn log, covering the "[ipv6-redacted]" branch.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, requestWithIP("2001:db8::1"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 for blocked IPv6 request, got %d", w.Code)
	}
}

// ── RegisterMetrics error paths ───────────────────────────────────────────────

// succeedOnceCounterMeter returns a valid noop counter on its first
// Int64Counter call and errCounterFailed on every subsequent call. Used to
// cover the second-counter error branch in IPRateLimiter.RegisterMetrics
// without failing the first counter registration.
type succeedOnceCounterMeter struct {
	metricnoop.Meter
	called int
}

func (m *succeedOnceCounterMeter) Int64Counter(_ string, _ ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	m.called++
	if m.called == 1 {
		return metricnoop.Int64Counter{}, nil
	}
	return nil, errCounterFailed
}

// TestIPRateLimiter_RegisterMetrics_FirstCounterError verifies that a meter
// failure on the first Int64Counter call causes RegisterMetrics to return an
// error (wcq_ip_rate_limit_blocked_total registration fails).
func TestIPRateLimiter_RegisterMetrics_FirstCounterError(t *testing.T) {
	_, l := newIPLimiter(t, 5, 10)
	if err := l.RegisterMetrics(failCounterMeter{}); err == nil {
		t.Fatal("expected error when first counter registration fails, got nil")
	}
}

// TestIPRateLimiter_RegisterMetrics_SecondCounterError verifies that a meter
// failure on the second Int64Counter call (wcq_ip_rate_limit_fail_open_total)
// causes RegisterMetrics to return an error even when the first counter
// registered successfully.
func TestIPRateLimiter_RegisterMetrics_SecondCounterError(t *testing.T) {
	_, l := newIPLimiter(t, 5, 10)
	if err := l.RegisterMetrics(&succeedOnceCounterMeter{}); err == nil {
		t.Fatal("expected error when second counter registration fails, got nil")
	}
}
