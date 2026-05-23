package scheduler_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/scheduler"
)

// ── Test doubles ─────────────────────────────────────────────────────────────

// stubClock is an injectable Nower that returns a fixed time.
type stubClock struct{ t time.Time }

func (c *stubClock) Now() time.Time { return c.t }

// stubLeader is a controllable LeaderElector.
type stubLeader struct{ leader bool }

func (l *stubLeader) TryAcquire(_ context.Context) bool { return l.leader }

// stubWriter records all Write and WriteDedup calls for assertion in tests.
// WriteDedup tracks seen dedup keys so a second call with the same key returns
// written=false, mirroring the production DB behaviour.
type stubWriter struct {
	events    []notification.EventType
	seenDedup map[string]bool
}

func (w *stubWriter) Write(_ context.Context, et notification.EventType, _, _ string, _ any) error {
	w.events = append(w.events, et)
	return nil
}

func (w *stubWriter) WriteDedup(_ context.Context, dedupKey string, et notification.EventType, _, _ string, _ any) (bool, error) {
	if w.seenDedup == nil {
		w.seenDedup = make(map[string]bool)
	}
	if w.seenDedup[dedupKey] {
		return false, nil
	}
	w.seenDedup[dedupKey] = true
	w.events = append(w.events, et)
	return true, nil
}

// stubParams returns configurable values for stale threshold params.
type stubParams struct {
	bankSec, withdrawSec int
	queueDepthThreshold  int // 0 means "use default"
}

func (s *stubParams) GetInt(_ context.Context, key string, defaultVal int) int {
	switch key {
	case domain.ParamKeyNotifyBankTransferStaleSec:
		return s.bankSec
	case domain.ParamKeyNotifyWithdrawalStaleSec:
		return s.withdrawSec
	case domain.ParamKeyNotifyBankTransferQueueDepthThreshold:
		if s.queueDepthThreshold != 0 {
			return s.queueDepthThreshold
		}
	}
	return defaultVal
}

// stubStore is a minimal Store for tests.
type stubStore struct {
	pendingTransfers   int
	pendingWithdrawals int
	oldest             time.Time
	oldestErr          error
	finishedMatches    []*domain.Match
	deadlineMatches    []scheduler.DeadlineMatch
	staleBankTransfers []*domain.BankTransferProof
	staleWithdrawals   []*domain.WithdrawalRequest
}

func (s *stubStore) CountPendingTransfers(_ context.Context) (int, error) {
	return s.pendingTransfers, nil
}
func (s *stubStore) CountPendingWithdrawals(_ context.Context) (int, error) {
	return s.pendingWithdrawals, nil
}
func (s *stubStore) OldestPendingTransferSince(_ context.Context) (time.Time, error) {
	return s.oldest, s.oldestErr
}
func (s *stubStore) DailySummary(_ context.Context, _ time.Time) (scheduler.DailySummaryRow, error) {
	return scheduler.DailySummaryRow{}, nil
}
func (s *stubStore) WeeklySummary(_ context.Context, _ time.Time) (scheduler.WeeklySummaryRow, error) {
	return scheduler.WeeklySummaryRow{}, nil
}
func (s *stubStore) ListFinishedMatchesMissingResult(_ context.Context) ([]*domain.Match, error) {
	return s.finishedMatches, nil
}
func (s *stubStore) ListUpcomingMatchesWithDeadline(_ context.Context, _ time.Duration) ([]scheduler.DeadlineMatch, error) {
	return s.deadlineMatches, nil
}
func (s *stubStore) ListStaleBankTransfers(_ context.Context, _ time.Time) ([]*domain.BankTransferProof, error) {
	return s.staleBankTransfers, nil
}
func (s *stubStore) ListStaleWithdrawals(_ context.Context, _ time.Time) ([]*domain.WithdrawalRequest, error) {
	return s.staleWithdrawals, nil
}

// ── Scheduler unit tests ──────────────────────────────────────────────────────

