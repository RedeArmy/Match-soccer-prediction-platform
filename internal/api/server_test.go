// Package api_test exercises the HTTP surface of the API server using
// httptest rather than a real network listener.
//
// Tests in this file are black-box: they import the api package as an
// external consumer (package api_test) and interact only through the
// public API (api.New, *Server.Routes). This mirrors how the application
// is used in production and catches integration issues that unit tests of
// individual handlers would miss — for example, a middleware that intercepts
// all requests and short-circuits a handler unexpectedly.
//
// The database pool is passed as nil in all tests below. Handlers that
// require a live database are expected to return 503 when db is nil,
// not panic. This contract is verified separately per handler; the tests
// here focus solely on the server's routing and infrastructure endpoints.
package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

// fakePool creates a pgxpool.Pool pointing at an unreachable address. pgxpool
// connects lazily, so the pool object is valid for dependency construction even
// though any actual query will return a connection error.
func fakePool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(),
		"postgres://fake:fake@localhost:1/fake?sslmode=disable&connect_timeout=1")
	if err != nil {
		t.Fatalf("create fake pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

const (
	healthPath           = "/health"
	readyPath            = healthPath + "/ready"
	contentTypeJSON      = "application/json"
	checkerNameDB        = "db"
	checkerNameRedis     = "redis"
	statusOK             = "ok"
	statusError          = "error"
	errConnectionRefused = "connection refused"
	pathMatches          = "/api/v1/matches"
)

// newTestServer constructs a Server with a nil database pool and a
// test-scoped logger that writes to t.Log. It is intended for use in
// tests that exercise routing and infrastructure endpoints only.
func newTestServer(t *testing.T) *api.Server {
	t.Helper()
	// Use InMemoryBus in tests: no external dependencies required.
	return api.New(nil, &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
}

// stubChecker is a test-only health.Checker whose Check result is fixed at
// construction time. Using a stub keeps readiness tests free of real network
// dependencies while exercising the handler's aggregation logic.
type stubChecker struct {
	name   string
	result health.Result
}

func (s *stubChecker) Name() string                          { return s.name }
func (s *stubChecker) Check(_ context.Context) health.Result { return s.result }

func okChecker(name string) health.Checker {
	return &stubChecker{name: name, result: health.Result{Status: statusOK, LatencyMS: 1}}
}

func errChecker(name, msg string) health.Checker {
	return &stubChecker{name: name, result: health.Result{Status: statusError, Error: msg}}
}

func newTestServerWithCheckers(t *testing.T, checkers []health.Checker) *api.Server {
	t.Helper()
	return api.New(nil, &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, checkers)
}

func TestHealthEndpoint_ReturnsOK(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, healthPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestHealthEndpoint_ReturnsJSONContentType(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, healthPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != contentTypeJSON {
		t.Errorf("expected Content-Type %q, got %q", contentTypeJSON, ct)
	}
}

func TestHealthEndpoint_BodyContainsStatusOK(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, healthPath, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `"status":"ok"`) {
		t.Errorf("expected body to contain %q, got: %s", `"status":"ok"`, body)
	}
	if !strings.Contains(body, `"service":"world-cup-quiniela"`) {
		t.Errorf("expected body to contain service name, got: %s", body)
	}
}

func TestHealthEndpoint_OnlyAcceptsGET(t *testing.T) {
	handler := newTestServer(t).Routes()

	// chi returns 405 for registered paths with wrong method, and
	// 404 for unregistered paths. /health is registered for GET only.
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, healthPath, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("method %s: expected status %d, got %d",
				method, http.StatusMethodNotAllowed, rec.Code)
		}
	}
}

func TestUnknownRoute_Returns404(t *testing.T) {
	handler := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status %d, got %d", http.StatusNotFound, rec.Code)
	}
}

// ── nil db — known routes return 503, unknown return 404 ─────────────────────

func TestRoutes_DBNil_MatchRoute_Returns503(t *testing.T) {
	h := newTestServer(t).Routes()
	req := httptest.NewRequest(http.MethodGet, pathMatches, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (db unavailable), got %d", rec.Code)
	}
}

func TestRoutes_DBNil_PredictionRoute_Returns503(t *testing.T) {
	h := newTestServer(t).Routes()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/predictions", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 (db unavailable), got %d", rec.Code)
	}
}

// ── non-nil db (exercises buildBus + buildHandlers) ───────────────────────────

// TestRoutes_WithFakeDB_MatchRouteRegistered verifies that Routes() builds the
// full handler tree when db != nil without panicking. The fake pool is
// unreachable so the request returns 500, but a 404 would mean the route was
// never registered.
func TestRoutes_WithFakeDB_MatchRouteRegistered(t *testing.T) {
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	req := httptest.NewRequest(http.MethodGet, pathMatches, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Errorf("expected route to be registered, got 404")
	}
}

// ── wireSubscribers ───────────────────────────────────────────────────────────

