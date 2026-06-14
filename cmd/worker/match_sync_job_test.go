package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// resetGlobalMatchSyncState zeroes every atomic in globalMatchSyncState so
// tests start from a clean slate regardless of execution order.
func resetGlobalMatchSyncState() {
	globalMatchSyncState.lastLiveCount.Store(0)
	globalMatchSyncState.lastPollUnixSec.Store(0)
	globalMatchSyncState.consecutiveZeroCount.Store(0)
	globalMatchSyncState.pollingPaused.Store(false)
	globalMatchSyncState.resumeAtUnix.Store(0)
}

// configParamSvc overrides specific param values; returns the default for any
// key not present in the maps, matching production behaviour.
type configParamSvc struct {
	bools   map[string]bool
	ints    map[string]int
	strings map[string]string
}

// Extra methods present on stubParamSvc in main_test.go — kept here so both
// stubs satisfy the same concrete shape without conflicting declarations.
func (s *configParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *configParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) { return nil, nil }
func (s *configParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (s *configParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *configParamSvc) GetString(_ context.Context, key, defaultVal string) string {
	if v, ok := s.strings[key]; ok {
		return v
	}
	return defaultVal
}
func (s *configParamSvc) GetInt(_ context.Context, key string, defaultVal int) int {
	if v, ok := s.ints[key]; ok {
		return v
	}
	return defaultVal
}
func (s *configParamSvc) GetDuration(_ context.Context, _ string, defaultVal time.Duration) time.Duration {
	return defaultVal
}
func (s *configParamSvc) GetBool(_ context.Context, key string, defaultVal bool) bool {
	if v, ok := s.bools[key]; ok {
		return v
	}
	return defaultVal
}
func (s *configParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error { return nil }
func (s *configParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (s *configParamSvc) BulkPreview(_ context.Context, _ map[string]string) ([]domain.ParamDiff, error) {
	return nil, nil
}
func (s *configParamSvc) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

var _ service.SystemParamService = (*configParamSvc)(nil)

// enabledParams returns a configParamSvc with match sync enabled and any
// additional int overrides, keeping test setup concise.
func enabledParams(ints map[string]int) *configParamSvc {
	return &configParamSvc{
		bools: map[string]bool{domain.ParamKeyMatchSyncEnabled: true},
		ints:  ints,
	}
}

// ── stubWorkerMatchSyncer ────────────────────────────────────────────────────

type stubWorkerMatchSyncer struct {
	liveCount int
	pollErr   error
}

func (s *stubWorkerMatchSyncer) PollAndApply(_ context.Context, _ int) (int, error) {
	return s.liveCount, s.pollErr
}
func (s *stubWorkerMatchSyncer) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	return nil
}
func (s *stubWorkerMatchSyncer) UnlinkExternal(_ context.Context, _ int) error { return nil }
func (s *stubWorkerMatchSyncer) ReconcileDate(_ context.Context, _, _ int) ([]service.SyncDiff, error) {
	return nil, nil
}
func (s *stubWorkerMatchSyncer) DailyFixtureSync(_ context.Context, _, _ *time.Time) (*service.DailySyncResult, error) {
	return &service.DailySyncResult{}, nil
}

var _ service.MatchSyncer = (*stubWorkerMatchSyncer)(nil)

// ── stubWorkerMatchRepo ──────────────────────────────────────────────────────

// stubWorkerMatchRepo implements repository.MatchRepository for worker tests.
// Only ListSyncCandidates is exercised by the functions under test.
type stubWorkerMatchRepo struct {
	candidates    []*domain.Match
	candidatesErr error
}

func (r *stubWorkerMatchRepo) Create(_ context.Context, _ *domain.Match) error { return nil }
func (r *stubWorkerMatchRepo) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return nil, nil
}
func (r *stubWorkerMatchRepo) Update(_ context.Context, _ *domain.Match) error { return nil }
func (r *stubWorkerMatchRepo) List(_ context.Context) ([]*domain.Match, error) { return nil, nil }
func (r *stubWorkerMatchRepo) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubWorkerMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return nil, nil
}
func (r *stubWorkerMatchRepo) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	return nil
}
func (r *stubWorkerMatchRepo) UnlinkExternal(_ context.Context, _ int) error { return nil }
func (r *stubWorkerMatchRepo) ListSyncCandidates(_ context.Context, _ int) ([]*domain.Match, error) {
	return r.candidates, r.candidatesErr
}
func (r *stubWorkerMatchRepo) UpdateSyncState(_ context.Context, _ int) error { return nil }
func (r *stubWorkerMatchRepo) FindByTeams(_ context.Context, _, _ string) (*domain.Match, error) {
	return nil, nil
}
func (r *stubWorkerMatchRepo) UpdateKickoff(_ context.Context, _ int, _ time.Time) error {
	return nil
}