func TestScheduler_IntervalJob_FiresWhenDue(t *testing.T) {
	t.Parallel()

	var fired int32
	s := scheduler.New(scheduler.Config{Log: zap.NewNop()})
	s.RegisterInterval("test-job", 5*time.Minute, func(_ context.Context) error {
		atomic.AddInt32(&fired, 1)
		return nil
	})

	base := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)

	// First tick: lastRun is zero → always fires.
	s.RunWithTick(context.Background(), base)
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("first tick: fired=%d; want 1", got)
	}

	// Second tick at +4 min: not yet due.
	s.RunWithTick(context.Background(), base.Add(4*time.Minute))
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("+4m tick: fired=%d; want 1 (not yet due)", got)
	}

	// Third tick at +5 min: due.
	s.RunWithTick(context.Background(), base.Add(5*time.Minute))
	if got := atomic.LoadInt32(&fired); got != 2 {
		t.Errorf("+5m tick: fired=%d; want 2", got)
	}
}

func TestScheduler_DailyJob_FiresAtConfiguredTime(t *testing.T) {
	t.Parallel()

	var fired int32
	loc := time.UTC
	s := scheduler.New(scheduler.Config{Location: loc, Log: zap.NewNop()})
	s.RegisterDaily("daily-job", 8, 0, func(_ context.Context) error {
		atomic.AddInt32(&fired, 1)
		return nil
	})

	// 07:59 — should not fire.
	s.RunWithTick(context.Background(), time.Date(2026, 5, 19, 7, 59, 0, 0, loc))
	if got := atomic.LoadInt32(&fired); got != 0 {
		t.Errorf("07:59 tick: fired=%d; want 0", got)
	}

	// 08:00 — should fire (first run).
	s.RunWithTick(context.Background(), time.Date(2026, 5, 19, 8, 0, 0, 0, loc))
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("08:00 tick: fired=%d; want 1", got)
	}

	// 08:01 same day — should NOT re-fire (already ran < 23h ago).
	s.RunWithTick(context.Background(), time.Date(2026, 5, 19, 8, 1, 0, 0, loc))
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("08:01 same day: fired=%d; want 1", got)
	}

	// Next day 08:00 — should fire again.
	s.RunWithTick(context.Background(), time.Date(2026, 5, 20, 8, 0, 0, 0, loc))
	if got := atomic.LoadInt32(&fired); got != 2 {
		t.Errorf("next day 08:00: fired=%d; want 2", got)
	}
}

func TestScheduler_WeeklyJob_FiresOnConfiguredWeekday(t *testing.T) {
	t.Parallel()

	var fired int32
	loc := time.UTC
	s := scheduler.New(scheduler.Config{Location: loc, Log: zap.NewNop()})
	// Monday 08:00
	s.RegisterWeekly("weekly-job", time.Monday, 8, 0, func(_ context.Context) error {
		atomic.AddInt32(&fired, 1)
		return nil
	})

	// Tuesday — should not fire.
	tuesday := time.Date(2026, 5, 19, 8, 0, 0, 0, loc) // 2026-05-19 is a Tuesday
	s.RunWithTick(context.Background(), tuesday)
	if got := atomic.LoadInt32(&fired); got != 0 {
		t.Errorf("tuesday 08:00: fired=%d; want 0", got)
	}

	// Monday — should fire.
	monday := time.Date(2026, 5, 25, 8, 0, 0, 0, loc) // next Monday
	s.RunWithTick(context.Background(), monday)
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("monday 08:00: fired=%d; want 1", got)
	}
}

func TestScheduler_NonLeader_SkipsAllJobs(t *testing.T) {
	t.Parallel()

	var fired int32
	s := scheduler.New(scheduler.Config{
		Elector: &stubLeader{leader: false},
		Log:     zap.NewNop(),
	})
	s.RegisterInterval("job", time.Millisecond, func(_ context.Context) error {
		atomic.AddInt32(&fired, 1)
		return nil
	})

	s.RunWithTick(context.Background(), time.Now())
	if got := atomic.LoadInt32(&fired); got != 0 {
		t.Errorf("non-leader: fired=%d; want 0", got)
	}
}

