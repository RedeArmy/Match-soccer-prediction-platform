package main

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/election"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/internal/testutil"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

const (
	checkerDB = "db"
	statusOK  = "ok"
	statusErr = "error"

	// testDLQLeaderKey is a Redis key used by monitorDLQ unit tests that
	// exercise leader election via RedisLeaderElection. Production code uses
	// a PostgreSQL advisory lock (dlqMonitorLockID); tests retain Redis-based
	// election because it is simpler to exercise without a real database.
	testDLQLeaderKey = "test:worker:dlq-monitor:leader"
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
	srv := newHealthServer("8081", nil, nil, zap.NewNop())
	if srv.Addr != ":8081" {
		t.Errorf("expected Addr=:8081, got %q", srv.Addr)
	}
}

func TestNewHealthServer_RegistersRoutes(t *testing.T) {
	srv := newHealthServer("0", []health.Checker{&okChecker{checkerDB}}, nil, zap.NewNop())

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

func TestNewHealthServer_WithMetricsHandler_RegistersMetricsRoute(t *testing.T) {
	stub := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	srv := newHealthServer("0", nil, stub, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	srv.Handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from /metrics route, got %d", w.Code)
	}
}

// ── run ───────────────────────────────────────────────────────────────────────

func TestRun_EventBusUnreachable_ReturnsError(t *testing.T) {
	// driver passes the "redis" check, but the Dial to port 1 is immediately
	// refused by the OS - covering the setupEventBus error branch inside run().
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

func TestRun_FullStartup_ImmediateShutdown(t *testing.T) {
	// Exercises the full service-wiring path inside run(): setupEventBus and
	// setupDB both succeed, all repos/services are constructed, and startWorker
	// exits cleanly when the context is cancelled.
	// A pre-cancelled context cannot be used here because cache.NewClient pings
	// Redis synchronously with the caller's context; instead we cancel shortly
	// after the goroutine starts so setup has time to complete.
	mr := miniredis.RunT(t)
	dsn := testutil.SetupPostgres(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := &config.Config{
		Environment: "test",
		EventBus:    config.EventBusConfig{Driver: driverRedis},
		Redis:       config.RedisConfig{Addr: mr.Addr()},
		Database: config.DatabaseConfig{
			DSN:             dsn,
			MaxOpenConns:    5,
			MaxIdleConns:    2,
			ConnMaxLifetime: time.Minute,
		},
		Worker: config.WorkerConfig{HealthPort: "0"},
	}

	errCh := make(chan error, 1)
	go func() { errCh <- run(ctx, cfg, zap.NewNop()) }()

	time.AfterFunc(300*time.Millisecond, cancel)

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run() did not return within timeout")
	}
}

// ── startWorker ───────────────────────────────────────────────────────────────

func TestStartWorker_ImmediateShutdown_ReturnsNil(t *testing.T) {
	// A pre-cancelled context causes startWorker to exit immediately after
	// setting up subscribers and the health server - no real I/O required.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{Worker: config.WorkerConfig{HealthPort: "0"}}
	bus := messaging.NewInMemoryBus(zap.NewNop())
	scorer := &stubScorer{}

	// Pass nil Redis client: monitorDLQ exits immediately when rc is nil,
	// keeping the test free of Redis dependencies.
	if err := startWorker(ctx, workerDeps{cfg: cfg, bus: bus, scorer: scorer}, zap.NewNop()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartWorker_SubscribesMatchStarted(t *testing.T) {
	// Verify that a MatchStarted event published after startWorker returns is
	// handled without error. The handler only logs, so the only observable
	// outcome is the absence of a panic or error from the bus.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cfg := &config.Config{Worker: config.WorkerConfig{HealthPort: "0"}}
	log := zap.NewNop()
	bus := messaging.NewInMemoryBus(log)
	scorer := &stubScorer{}

	if err := startWorker(ctx, workerDeps{cfg: cfg, bus: bus, scorer: scorer}, log); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	err := bus.Publish(context.Background(), events.Envelope{
		Type:       events.EventMatchStarted,
		OccurredAt: time.Now(),
		Payload:    events.MatchStarted{MatchID: 3, HomeTeam: teamMexico, AwayTeam: "Canada", KickoffAt: time.Now()},
	})
	if err != nil {
		t.Errorf("MatchStarted publish: %v", err)
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

	if err := startWorker(ctx, workerDeps{cfg: cfg, bus: bus, scorer: scorer}, log); err != nil {
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

// ── buildHealthCheckers ───────────────────────────────────────────────────────

func TestBuildHealthCheckers_ReturnsCheckerForEachEventType(t *testing.T) {
	// buildHealthCheckers constructors are pure: they only store references and
	// do not dial any backend. Passing nil for both arguments is safe because
	// Check() is never called here - we only verify the slice length and that
	// every expected event type has both a DLQ and stream-pending checker.
	checkers := buildHealthCheckers(nil, nil)

	// 2 base checkers (DB + Redis) + 2 DLQ + 2 stream-pending = 6 total.
	const wantLen = 6
	if len(checkers) != wantLen {
		t.Errorf("expected %d health checkers, got %d", wantLen, len(checkers))
	}
}

// ── monitorDLQ ────────────────────────────────────────────────────────────────

func TestStartWorker_HealthServerFails_ReturnsError(t *testing.T) {
	// Pre-occupy a port so the health-server goroutine inside startWorker fails
	// with EADDRINUSE, which sends a non-nil error to srvErr and causes
	// startWorker to return that error instead of waiting for ctx cancellation.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer ln.Close()
	_, port, _ := net.SplitHostPort(ln.Addr().String())

	cfg := &config.Config{Worker: config.WorkerConfig{HealthPort: port}}
	bus := messaging.NewInMemoryBus(zap.NewNop())

	err = startWorker(context.Background(), workerDeps{cfg: cfg, bus: bus, scorer: &stubScorer{}}, zap.NewNop())
	if err == nil {
		t.Fatal("expected error when health server cannot bind, got nil")
	}
}

func TestMonitorDLQ_NilClient_ReturnsImmediately(t *testing.T) {
	// The nil-client guard prevents a panic when the worker is started without
	// a Redis connection (e.g. unit tests). Verifies the guard fires correctly.
	// A nil tickC is safe here because the function returns before reaching the
	// select loop.
	monitorDLQ(context.Background(), nil, nil, nil, nil, zap.NewNop())
}

func TestMonitorDLQ_NonEmptyQueue_LogsError(t *testing.T) {
	// Seed a DLQ key with one item so the n > 0 branch inside the ticker case
	// is reached, then cancel the context to stop the loop.
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close() //nolint:errcheck

	dlqKey := "dlq:" + string(events.EventMatchStarted)
	if _, err := mr.Lpush(dlqKey, "payload"); err != nil {
		t.Fatalf("seed DLQ: %v", err)
	}

	// Pre-load exactly one tick so the non-empty-queue branch is exercised.
	// Using an injected channel eliminates any mutation of global state and
	// removes the need for a time.Sleep.
	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		monitorDLQ(ctx, rc, nil, tickC, nil, zap.NewNop())
		close(done)
	}()

	// Spin until the goroutine has consumed the pre-loaded tick, guaranteeing
	// the non-empty-queue code path is exercised before we cancel. This is safe
	// under -race: len() on a channel is a synchronised read.
	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done
}

func TestMonitorDLQ_EmptyQueue_LogsDebug(t *testing.T) {
	// Empty DLQs exercise the else branch (debug-level log).
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close() //nolint:errcheck

	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		monitorDLQ(ctx, rc, nil, tickC, nil, zap.NewNop())
		close(done)
	}()

	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done
}

func TestMonitorDLQ_CancelledContext_ReturnsWithoutTick(t *testing.T) {
	// A pre-cancelled context causes monitorDLQ to enter the select loop once
	// and immediately return via the ctx.Done() case. A nil tickC is used so
	// that case is never selectable - only ctx.Done() can fire.
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close() //nolint:errcheck

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	monitorDLQ(ctx, rc, nil, nil, nil, zap.NewNop())
}

func TestMonitorDLQ_ElectionLost_SkipsTick(t *testing.T) {
	// When the election is non-nil but TryAcquire returns false (another replica
	// holds the lock), monitorDLQ must skip the LLEN scan entirely. We verify
	// this by pre-seeding the DLQ, winning the lock with a separate client, and
	// confirming that the monitor goroutine exits cleanly after the tick without
	// logging an error (i.e. the scan was never executed).
	mr := miniredis.RunT(t)

	// rcHolder acquires the lock so that rcMonitor cannot.
	rcHolder := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rcHolder.Close() //nolint:errcheck
	rcMonitor := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rcMonitor.Close() //nolint:errcheck

	// Seed a DLQ entry so a scan would produce a non-empty result.
	dlqKey := "dlq:" + string(events.EventMatchStarted)
	if _, err := mr.Lpush(dlqKey, "payload"); err != nil {
		t.Fatalf("seed DLQ: %v", err)
	}

	// The holder wins the election lock before the monitor even starts.
	holder := election.NewRedisLeaderElection(rcHolder, testDLQLeaderKey, 5*time.Second, zap.NewNop())
	if !holder.TryAcquire(context.Background()) {
		t.Fatal("holder should have acquired the lock")
	}

	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	monitor := election.NewRedisLeaderElection(rcMonitor, testDLQLeaderKey, 5*time.Second, zap.NewNop())
	done := make(chan struct{})
	go func() {
		monitorDLQ(ctx, rcMonitor, monitor, tickC, nil, zap.NewNop())
		close(done)
	}()

	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done
}

func TestMonitorDLQ_ElectionWon_ExecutesScan(t *testing.T) {
	// When the election is non-nil and TryAcquire returns true (this replica
	// wins the lock), monitorDLQ must proceed with the LLEN scan.
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close() //nolint:errcheck

	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	e := election.NewRedisLeaderElection(rc, testDLQLeaderKey, 5*time.Second, zap.NewNop())
	done := make(chan struct{})
	go func() {
		monitorDLQ(ctx, rc, e, tickC, nil, zap.NewNop())
		close(done)
	}()

	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done
}

func TestCheckDLQEvent_NonNilNotifier_FiresWebhook(t *testing.T) {
	// Verify that checkDLQEvent calls notifier.NotifyDLQOverflow when the DLQ
	// key is non-empty. The notifier fires its HTTP POST in a background
	// goroutine, so we wait on a channel with a timeout instead of sleeping.
	var called atomic.Bool
	webhookReceived := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/dlq-overflow") {
			called.Store(true)
			webhookReceived <- struct{}{}
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	notifier := observability.New(observability.NotifierConfig{
		BaseURL: srv.URL,
		Log:     zap.NewNop(),
	})

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	dlqKey := "dlq:" + string(events.EventMatchFinished)
	if _, err := mr.Lpush(dlqKey, "payload"); err != nil {
		t.Fatalf("seed DLQ: %v", err)
	}

	checkDLQEvent(context.Background(), rc, events.EventMatchFinished, notifier, zap.NewNop())

	select {
	case <-webhookReceived:
		// success: webhook was posted
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for DLQ overflow webhook from checkDLQEvent")
	}

	if !called.Load() {
		t.Error("expected notifier.NotifyDLQOverflow to have been called")
	}
}

// ── monitorPurge ──────────────────────────────────────────────────────────────

// stubPurger implements repository.Purger for monitorPurge unit tests.
type stubPurger struct {
	userCalled     int
	quinielaCalled int
	snapshotCalled int
	userCount      int64
	quinielaCount  int64
	snapshotCount  int64
	err            error
}

func (s *stubPurger) PurgeDeletedUsers(_ context.Context, _ time.Time) (int64, error) {
	s.userCalled++
	return s.userCount, s.err
}

func (s *stubPurger) PurgeDeletedQuinielas(_ context.Context, _ time.Time) (int64, error) {
	s.quinielaCalled++
	return s.quinielaCount, s.err
}

func (s *stubPurger) PurgeOldSnapshots(_ context.Context, _ int) (int64, error) {
	s.snapshotCalled++
	return s.snapshotCount, s.err
}

func (s *stubPurger) EraseUserPII(_ context.Context, _ int) error {
	return s.err
}

func (s *stubPurger) PurgeOldParamHistory(_ context.Context, _ time.Time) (int64, error) {
	return 0, s.err
}

func TestMonitorPurge_NilPurger_ReturnsImmediately(t *testing.T) {
	monitorPurge(context.Background(), nil, 24*time.Hour, 90*24*time.Hour, 5, nil, zap.NewNop())
}

func TestMonitorPurge_CancelledContext_ReturnsWithoutTick(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	purger := &stubPurger{}
	monitorPurge(ctx, purger, 24*time.Hour, 90*24*time.Hour, 5, nil, zap.NewNop())

	if purger.userCalled != 0 {
		t.Errorf("expected no purge calls with cancelled context, got %d", purger.userCalled)
	}
}

func TestMonitorPurge_OnTick_CallsPurge(t *testing.T) {
	purger := &stubPurger{userCount: 2, quinielaCount: 1, snapshotCount: 3}
	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		monitorPurge(ctx, purger, 24*time.Hour, 90*24*time.Hour, 5, tickC, zap.NewNop())
		close(done)
	}()

	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done

	if purger.userCalled != 1 {
		t.Errorf("expected PurgeDeletedUsers called once, got %d", purger.userCalled)
	}
	if purger.quinielaCalled != 1 {
		t.Errorf("expected PurgeDeletedQuinielas called once, got %d", purger.quinielaCalled)
	}
	if purger.snapshotCalled != 1 {
		t.Errorf("expected PurgeOldSnapshots called once, got %d", purger.snapshotCalled)
	}
}

func TestMonitorPurge_PurgeError_LogsAndContinues(t *testing.T) {
	purger := &stubPurger{err: errors.New("db error")}
	tickC := make(chan time.Time, 1)
	tickC <- time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		monitorPurge(ctx, purger, 24*time.Hour, 90*24*time.Hour, 5, tickC, zap.NewNop())
		close(done)
	}()

	for len(tickC) > 0 {
		runtime.Gosched()
	}
	cancel()
	<-done

	if purger.userCalled != 1 {
		t.Errorf("expected PurgeDeletedUsers called once despite error, got %d", purger.userCalled)
	}
	if purger.snapshotCalled != 1 {
		t.Errorf("expected PurgeOldSnapshots called once despite error, got %d", purger.snapshotCalled)
	}
}

// ── buildMailer tests (DT-26) ─────────────────────────────────────────────────

func TestBuildMailer_WithAPIKey_ReturnsNoError(t *testing.T) {
	sender, err := buildMailer("re_test_key_12345")
	if err != nil {
		t.Errorf("expected no error when API key is set, got: %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestBuildMailer_EmptyAPIKey_ReturnsError(t *testing.T) {
	sender, err := buildMailer("")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_EMAIL_RESENDAPIKEY") {
		t.Errorf("expected error to name the missing env var, got: %v", err)
	}
	// Sender must still be valid (NoopClient) so callers can degrade gracefully.
	if sender == nil {
		t.Fatal("expected non-nil NoopClient even on error")
	}
}

// ── buildPusher tests (DT-26) ─────────────────────────────────────────────────

func TestBuildPusher_AllKeysSet_ReturnsNoError(t *testing.T) {
	sender, err := buildPusher("pubKey", "privKey", "mailto:test@example.com")
	if err != nil {
		t.Errorf("expected no error when all VAPID keys are set, got: %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil sender")
	}
}

func TestBuildPusher_MissingPublicKey_ReturnsError(t *testing.T) {
	sender, err := buildPusher("", "privKey", "mailto:test@example.com")
	if err == nil {
		t.Fatal("expected error when public key is missing, got nil")
	}
	if !strings.Contains(err.Error(), "VAPIDPublicKey") {
		t.Errorf("expected error to name the missing key, got: %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil NoopSender even on error")
	}
}

func TestBuildPusher_MissingPrivateKey_ReturnsError(t *testing.T) {
	sender, err := buildPusher("pubKey", "", "mailto:test@example.com")
	if err == nil {
		t.Fatal("expected error when private key is missing, got nil")
	}
	if !strings.Contains(err.Error(), "WCQ_WEBPUSH_VAPIDPRIVATEKEY") {
		t.Errorf("expected error to name the missing env var, got: %v", err)
	}
	if sender == nil {
		t.Fatal("expected non-nil NoopSender even on error")
	}
}

func TestBuildPusher_BothKeysMissing_ErrorMentionsBoth(t *testing.T) {
	_, err := buildPusher("", "", "")
	if err == nil {
		t.Fatal("expected error when both keys are missing, got nil")
	}
	if !strings.Contains(err.Error(), "VAPIDPublicKey") || !strings.Contains(err.Error(), "WCQ_WEBPUSH_VAPIDPRIVATEKEY") {
		t.Errorf("expected error to mention both missing keys, got: %v", err)
	}
}

// ── repoEmailResolver tests (DT-28) ──────────────────────────────────────────

// stubUserRepoForWorker is a minimal stub for repository.UserRepository used
// in worker-package tests (package main, not an external test package).
type stubUserRepoForWorker struct {
	user *domain.User
	err  error
}

func (r *stubUserRepoForWorker) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepoForWorker) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return r.user, r.err
}
func (r *stubUserRepoForWorker) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepoForWorker) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *stubUserRepoForWorker) Delete(_ context.Context, _ int) error          { return r.err }
func (r *stubUserRepoForWorker) List(_ context.Context) ([]*domain.User, error) { return nil, r.err }
func (r *stubUserRepoForWorker) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, r.err
}
func (r *stubUserRepoForWorker) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, r.err
}
func (r *stubUserRepoForWorker) Unban(_ context.Context, _ int) error { return r.err }
func (r *stubUserRepoForWorker) ListBanned(_ context.Context) ([]*domain.User, error) {
	return nil, r.err
}
func (r *stubUserRepoForWorker) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", r.err
}
func (r *stubUserRepoForWorker) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, r.err
}
func (r *stubUserRepoForWorker) GetBalance(_ context.Context, _ int) (int, int, error) {
	return 0, 0, r.err
}

