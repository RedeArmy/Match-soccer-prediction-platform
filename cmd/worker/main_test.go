package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

const (
	checkerDB = "db"
	statusOK  = "ok"
	statusErr = "error"
)

// ── stub health checkers ──────────────────────────────────────────────────────

type okChecker struct{ name string }

func (c *okChecker) Name() string { return c.name }
func (c *okChecker) Check(_ context.Context) health.Result {
	return health.Result{Status: statusOK, LatencyMS: 1}
}

type failChecker struct{ name string }

func (c *failChecker) Name() string { return c.name }
func (c *failChecker) Check(_ context.Context) health.Result {
	return health.Result{Status: statusErr, Error: "connection refused"}
}

// ── handleLiveness ────────────────────────────────────────────────────────────

func TestHandleLiveness_Returns200(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, pathHealth, nil)
	w := httptest.NewRecorder()
	handleLiveness(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleLiveness_ResponseBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, pathHealth, nil)
	w := httptest.NewRecorder()
	handleLiveness(w, req)

	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["status"] != statusOK {
		t.Errorf("expected status=ok, got %q", body["status"])
	}
	if body["service"] != "world-cup-quiniela-worker" {
		t.Errorf("expected service=world-cup-quiniela-worker, got %q", body["service"])
	}
}

func TestHandleLiveness_ContentTypeJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, pathHealth, nil)
	w := httptest.NewRecorder()
	handleLiveness(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type=application/json, got %q", ct)
	}
}

// ── handleReadiness ───────────────────────────────────────────────────────────

func TestHandleReadiness_AllCheckersOK_Returns200(t *testing.T) {
	checkers := []health.Checker{&okChecker{checkerDB}, &okChecker{driverRedis}}
	h := health.ReadinessHandler(checkers)

	req := httptest.NewRequest(http.MethodGet, pathHealthReady, nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleReadiness_AllCheckersOK_BodyStatusOK(t *testing.T) {
	checkers := []health.Checker{&okChecker{checkerDB}}
	h := health.ReadinessHandler(checkers)

	req := httptest.NewRequest(http.MethodGet, pathHealthReady, nil)
	w := httptest.NewRecorder()
	h(w, req)

	var resp health.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != statusOK {
		t.Errorf("expected status=ok, got %q", resp.Status)
	}
	if resp.Checks[checkerDB].Status != statusOK {
		t.Errorf("expected db check=ok, got %q", resp.Checks[checkerDB].Status)
	}
}

func TestHandleReadiness_CheckerFails_Returns503(t *testing.T) {
	checkers := []health.Checker{&okChecker{checkerDB}, &failChecker{driverRedis}}
	h := health.ReadinessHandler(checkers)

	req := httptest.NewRequest(http.MethodGet, pathHealthReady, nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleReadiness_CheckerFails_BodyStatusError(t *testing.T) {
	checkers := []health.Checker{&failChecker{driverRedis}}
	h := health.ReadinessHandler(checkers)

	req := httptest.NewRequest(http.MethodGet, pathHealthReady, nil)
	w := httptest.NewRecorder()
	h(w, req)

	var resp health.Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Status != statusErr {
		t.Errorf("expected status=error, got %q", resp.Status)
	}
}

func TestHandleReadiness_NoCheckers_Returns200(t *testing.T) {
	h := health.ReadinessHandler(nil)

	req := httptest.NewRequest(http.MethodGet, pathHealthReady, nil)
	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with no checkers, got %d", w.Code)
	}
}

// ── newHealthServer ───────────────────────────────────────────────────────────

func TestNewHealthServer_HasCorrectAddr(t *testing.T) {
	srv := newHealthServer("8081", nil, zap.NewNop())
	if srv.Addr != ":8081" {
		t.Errorf("expected Addr=:8081, got %q", srv.Addr)
	}
}

func TestNewHealthServer_RegistersRoutes(t *testing.T) {
	srv := newHealthServer("0", []health.Checker{&okChecker{checkerDB}}, zap.NewNop())

	cases := []struct {
		path string
		want int
	}{
		{pathHealth, http.StatusOK},
		{pathHealthReady, http.StatusOK},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		w := httptest.NewRecorder()
		srv.Handler.ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("GET %s: expected %d, got %d", tc.path, tc.want, w.Code)
		}
	}
}

// ── run ───────────────────────────────────────────────────────────────────────

func TestRun_EventBusUnreachable_ReturnsError(t *testing.T) {
	// driver passes the "redis" check, but the Dial to port 1 is immediately
	// refused by the OS — covering the setupEventBus error branch inside run().
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: driverRedis},
		Redis:    config.RedisConfig{Addr: "localhost:1"},
	}
	err := run(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for unreachable Redis, got nil")
	}
	if !strings.Contains(err.Error(), "event bus") {
		t.Errorf("expected error to mention event bus, got: %v", err)
	}
}

func TestRun_WrongEventBusDriver_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: "in_memory"},
	}
	err := run(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for wrong driver, got nil")
	}
	if !strings.Contains(err.Error(), driverRedis) {
		t.Errorf("expected error to mention %s, got: %v", driverRedis, err)
	}
}

func TestRun_EmptyDriver_ReturnsError(t *testing.T) {
	cfg := &config.Config{
		EventBus: config.EventBusConfig{Driver: ""},
	}
	err := run(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for empty driver, got nil")
	}
}

func TestRun_InvalidDSN_ReturnsError(t *testing.T) {
	// The driver check passes (redis), event bus succeeds (miniredis), then
	// setupDB fails on an invalid DSN. This exercises the full event bus
	// setup path inside run() without requiring a real Postgres instance.
	cfg := redisOnlyCfg(t)
	err := run(context.Background(), cfg, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for invalid DSN, got nil")
	}
	if !strings.Contains(err.Error(), "database") {
		t.Errorf("expected error to mention database, got: %v", err)
	}
}

// ── startWorker ───────────────────────────────────────────────────────────────

func TestStartWorker_ImmediateShutdown_ReturnsNil(t *testing.T) {
	// A pre-cancelled context causes startWorker to exit immediately after
	// setting up subscribers and the health server — no real I/O required.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{Worker: config.WorkerConfig{HealthPort: "0"}}
	bus := messaging.NewInMemoryBus(zap.NewNop())
	scorer := &stubScorer{}

	// Pass nil Redis client: monitorDLQ exits immediately when rc is nil,
	// keeping the test free of Redis dependencies.
	if err := startWorker(ctx, cfg, bus, scorer, nil, nil, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartWorker_SubscribesBeforeShutdown(t *testing.T) {
	// Using a pre-cancelled context makes startWorker run synchronously:
	// it calls bus.Subscribe, creates the health server, starts its goroutine,
	// then immediately returns when <-ctx.Done() fires. After startWorker
	// returns, the subscriber is guaranteed to be registered on the bus so
	// a subsequent synchronous Publish call can verify it was wired correctly.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{Worker: config.WorkerConfig{HealthPort: "0"}}
	log := zap.NewNop()
	bus := messaging.NewInMemoryBus(log)
	scorer := &stubScorer{}

	if err := startWorker(ctx, cfg, bus, scorer, nil, nil, log); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// InMemoryBus delivers synchronously; by the time Publish returns the
	// handler has already been invoked.
	_ = bus.Publish(context.Background(), events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload:    events.MatchFinished{MatchID: 7},
	})

	if scorer.lastID != 7 {
		t.Errorf("expected subscriber registered for match 7, got id=%d", scorer.lastID)
	}
}