func TestScheduler_JobError_DoesNotStopSubsequentJobs(t *testing.T) {
	t.Parallel()

	var secondFired int32
	s := scheduler.New(scheduler.Config{Log: zap.NewNop()})
	s.RegisterInterval("fail-job", time.Millisecond, func(_ context.Context) error {
		return context.DeadlineExceeded
	})
	s.RegisterInterval("ok-job", time.Millisecond, func(_ context.Context) error {
		atomic.AddInt32(&secondFired, 1)
		return nil
	})

	s.RunWithTick(context.Background(), time.Now())
	if got := atomic.LoadInt32(&secondFired); got != 1 {
		t.Errorf("second job after error: fired=%d; want 1", got)
	}
}

// ── Job unit tests ────────────────────────────────────────────────────────────

func TestJobs_AdminPendingReminder_EmitsEventWhenPendingExist(t *testing.T) {
	t.Parallel()

	store := &stubStore{pendingTransfers: 3, pendingWithdrawals: 1}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminPendingReminder {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminPendingReminder)
	}
}

func TestJobs_AdminPendingReminder_SkipsWhenNoPending(t *testing.T) {
	t.Parallel()

	store := &stubStore{pendingTransfers: 0, pendingWithdrawals: 0}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("events: %v; want none", writer.events)
	}
}

func TestJobs_AdminPendingReminder_QueueDepthExceeded_EmitsBothEvents(t *testing.T) {
	t.Parallel()

	// 25 pending transfers with a threshold of 20 → both the regular reminder
	// and the P0 queue-depth alert should be emitted.
	store := &stubStore{pendingTransfers: 25, pendingWithdrawals: 0}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{queueDepthThreshold: 20},
		Log:    zap.NewNop(),
	})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 2 {
		t.Fatalf("events: got %d; want 2", len(writer.events))
	}
	if writer.events[0] != notification.EventAdminPendingReminder {
		t.Errorf("events[0]: got %s; want %s", writer.events[0], notification.EventAdminPendingReminder)
	}
	if writer.events[1] != notification.EventAdminBankTransferQueueDepth {
		t.Errorf("events[1]: got %s; want %s", writer.events[1], notification.EventAdminBankTransferQueueDepth)
	}
}

func TestJobs_AdminPendingReminder_BelowQueueDepthThreshold_SkipsQueueDepthEvent(t *testing.T) {
	t.Parallel()

	// 5 pending transfers with a threshold of 20 → only the regular reminder.
	store := &stubStore{pendingTransfers: 5, pendingWithdrawals: 2}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{queueDepthThreshold: 20},
		Log:    zap.NewNop(),
	})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminPendingReminder {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminPendingReminder)
	}
}

func TestJobs_AdminMatchResultPending_EmitsOnePerMatch(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		finishedMatches: []*domain.Match{
			{ID: 10, HomeTeam: "Brazil", AwayTeam: "France", KickoffAt: time.Now().Add(-95 * time.Minute)},
			{ID: 11, HomeTeam: "Germany", AwayTeam: "Spain", KickoffAt: time.Now().Add(-100 * time.Minute)},
		},
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminMatchResultPending(context.Background()); err != nil {
		t.Fatalf("AdminMatchResultPending: %v", err)
	}
	if len(writer.events) != 2 {
		t.Errorf("events: got %d; want 2", len(writer.events))
	}
	for _, et := range writer.events {
		if et != notification.EventAdminMatchResultPending {
			t.Errorf("unexpected event: %s", et)
		}
	}
}

