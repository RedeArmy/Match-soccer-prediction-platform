package middleware_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// stubIPAllower is a test double for middleware.IPAllower that always returns
// a fixed result.
type stubIPAllower struct {
	allowed    bool
	retryAfter int
}

func (s *stubIPAllower) Allow(_ context.Context, _ string) (bool, int) {
	return s.allowed, s.retryAfter
}

// ── NewIPRateLimiter ──────────────────────────────────────────────────────────

func TestNewIPRateLimiter_NilMeter_DoesNotPanic(t *testing.T) {
	global := &stubIPAllower{allowed: true}
	webhook := &stubIPAllower{allowed: true}
	// nil meter should not panic; the counter fields remain nil and are no-ops.
	limiter := middleware.NewIPRateLimiter(global, webhook, nil, zaptest.NewLogger(t))
	if limiter == nil {
		t.Fatal("NewIPRateLimiter returned nil")
	}
}

// ── Global ────────────────────────────────────────────────────────────────────

func TestIPRateLimiter_Global_AllowsRequestUnderLimit(t *testing.T) {
	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: true},
		&stubIPAllower{allowed: false}, // webhook store is stricter — not used on global path
		nil, zaptest.NewLogger(t),
	)

	var reached bool
	handler := limiter.Global()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/matches", nil)
	req.RemoteAddr = "1.2.3.4"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !reached {
		t.Error("expected handler to be reached on allowed request")
	}
}

func TestIPRateLimiter_Global_Blocks429WhenOverLimit(t *testing.T) {
	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: false, retryAfter: 1},
		&stubIPAllower{allowed: true},
		nil, zaptest.NewLogger(t),
	)

	handler := limiter.Global()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/predictions", nil)
	req.RemoteAddr = "5.6.7.8"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestIPRateLimiter_Global_PassesThroughWhenRemoteAddrEmpty(t *testing.T) {
	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: false}, // would block if IP were extracted
		&stubIPAllower{allowed: false},
		nil, zaptest.NewLogger(t),
	)

	var reached bool
	handler := limiter.Global()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	req.RemoteAddr = "" // empty → fail-open
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !reached {
		t.Error("expected handler to be reached when RemoteAddr is empty (fail-open)")
	}
}

// ── Webhook ───────────────────────────────────────────────────────────────────

func TestIPRateLimiter_Webhook_AllowsRequest(t *testing.T) {
	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: false}, // global store not used on webhook path
		&stubIPAllower{allowed: true},
		nil, zaptest.NewLogger(t),
	)

	var reached bool
	handler := limiter.Webhook()(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		reached = true
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", nil)
	req.RemoteAddr = "203.0.113.1"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !reached {
		t.Error("expected handler to be reached on allowed webhook request")
	}
}

func TestIPRateLimiter_Webhook_Blocks429WhenOverLimit(t *testing.T) {
	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: true},
		&stubIPAllower{allowed: false, retryAfter: 2},
		nil, zaptest.NewLogger(t),
	)

	handler := limiter.Webhook()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", nil)
	req.RemoteAddr = "9.8.7.6"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rec.Code)
	}
}

// ── OTel counter ──────────────────────────────────────────────────────────────

func TestIPRateLimiter_BlockedCounterIncremented(t *testing.T) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	meter := mp.Meter("test")

	limiter := middleware.NewIPRateLimiter(
		&stubIPAllower{allowed: false, retryAfter: 1},
		&stubIPAllower{allowed: false, retryAfter: 1},
		meter, zaptest.NewLogger(t),
	)

	handler := limiter.Global()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "10.0.0.1"
	handler.ServeHTTP(httptest.NewRecorder(), req)

	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	found := false
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == "wcq_ip_rate_limit_blocked_total" {
				found = true
			}
		}
	}
	if !found {
		t.Error("expected wcq_ip_rate_limit_blocked_total counter after blocked request")
	}
}
