package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// resetGlobalDailyFixtureSyncState clears the idempotency guard so tests start
// from a clean slate regardless of execution order.
func resetGlobalDailyFixtureSyncState() {
	globalDailyFixtureSyncState.lastSuccessDate.Store("")
}

// noopNotifier returns a disabled Notifier (baseURL is empty → all methods are
// no-ops). Safe to use wherever the test does not need to verify HTTP calls.
func noopNotifier() *observability.Notifier {
	return observability.New(observability.NotifierConfig{Log: zap.NewNop()})
}

// stubDailySyncer wraps stubWorkerMatchSyncer but lets us override
// DailyFixtureSync independently.
type stubDailySyncer struct {
	stubWorkerMatchSyncer
	dailyResult *service.DailySyncResult
	dailyErr    error
}

func (s *stubDailySyncer) DailyFixtureSync(_ context.Context, _, _ *time.Time) (*service.DailySyncResult, error) {
	if s.dailyErr != nil {
		return nil, s.dailyErr
	}
	if s.dailyResult != nil {
		return s.dailyResult, nil
	}
	return &service.DailySyncResult{}, nil
}

var _ service.MatchSyncer = (*stubDailySyncer)(nil)

// ── makeDailyFixtureSyncJob ───────────────────────────────────────────────────

func TestMakeDailyFixtureSyncJob_FirstRun_StoresDate(t *testing.T) {
	resetGlobalDailyFixtureSyncState()

	syncer := &stubDailySyncer{dailyResult: &service.DailySyncResult{Total: 5, Updated: 2}}
	job := makeDailyFixtureSyncJob(&configParamSvc{}, syncer, noopNotifier(), zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stored, _ := globalDailyFixtureSyncState.lastSuccessDate.Load().(string)
	if stored == "" {
		t.Error("expected lastSuccessDate to be stored after successful sync")
	}
	// Value must be in YYYY-MM-DD format.
	if len(stored) != 10 || stored[4] != '-' || stored[7] != '-' {
		t.Errorf("stored date format invalid: %q", stored)
	}
}

func TestMakeDailyFixtureSyncJob_IdempotencyGuard_SkipsOnSameDate(t *testing.T) {
	resetGlobalDailyFixtureSyncState()

	// Seed today's date so the guard fires immediately.
	loc, _ := time.LoadLocation("America/Guatemala")
	today := time.Now().In(loc).Format("2006-01-02")
	globalDailyFixtureSyncState.lastSuccessDate.Store(today)

	callCount := 0
	syncer := &stubDailySyncer{}
	// Override DailyFixtureSync to track calls.
	trackingSyncer := &trackingDailySyncer{inner: syncer, count: &callCount}
	job := makeDailyFixtureSyncJob(&configParamSvc{}, trackingSyncer, noopNotifier(), zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 0 {
		t.Errorf("DailyFixtureSync should not be called when today is already stored, got %d calls", callCount)
	}
}

func TestMakeDailyFixtureSyncJob_SyncError_ReturnsNilAndNotifies(t *testing.T) {
	resetGlobalDailyFixtureSyncState()

	var webhookCalled atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/daily-fixture-sync-failed") {
			webhookCalled.Store(true)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	notifier := observability.New(observability.NotifierConfig{
		BaseURL: srv.URL,
		Log:     zap.NewNop(),
	})

	syncer := &stubDailySyncer{dailyErr: errors.New("provider unavailable")}
	job := makeDailyFixtureSyncJob(&configParamSvc{}, syncer, notifier, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Fatalf("expected nil on sync error, got %v", err)
	}

	// Wait for the fire() goroutine inside the notifier to complete.
	notifier.Close()

	if !webhookCalled.Load() {
		t.Error("expected NotifyDailyFixtureSyncFailed webhook to be posted")
	}

	// lastSuccessDate must NOT be stored on failure.
	stored, _ := globalDailyFixtureSyncState.lastSuccessDate.Load().(string)
	if stored != "" {
		t.Errorf("lastSuccessDate should not be stored on failure, got %q", stored)
	}
}

func TestMakeDailyFixtureSyncJob_InvalidTimezone_FallsBackToUTC(t *testing.T) {
	resetGlobalDailyFixtureSyncState()

	params := &configParamSvc{
		strings: map[string]string{"notify.scheduler_timezone": "Invalid/Zone"},
	}
	syncer := &stubDailySyncer{dailyResult: &service.DailySyncResult{}}
	job := makeDailyFixtureSyncJob(params, syncer, noopNotifier(), zap.NewNop())

	// Must not panic and must succeed.
	if err := job(context.Background()); err != nil {
		t.Fatalf("unexpected error with invalid timezone: %v", err)
	}

	stored, _ := globalDailyFixtureSyncState.lastSuccessDate.Load().(string)
	if stored == "" {
		t.Error("expected lastSuccessDate to be stored even with fallback UTC timezone")
	}
}

func TestMakeDailyFixtureSyncJob_SyncSuccess_ReturnsNil(t *testing.T) {
	resetGlobalDailyFixtureSyncState()

	syncer := &stubDailySyncer{dailyResult: &service.DailySyncResult{Total: 10, Linked: 3, Updated: 2, Corrected: 1}}
	job := makeDailyFixtureSyncJob(&configParamSvc{}, syncer, noopNotifier(), zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil on success, got %v", err)
	}
}

// trackingDailySyncer wraps a MatchSyncer and counts DailyFixtureSync calls.
type trackingDailySyncer struct {
	inner service.MatchSyncer
	count *int
}

func (s *trackingDailySyncer) PollAndApply(ctx context.Context, w int) (int, error) {
	return s.inner.PollAndApply(ctx, w)
}
func (s *trackingDailySyncer) LinkExternal(ctx context.Context, id int, p string, e int64) error {
	return s.inner.LinkExternal(ctx, id, p, e)
}
func (s *trackingDailySyncer) UnlinkExternal(ctx context.Context, id int) error {
	return s.inner.UnlinkExternal(ctx, id)
}
func (s *trackingDailySyncer) ReconcileDate(ctx context.Context, l, se int) ([]service.SyncDiff, error) {
	return s.inner.ReconcileDate(ctx, l, se)
}
func (s *trackingDailySyncer) DailyFixtureSync(ctx context.Context, start, end *time.Time) (*service.DailySyncResult, error) {
	*s.count++
	return s.inner.DailyFixtureSync(ctx, start, end)
}

var _ service.MatchSyncer = (*trackingDailySyncer)(nil)