func TestJobs_PredictionDeadlineApproaching_EmitsPerUser(t *testing.T) {
	t.Parallel()

	// MinutesLeft=60 aligns with the default bucket-1 lead time (60±5 min).
	m := &domain.Match{ID: 5, HomeTeam: "Mexico", AwayTeam: "USA", KickoffAt: time.Now().Add(60 * time.Minute)}
	store := &stubStore{
		deadlineMatches: []scheduler.DeadlineMatch{
			{Match: m, MissingUserIDs: []int{100, 101, 102}, MinutesLeft: 60},
		},
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{},
		Log:    zap.NewNop(),
	})

	if err := jobs.PredictionDeadlineApproaching(context.Background()); err != nil {
		t.Fatalf("PredictionDeadlineApproaching: %v", err)
	}
	if len(writer.events) != 3 {
		t.Errorf("events: got %d; want 3", len(writer.events))
	}
	for _, et := range writer.events {
		if et != notification.EventPredictionMissingReminder {
			t.Errorf("unexpected event: %s", et)
		}
	}
}

func TestJobs_PredictionDeadlineApproaching_SkipsOutsideBucket(t *testing.T) {
	t.Parallel()

	// MinutesLeft=30 is between the default buckets (60 and 15) so no reminder
	// should fire.
	m := &domain.Match{ID: 7, HomeTeam: "Brazil", AwayTeam: "Germany", KickoffAt: time.Now().Add(30 * time.Minute)}
	store := &stubStore{
		deadlineMatches: []scheduler.DeadlineMatch{
			{Match: m, MissingUserIDs: []int{200}, MinutesLeft: 30},
		},
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{},
		Log:    zap.NewNop(),
	})

	if err := jobs.PredictionDeadlineApproaching(context.Background()); err != nil {
		t.Fatalf("PredictionDeadlineApproaching: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("events: got %d; want 0 (MinutesLeft=30 outside both buckets)", len(writer.events))
	}
}

func TestJobs_PredictionDeadlineApproaching_DedupPreventsDouble(t *testing.T) {
	t.Parallel()

	m := &domain.Match{ID: 9, HomeTeam: "France", AwayTeam: "Spain", KickoffAt: time.Now().Add(60 * time.Minute)}
	store := &stubStore{
		deadlineMatches: []scheduler.DeadlineMatch{
			{Match: m, MissingUserIDs: []int{300}, MinutesLeft: 60},
		},
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{},
		Log:    zap.NewNop(),
	})

	// First run: should emit one event.
	if err := jobs.PredictionDeadlineApproaching(context.Background()); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if len(writer.events) != 1 {
		t.Fatalf("first run: events=%d; want 1", len(writer.events))
	}

	// Second run (same bucket, same user, same match): dedup key already seen.
	if err := jobs.PredictionDeadlineApproaching(context.Background()); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if len(writer.events) != 1 {
		t.Errorf("second run: events=%d; want 1 (dedup should prevent double-emit)", len(writer.events))
	}
}