var _ repository.MatchRepository = (*stubWorkerMatchRepo)(nil)

// ── handlePauseResume ─────────────────────────────────────────────────────────

func TestHandlePauseResume_NotPaused_ReturnsFalse(t *testing.T) {
	resetGlobalMatchSyncState()

	if handlePauseResume(zap.NewNop()) {
		t.Error("expected false when not paused")
	}
}

func TestHandlePauseResume_PausedWithSentinel_ReturnsTrue(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.pollingPaused.Store(true)
	globalMatchSyncState.resumeAtUnix.Store(0) // sentinel: no upcoming match known

	if !handlePauseResume(zap.NewNop()) {
		t.Error("expected true when paused with sentinel 0")
	}
	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("pause state should not be cleared by sentinel 0")
	}
}

func TestHandlePauseResume_PausedFutureTime_ReturnsTrue(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.pollingPaused.Store(true)
	globalMatchSyncState.resumeAtUnix.Store(time.Now().Add(10 * time.Minute).Unix())

	if !handlePauseResume(zap.NewNop()) {
		t.Error("expected true when resume time is in the future")
	}
	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("pause state should not be cleared before resumeAt")
	}
}

func TestHandlePauseResume_PausedPastTime_ResetsAndReturnsFalse(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.pollingPaused.Store(true)
	globalMatchSyncState.resumeAtUnix.Store(time.Now().Add(-1 * time.Minute).Unix())
	globalMatchSyncState.consecutiveZeroCount.Store(5)

	if handlePauseResume(zap.NewNop()) {
		t.Error("expected false when resumeAt has passed")
	}
	if globalMatchSyncState.pollingPaused.Load() {
		t.Error("pause state should be cleared after resumeAt passed")
	}
	if globalMatchSyncState.consecutiveZeroCount.Load() != 0 {
		t.Error("consecutiveZeroCount should be reset to 0")
	}
	if globalMatchSyncState.resumeAtUnix.Load() != 0 {
		t.Error("resumeAtUnix should be reset to 0")
	}
}

// ── nextScheduledMatchKickoff ─────────────────────────────────────────────────

func TestNextScheduledMatchKickoff_RepoError_ReturnsFalse(t *testing.T) {
	repo := &stubWorkerMatchRepo{candidatesErr: errors.New("db down")}
	_, found := nextScheduledMatchKickoff(context.Background(), repo)
	if found {
		t.Error("expected found=false on repo error")
	}
}

func TestNextScheduledMatchKickoff_NoCandidates_ReturnsFalse(t *testing.T) {
	repo := &stubWorkerMatchRepo{candidates: nil}
	_, found := nextScheduledMatchKickoff(context.Background(), repo)
	if found {
		t.Error("expected found=false when no candidates returned")
	}
}

func TestNextScheduledMatchKickoff_NoFutureScheduled_ReturnsFalse(t *testing.T) {
	past := time.Now().Add(-1 * time.Hour)
	repo := &stubWorkerMatchRepo{candidates: []*domain.Match{
		// Live match — filtered out regardless of kickoff
		{Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(1 * time.Hour)},
		// Scheduled but in the past
		{Status: domain.MatchStatusScheduled, KickoffAt: past},
	}}
	_, found := nextScheduledMatchKickoff(context.Background(), repo)
	if found {
		t.Error("expected found=false when no future scheduled matches exist")
	}
}