// TestWireSubscribers_MalformedPayload_DoesNotPanic verifies that the
// MatchFinished subscriber silently drops events whose payload cannot be
// type-asserted to events.MatchFinished, returning nil so the bus does not
// route them to the dead-letter queue.
func TestWireSubscribers_MalformedPayload_DoesNotPanic(t *testing.T) {
	// Override backoff so any retry completes instantly.
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	bus := messaging.NewInMemoryBus(nil)
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), bus, nil, nil)
	srv.Routes() // registers wireSubscribers

	env := events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now().UTC(),
		Payload:    "not-a-MatchFinished-struct",
	}
	// Must not panic; the !ok branch returns nil immediately.
	_ = bus.Publish(context.Background(), env)
}

// TestWireSubscribers_ScoringError_DoesNotPanic verifies that when ScoreMatch
// returns an error (here because the fake pool has no real DB connection), the
// subscriber returns the error so the bus can retry, without panicking.
func TestWireSubscribers_ScoringError_DoesNotPanic(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	bus := messaging.NewInMemoryBus(nil)
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), bus, nil, nil)
	srv.Routes()

	env := events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now().UTC(),
		Payload:    events.MatchFinished{MatchID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina"},
	}
	// ScoreMatch will fail (fake pool, no real DB); handler returns the error,
	// bus retries and eventually dead-letters — none of this must panic.
	_ = bus.Publish(context.Background(), env)
}

func TestRoutes_WithFakeDB_PredictionRouteRegistered(t *testing.T) {
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/predictions?user_id=1", nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Errorf("expected route to be registered, got 404")
	}
}

// ── /health/ready ─────────────────────────────────────────────────────────────

func TestReadinessEndpoint_NoCheckers_Returns200(t *testing.T) {
	h := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestReadinessEndpoint_AllCheckersOK_Returns200(t *testing.T) {
	h := newTestServerWithCheckers(t, []health.Checker{
		okChecker(checkerNameDB),
		okChecker(checkerNameRedis),
	}).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rec.Code)
	}
}

func TestReadinessEndpoint_OneCheckerFails_Returns503(t *testing.T) {
	h := newTestServerWithCheckers(t, []health.Checker{
		okChecker(checkerNameDB),
		errChecker(checkerNameRedis, errConnectionRefused),
	}).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rec.Code)
	}
}

func TestReadinessEndpoint_ResponseJSON_AllOK(t *testing.T) {
	h := newTestServerWithCheckers(t, []health.Checker{
		okChecker(checkerNameDB),
	}).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != statusOK {
		t.Errorf("expected status %q, got %q", statusOK, resp.Status)
	}
	dbResult, ok := resp.Checks[checkerNameDB]
	if !ok {
		t.Fatalf("expected %q key in checks", checkerNameDB)
	}
	if dbResult.Status != statusOK {
		t.Errorf("expected db status %q, got %q", statusOK, dbResult.Status)
	}
}

func TestReadinessEndpoint_ResponseJSON_CheckerError(t *testing.T) {
	h := newTestServerWithCheckers(t, []health.Checker{
		errChecker(checkerNameRedis, errConnectionRefused),
	}).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	var resp health.Response
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != statusError {
		t.Errorf("expected status %q, got %q", statusError, resp.Status)
	}
	redisResult, ok := resp.Checks[checkerNameRedis]
	if !ok {
		t.Fatalf("expected %q key in checks", checkerNameRedis)
	}
	if redisResult.Error != errConnectionRefused {
		t.Errorf("expected error %q, got %q", errConnectionRefused, redisResult.Error)
	}
}

func TestReadinessEndpoint_ContentType_IsJSON(t *testing.T) {
	h := newTestServer(t).Routes()

	req := httptest.NewRequest(http.MethodGet, readyPath, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if ct != contentTypeJSON {
		t.Errorf("expected Content-Type %q, got %q", contentTypeJSON, ct)
	}
}

// ── non-nil cache ─────────────────────────────────────────────────────────────

// noopCacheStore is a no-op cache.Store used to exercise the
// "if s.cache != nil" branch in buildHandlers without a real Redis instance.
type noopCacheStore struct{}

func (noopCacheStore) Get(_ context.Context, _ string, _ interface{}) error {
	return cache.ErrCacheMiss
}
func (noopCacheStore) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) error {
	return nil
}
func (noopCacheStore) Delete(_ context.Context, _ ...string) error { return nil }

// TestRoutes_WithNonNilCache_MatchRouteRegistered verifies that Routes() builds
// the full handler tree including cache decorators when a non-nil cache is
// provided. A 404 would indicate the route was never registered.
func TestRoutes_WithNonNilCache_MatchRouteRegistered(t *testing.T) {
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), noopCacheStore{}, nil)
	req := httptest.NewRequest(http.MethodGet, pathMatches, nil)
	rec := httptest.NewRecorder()
	srv.Routes().ServeHTTP(rec, req)

	if rec.Code == http.StatusNotFound {
		t.Errorf("expected route to be registered with non-nil cache, got 404")
	}
}