func TestJobs_AdminDailySummary_EmitsEvent(t *testing.T) {
	t.Parallel()

	store := &stubStore{}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminDailySummary(context.Background()); err != nil {
		t.Fatalf("AdminDailySummary: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminDailySummary {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminDailySummary)
	}
}

func TestJobs_AdminWeeklyReport_EmitsEvent(t *testing.T) {
	t.Parallel()

	store := &stubStore{}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminWeeklyReport(context.Background()); err != nil {
		t.Fatalf("AdminWeeklyReport: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminWeeklyReport {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminWeeklyReport)
	}
}

func TestRealClock_Now_ReturnsCurrentTime(t *testing.T) {
	t.Parallel()

	before := time.Now()
	c := scheduler.RealClock{}
	got := c.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Errorf("RealClock.Now() = %v; want between %v and %v", got, before, after)
	}
}

func TestScheduler_Run_StopsOnContextCancel(t *testing.T) {
	t.Parallel()

	s := scheduler.New(scheduler.Config{
		Tick: time.Hour, // long tick so it never fires within the test
		Log:  zap.NewNop(),
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Scheduler.Run did not stop after context cancellation")
	}
}

func TestJobs_AdminPendingReminder_OldestNonZero_IncludedInPayload(t *testing.T) {
	t.Parallel()

	oldest := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	store := &stubStore{pendingTransfers: 2, pendingWithdrawals: 0, oldest: oldest}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 1 {
		t.Fatalf("expected 1 event; got %d", len(writer.events))
	}
}

func TestJobs_AdminPendingReminder_OldestError_StillEmitsEvent(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		pendingTransfers:   1,
		pendingWithdrawals: 0,
		oldestErr:          errors.New("db error"),
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	// Event is still emitted even though OldestPendingTransferSince failed.
	if len(writer.events) != 1 {
		t.Errorf("expected 1 event; got %d", len(writer.events))
	}
}

// ── StaleEscalation tests ─────────────────────────────────────────────────────

func TestJobs_StaleEscalation_EmitsBankTransferStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	// proof created 13 h ago; threshold is 12 h → stale
	staleProof := &domain.BankTransferProof{
		ID: 7, UserID: 42, AmountCents: 100_000, Currency: "GTQ",
		CreatedAt: now.Add(-13 * time.Hour),
	}
	store := &stubStore{staleBankTransfers: []*domain.BankTransferProof{staleProof}}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{bankSec: 12 * 3600, withdrawSec: 24 * 3600},
		Clock:  &stubClock{t: now},
		Log:    zap.NewNop(),
	})

	if err := jobs.StaleEscalation(context.Background()); err != nil {
		t.Fatalf("StaleEscalation: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminBankTransferStale {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminBankTransferStale)
	}
}

func TestJobs_StaleEscalation_EmitsWithdrawalStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	staleReq := &domain.WithdrawalRequest{
		ID: 3, UserID: 9, AmountCents: 50_000, Currency: "GTQ",
		CreatedAt: now.Add(-25 * time.Hour), // 25 h > 24 h threshold
	}
	store := &stubStore{staleWithdrawals: []*domain.WithdrawalRequest{staleReq}}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{bankSec: 12 * 3600, withdrawSec: 24 * 3600},
		Clock:  &stubClock{t: now},
		Log:    zap.NewNop(),
	})

	if err := jobs.StaleEscalation(context.Background()); err != nil {
		t.Fatalf("StaleEscalation: %v", err)
	}
	if len(writer.events) != 1 || writer.events[0] != notification.EventAdminWithdrawalStale {
		t.Errorf("events: %v; want [%s]", writer.events, notification.EventAdminWithdrawalStale)
	}
}

func TestJobs_StaleEscalation_NothingStale_NoEvents(t *testing.T) {
	t.Parallel()

	store := &stubStore{} // both slices nil → no stale items
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  store,
		Writer: writer,
		Params: &stubParams{bankSec: 12 * 3600, withdrawSec: 24 * 3600},
		Log:    zap.NewNop(),
	})

	if err := jobs.StaleEscalation(context.Background()); err != nil {
		t.Fatalf("StaleEscalation: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("events: got %d; want 0", len(writer.events))
	}
}

func TestJobs_StaleEscalation_NilParams_Skips(t *testing.T) {
	t.Parallel()

	store := &stubStore{
		staleBankTransfers: []*domain.BankTransferProof{{ID: 1, CreatedAt: time.Now().Add(-24 * time.Hour)}},
	}
	writer := &stubWriter{}
	// Params is intentionally nil — StaleEscalation must be a no-op.
	jobs := scheduler.NewJobs(scheduler.JobsConfig{Store: store, Writer: writer, Log: zap.NewNop()})

	if err := jobs.StaleEscalation(context.Background()); err != nil {
		t.Fatalf("StaleEscalation with nil params: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("events: got %d; want 0 (params=nil should skip)", len(writer.events))
	}
}

// ── HealthChecker tests ───────────────────────────────────────────────────────

func TestSchedulerHealthChecker_Name(t *testing.T) {
	t.Parallel()
	s := scheduler.New(scheduler.Config{Log: zap.NewNop()})
	if got := s.HealthChecker(3.0).Name(); got != "scheduler" {
		t.Errorf("Name() = %q; want %q", got, "scheduler")
	}
}

func TestSchedulerHealthChecker_NoJobs_OK(t *testing.T) {
	t.Parallel()
	s := scheduler.New(scheduler.Config{Log: zap.NewNop()})
	checker := s.HealthChecker(3.0)
	result := checker.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("Check() status = %q; want %q", result.Status, "ok")
	}
}