func TestNextScheduledMatchKickoff_ReturnsEarliestFuture(t *testing.T) {
	earlier := time.Now().Add(1 * time.Hour)
	later := time.Now().Add(3 * time.Hour)
	repo := &stubWorkerMatchRepo{candidates: []*domain.Match{
		{Status: domain.MatchStatusScheduled, KickoffAt: later},
		{Status: domain.MatchStatusScheduled, KickoffAt: earlier},
	}}
	kickoff, found := nextScheduledMatchKickoff(context.Background(), repo)
	if !found {
		t.Fatal("expected found=true with future scheduled matches")
	}
	if !kickoff.Equal(earlier) {
		t.Errorf("expected earliest kickoff %v, got %v", earlier, kickoff)
	}
}

// ── suspendPollingIfThresholdReached ─────────────────────────────────────────

func TestSuspendPollingIfThreshold_BelowThreshold_DoesNotPause(t *testing.T) {
	resetGlobalMatchSyncState()
	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
	})

	suspendPollingIfThresholdReached(context.Background(), params, &stubWorkerMatchRepo{}, 10, zap.NewNop())

	if globalMatchSyncState.pollingPaused.Load() {
		t.Error("should not pause when below threshold")
	}
	if globalMatchSyncState.consecutiveZeroCount.Load() != 1 {
		t.Errorf("zero count should be 1 after first zero tick, got %d",
			globalMatchSyncState.consecutiveZeroCount.Load())
	}
}

func TestSuspendPollingIfThreshold_AtThreshold_NoMatches_PausesWithSentinel(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2) // next Add(1) reaches 3
	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
	})

	suspendPollingIfThresholdReached(context.Background(), params, &stubWorkerMatchRepo{}, 10, zap.NewNop())

	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("expected polling to be paused at threshold with no matches")
	}
	if globalMatchSyncState.resumeAtUnix.Load() != 0 {
		t.Error("expected sentinel 0 when no future matches found")
	}
}

func TestSuspendPollingIfThreshold_AtThreshold_FutureMatch_PausesWithResumeAt(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2)
	prematchWindow := 10
	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
	})
	nextKickoff := time.Now().Add(2 * time.Hour)
	repo := &stubWorkerMatchRepo{candidates: []*domain.Match{
		{Status: domain.MatchStatusScheduled, KickoffAt: nextKickoff},
	}}

	suspendPollingIfThresholdReached(context.Background(), params, repo, prematchWindow, zap.NewNop())

	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("expected polling to be paused")
	}
	want := nextKickoff.Unix() - int64(prematchWindow*60)
	if globalMatchSyncState.resumeAtUnix.Load() != want {
		t.Errorf("resumeAtUnix: want %d, got %d", want, globalMatchSyncState.resumeAtUnix.Load())
	}
}

func TestSuspendPollingIfThreshold_KickoffInsideWindow_DoesNotPause(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2)
	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
	})
	// Kickoff only 5 min away, pre-match window is 10 min → already inside window
	nextKickoff := time.Now().Add(5 * time.Minute)
	repo := &stubWorkerMatchRepo{candidates: []*domain.Match{
		{Status: domain.MatchStatusScheduled, KickoffAt: nextKickoff},
	}}

	suspendPollingIfThresholdReached(context.Background(), params, repo, 10, zap.NewNop())

	if globalMatchSyncState.pollingPaused.Load() {
		t.Error("should not pause when kickoff is already within pre-match window")
	}
}

// ── makeMatchSyncJob ──────────────────────────────────────────────────────────

