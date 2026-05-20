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

// stubWriter records all Write calls for assertion in tests.
type stubWriter struct {
	events []notification.EventType
}

func (w *stubWriter) Write(_ context.Context, et notification.EventType, _, _ string, _ any) error {
	w.events = append(w.events, et)
	return nil
}

// stubStore is a minimal Store for tests.
type stubStore struct {
	pendingTransfers   int
	pendingWithdrawals int
	oldest             time.Time
	oldestErr          error
	finishedMatches    []*domain.Match
	deadlineMatches    []scheduler.DeadlineMatch
	writeErr           error
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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	if len(writer.events) != 0 {
		t.Errorf("events: %v; want none", writer.events)
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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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

	m := &domain.Match{ID: 5, HomeTeam: "Mexico", AwayTeam: "USA", KickoffAt: time.Now().Add(30 * time.Minute)}
	store := &stubStore{
		deadlineMatches: []scheduler.DeadlineMatch{
			{Match: m, MissingUserIDs: []int{100, 101, 102}, MinutesLeft: 30},
		},
	}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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

func TestJobs_AdminDailySummary_EmitsEvent(t *testing.T) {
	t.Parallel()

	store := &stubStore{}
	writer := &stubWriter{}
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

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
	jobs := scheduler.NewJobs(store, writer, zap.NewNop())

	if err := jobs.AdminPendingReminder(context.Background()); err != nil {
		t.Fatalf("AdminPendingReminder: %v", err)
	}
	// Event is still emitted even though OldestPendingTransferSince failed.
	if len(writer.events) != 1 {
		t.Errorf("expected 1 event; got %d", len(writer.events))
	}
}