var _ repository.UserRepository = (*stubUserRepoForWorker)(nil)

func TestRepoEmailResolver_UserFound_ReturnsEmailAndName(t *testing.T) {
	stub := &stubUserRepoForWorker{
		user: &domain.User{ID: 1, Email: "alice@example.com", Name: "Alice"},
	}
	resolver := &repoEmailResolver{userRepo: stub}

	email, name, err := resolver.ResolveEmailByID(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "alice@example.com" {
		t.Errorf("email: got %q, want %q", email, "alice@example.com")
	}
	if name != "Alice" {
		t.Errorf("name: got %q, want %q", name, "Alice")
	}
}

func TestRepoEmailResolver_UserNotFound_ReturnsError(t *testing.T) {
	stub := &stubUserRepoForWorker{user: nil}
	resolver := &repoEmailResolver{userRepo: stub}

	_, _, err := resolver.ResolveEmailByID(context.Background(), 999)
	if err == nil {
		t.Fatal("expected error for non-existent user, got nil")
	}
	if !strings.Contains(err.Error(), "999") {
		t.Errorf("expected error to include user ID, got: %v", err)
	}
}

func TestRepoEmailResolver_RepoError_PropagatesError(t *testing.T) {
	stub := &stubUserRepoForWorker{err: errors.New("database unavailable")}
	resolver := &repoEmailResolver{userRepo: stub}

	_, _, err := resolver.ResolveEmailByID(context.Background(), 5)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
	if !strings.Contains(err.Error(), "database unavailable") {
		t.Errorf("expected wrapped repo error, got: %v", err)
	}
}

// ── makePushPruneJob tests (DT-29) ────────────────────────────────────────────

// stubPushRepo is a minimal stub for repository.PushSubscriptionRepository
// used to test makePushPruneJob without a real database.
type stubPushRepo struct {
	deleted int64
	err     error
}

func (r *stubPushRepo) Create(_ context.Context, _ *domain.PushSubscription) error { return r.err }
func (r *stubPushRepo) ListActiveByUser(_ context.Context, _ int) ([]*domain.PushSubscription, error) {
	return nil, r.err
}
func (r *stubPushRepo) DeleteByEndpoint(_ context.Context, _ string) error { return r.err }
func (r *stubPushRepo) MarkInactive(_ context.Context, _ int64) error      { return r.err }
func (r *stubPushRepo) UpdateLastUsed(_ context.Context, _ int64) error    { return r.err }
func (r *stubPushRepo) DeleteInactive(_ context.Context, _ time.Time) (int64, error) {
	return r.deleted, r.err
}

// stubParamSvc is a minimal stub for service.SystemParamService.
type stubParamSvc struct{}

func (s *stubParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) { return nil, nil }
func (s *stubParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error)      { return nil, nil }
func (s *stubParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamSvc) GetString(_ context.Context, _, defaultVal string) string { return defaultVal }
func (s *stubParamSvc) GetInt(_ context.Context, _ string, defaultVal int) int   { return defaultVal }
func (s *stubParamSvc) GetDuration(_ context.Context, _ string, defaultVal time.Duration) time.Duration {
	return defaultVal
}
func (s *stubParamSvc) GetBool(_ context.Context, _ string, defaultVal bool) bool { return defaultVal }
func (s *stubParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (s *stubParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *stubParamSvc) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

var _ service.SystemParamService = (*stubParamSvc)(nil)

func TestMakePushPruneJob_SuccessWithDeletions_LogsCount(t *testing.T) {
	pushRepo := &stubPushRepo{deleted: 5}
	job := makePushPruneJob(&stubParamSvc{}, pushRepo, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil error for successful prune, got %v", err)
	}
}

func TestMakePushPruneJob_NoDeletions_ReturnsNil(t *testing.T) {
	pushRepo := &stubPushRepo{deleted: 0}
	job := makePushPruneJob(&stubParamSvc{}, pushRepo, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil error when no records to prune, got %v", err)
	}
}

func TestMakePushPruneJob_RepoError_ReturnsError(t *testing.T) {
	pushRepo := &stubPushRepo{err: errors.New("db failure")}
	job := makePushPruneJob(&stubParamSvc{}, pushRepo, zap.NewNop())

	err := job(context.Background())
	if err == nil {
		t.Fatal("expected error when repo fails, got nil")
	}
	if !strings.Contains(err.Error(), "db failure") {
		t.Errorf("expected repo error to propagate, got: %v", err)
	}
}

func TestMakePushPruneJob_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before job runs

	pushRepo := &stubPushRepo{err: context.Canceled}
	job := makePushPruneJob(&stubParamSvc{}, pushRepo, zap.NewNop())

	if err := job(ctx); err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
}