func TestSchedulerHealthChecker_StartupGrace_OK(t *testing.T) {
	// Jobs that have never fired (lastRun zero) must not trigger the health check.
	t.Parallel()
	clock := &stubClock{t: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)}
	s := scheduler.New(scheduler.Config{Clock: clock, Log: zap.NewNop()})
	s.RegisterInterval("new-job", time.Minute, func(_ context.Context) error { return nil })

	checker := s.HealthChecker(3.0)
	result := checker.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("startup grace: status = %q; want %q", result.Status, "ok")
	}
}

func TestSchedulerHealthChecker_IntervalJobOverdue_Error(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	clock := &stubClock{t: base}
	s := scheduler.New(scheduler.Config{Clock: clock, Log: zap.NewNop()})
	s.RegisterInterval("tick-job", 5*time.Minute, func(_ context.Context) error { return nil })

	// Fire the job so lastRun is set.
	s.RunWithTick(context.Background(), base)

	// Advance clock past threshold (3× interval = 15 min).
	clock.t = base.Add(16 * time.Minute)

	checker := s.HealthChecker(3.0)
	result := checker.Check(context.Background())
	if result.Status != "error" {
		t.Errorf("overdue interval: status = %q; want %q", result.Status, "error")
	}
	if result.Error == "" {
		t.Error("overdue interval: expected non-empty Error field")
	}
}

func TestSchedulerHealthChecker_IntervalJobWithinThreshold_OK(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	clock := &stubClock{t: base}
	s := scheduler.New(scheduler.Config{Clock: clock, Log: zap.NewNop()})
	s.RegisterInterval("tick-job", 5*time.Minute, func(_ context.Context) error { return nil })
	s.RunWithTick(context.Background(), base)

	// Advance to just under the threshold (3×5 min = 15 min).
	clock.t = base.Add(14 * time.Minute)

	checker := s.HealthChecker(3.0)
	result := checker.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("within threshold: status = %q; want %q — %s", result.Status, "ok", result.Error)
	}
}

func TestSchedulerHealthChecker_DailyJobOverdue_Error(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	clock := &stubClock{t: base}
	s := scheduler.New(scheduler.Config{Clock: clock, Log: zap.NewNop()})
	s.RegisterDaily("daily-job", 8, 0, func(_ context.Context) error { return nil })
	s.RunWithTick(context.Background(), base)

	// Advance past threshold (3 × 25 h = 75 h).
	clock.t = base.Add(76 * time.Hour)

	result := s.HealthChecker(3.0).Check(context.Background())
	if result.Status != "error" {
		t.Errorf("overdue daily: status = %q; want %q", result.Status, "error")
	}
}

func TestSchedulerHealthChecker_WeeklyJobOverdue_Error(t *testing.T) {
	t.Parallel()

	// 2026-05-25 is a Monday.
	base := time.Date(2026, 5, 25, 8, 0, 0, 0, time.UTC)
	clock := &stubClock{t: base}
	s := scheduler.New(scheduler.Config{Clock: clock, Log: zap.NewNop()})
	s.RegisterWeekly("weekly-job", time.Monday, 8, 0, func(_ context.Context) error { return nil })
	s.RunWithTick(context.Background(), base)

	// Advance past threshold (3 × (7×24h + 1h) = 3 × 169h = 507 h).
	clock.t = base.Add(508 * time.Hour)

	result := s.HealthChecker(3.0).Check(context.Background())
	if result.Status != "error" {
		t.Errorf("overdue weekly: status = %q; want %q", result.Status, "error")
	}
}