func TestMakeMatchSyncJob_Disabled_ReturnsNil(t *testing.T) {
	resetGlobalMatchSyncState()
	params := &configParamSvc{bools: map[string]bool{domain.ParamKeyMatchSyncEnabled: false}}
	job := makeMatchSyncJob(params, &stubWorkerMatchSyncer{}, &stubWorkerMatchRepo{}, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestMakeMatchSyncJob_Paused_SkipsTick(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.pollingPaused.Store(true)
	globalMatchSyncState.resumeAtUnix.Store(time.Now().Add(1 * time.Hour).Unix())

	job := makeMatchSyncJob(enabledParams(nil), &stubWorkerMatchSyncer{}, &stubWorkerMatchRepo{}, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	// lastPollUnixSec must not have been updated (poll was skipped).
	if globalMatchSyncState.lastPollUnixSec.Load() != 0 {
		t.Error("lastPollUnixSec should remain 0 when polling is paused")
	}
}

func TestMakeMatchSyncJob_SlowPollSkip_ReturnsNil(t *testing.T) {
	resetGlobalMatchSyncState()
	// lastLiveCount=0, lastPoll=1 s ago, slowSec=120 → skip
	globalMatchSyncState.lastPollUnixSec.Store(time.Now().Unix() - 1)

	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncSlowPollIntervalSec: 120,
	})
	job := makeMatchSyncJob(params, &stubWorkerMatchSyncer{}, &stubWorkerMatchRepo{}, zap.NewNop())

	prevPoll := globalMatchSyncState.lastPollUnixSec.Load()
	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	// lastPollUnixSec must remain unchanged (slow-poll skip fired).
	if globalMatchSyncState.lastPollUnixSec.Load() != prevPoll {
		t.Error("lastPollUnixSec should not be updated when slow-poll skip fires")
	}
}

func TestMakeMatchSyncJob_PollError_ReturnsNil(t *testing.T) {
	resetGlobalMatchSyncState()
	syncer := &stubWorkerMatchSyncer{pollErr: errors.New("provider timeout")}
	job := makeMatchSyncJob(enabledParams(nil), syncer, &stubWorkerMatchRepo{}, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil on PollAndApply error, got %v", err)
	}
}

func TestMakeMatchSyncJob_LiveMatches_ResetsZeroCount(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2)

	syncer := &stubWorkerMatchSyncer{liveCount: 3}
	job := makeMatchSyncJob(enabledParams(nil), syncer, &stubWorkerMatchRepo{}, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if globalMatchSyncState.consecutiveZeroCount.Load() != 0 {
		t.Errorf("consecutiveZeroCount should be 0 when live matches exist, got %d",
			globalMatchSyncState.consecutiveZeroCount.Load())
	}
	if globalMatchSyncState.lastLiveCount.Load() != 3 {
		t.Errorf("lastLiveCount should be 3, got %d", globalMatchSyncState.lastLiveCount.Load())
	}
}

func TestMakeMatchSyncJob_ThresholdReached_SuspendsPolling(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2) // one more zero reaches threshold 3

	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
	})
	// liveCount=0 → accumulates zero count; no repo candidates → sentinel pause
	job := makeMatchSyncJob(params, &stubWorkerMatchSyncer{liveCount: 0}, &stubWorkerMatchRepo{}, zap.NewNop())

	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("expected polling to be suspended after threshold reached")
	}
	if globalMatchSyncState.resumeAtUnix.Load() != 0 {
		t.Error("expected sentinel 0 (no upcoming matches)")
	}
}

func TestMakeMatchSyncJob_ThresholdWithFutureMatch_SetsResumeAt(t *testing.T) {
	resetGlobalMatchSyncState()
	globalMatchSyncState.consecutiveZeroCount.Store(2)

	nextKickoff := time.Now().Add(3 * time.Hour)
	repo := &stubWorkerMatchRepo{candidates: []*domain.Match{
		{Status: domain.MatchStatusScheduled, KickoffAt: nextKickoff},
	}}
	params := enabledParams(map[string]int{
		domain.ParamKeyMatchSyncStopAfterZeroLiveCount: 3,
		domain.ParamKeyMatchSyncPrematchWindowMin:      10,
	})

	job := makeMatchSyncJob(params, &stubWorkerMatchSyncer{liveCount: 0}, repo, zap.NewNop())
	if err := job(context.Background()); err != nil {
		t.Errorf("expected nil, got %v", err)
	}

	if !globalMatchSyncState.pollingPaused.Load() {
		t.Error("expected polling to be suspended")
	}
	want := nextKickoff.Unix() - int64(10*60)
	if globalMatchSyncState.resumeAtUnix.Load() != want {
		t.Errorf("resumeAtUnix: want %d, got %d", want, globalMatchSyncState.resumeAtUnix.Load())
	}
}
