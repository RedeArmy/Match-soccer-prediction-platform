package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// stubScorer is a MatchScorer stub that records the last matchID it received
// and can be configured to return an error.
type stubScorer struct {
	called int
	lastID int
	err    error
}

func (s *stubScorer) ScoreMatch(_ context.Context, matchID int) error {
	s.called++
	s.lastID = matchID
	return s.err
}

// envelopeWithMap simulates the payload state produced by RedisBus after a
// JSON round-trip: the concrete events.MatchFinished struct becomes a
// map[string]interface{} because encoding/json unmarshals `any` fields that way.
func envelopeWithMap(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload: map[string]interface{}{
			"MatchID":   float64(matchID), // JSON numbers unmarshal as float64
			"HomeTeam":  teamMexico,
			"AwayTeam":  "Canada",
			"HomeScore": float64(2),
			"AwayScore": float64(1),
		},
	}
}

// envelopeWithStruct simulates the payload state produced by InMemoryBus:
// the concrete type is preserved in memory, so a type assertion would work
// without decodePayload. decodePayload must also handle this case correctly.
func envelopeWithStruct(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now(),
		Payload: events.MatchFinished{
			MatchID:   matchID,
			HomeTeam:  teamMexico,
			AwayTeam:  "Canada",
			HomeScore: 2,
			AwayScore: 1,
		},
	}
}

// matchStartedEnvelopeWithMap simulates the RedisBus JSON round-trip for a
// MatchStarted payload: the concrete struct becomes map[string]interface{}.
func matchStartedEnvelopeWithMap(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchStarted,
		OccurredAt: time.Now(),
		Payload: map[string]interface{}{
			"MatchID":   float64(matchID),
			"HomeTeam":  teamMexico,
			"AwayTeam":  "Canada",
			"KickoffAt": time.Now().UTC().Format(time.RFC3339),
		},
	}
}

// matchStartedEnvelopeWithStruct simulates the InMemoryBus path for a
// MatchStarted payload: the concrete type is preserved without JSON encoding.
func matchStartedEnvelopeWithStruct(matchID int) events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchStarted,
		OccurredAt: time.Now(),
		Payload: events.MatchStarted{
			MatchID:   matchID,
			HomeTeam:  teamMexico,
			AwayTeam:  "Canada",
			KickoffAt: time.Now().UTC(),
		},
	}
}

// ── decodePayload ─────────────────────────────────────────────────────────────

func TestDecodePayload_MapPayload_DecodesCorrectly(t *testing.T) {
	env := envelopeWithMap(42)
	got, err := decodePayload[events.MatchFinished](env)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.MatchID != 42 {
		t.Errorf("expected MatchID=42, got %d", got.MatchID)
	}
	if got.HomeTeam != teamMexico {
		t.Errorf("expected HomeTeam=%s, got %q", teamMexico, got.HomeTeam)
	}
}

func TestDecodePayload_StructPayload_DecodesCorrectly(t *testing.T) {
	env := envelopeWithStruct(7)
	got, err := decodePayload[events.MatchFinished](env)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.MatchID != 7 {
		t.Errorf("expected MatchID=7, got %d", got.MatchID)
	}
}

func TestDecodePayload_UnmarshalablePayload_ReturnsError(t *testing.T) {
	// A channel cannot be marshalled to JSON, so Marshal will fail.
	env := events.Envelope{
		Type:    events.EventMatchFinished,
		Payload: make(chan int),
	}
	_, err := decodePayload[events.MatchFinished](env)
	if err == nil {
		t.Error("expected error for unmarshalable payload, got nil")
	}
}

func TestDecodePayload_WrongTypeInMap_ReturnsError(t *testing.T) {
	// MatchID is int; "not-a-number" cannot be decoded into int -> unmarshal fails.
	env := events.Envelope{
		Type: events.EventMatchFinished,
		Payload: map[string]interface{}{
			"MatchID": "not-a-number",
		},
	}
	_, err := decodePayload[events.MatchFinished](env)
	if err == nil {
		t.Error("expected unmarshal error for wrong type, got nil")
	}
}

// ── newMatchStartedHandler ────────────────────────────────────────────────────

func TestMatchStartedHandler_MapPayload_ReturnsNil(t *testing.T) {
	h := newMatchStartedHandler(zap.NewNop())

	if err := h(context.Background(), matchStartedEnvelopeWithMap(10)); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestMatchStartedHandler_StructPayload_ReturnsNil(t *testing.T) {
	h := newMatchStartedHandler(zap.NewNop())

	if err := h(context.Background(), matchStartedEnvelopeWithStruct(11)); err != nil {
		t.Errorf(fmtUnexpectedErr, err)
	}
}

func TestMatchStartedHandler_UndecodablePayload_ReturnsNil(t *testing.T) {
	// A channel cannot be JSON-marshalled. The handler must log and return nil
	// rather than an error so the bus does not retry a structurally invalid
	// message - identical policy to newMatchFinishedHandler.
	h := newMatchStartedHandler(zap.NewNop())

	env := events.Envelope{
		Type:    events.EventMatchStarted,
		Payload: make(chan int),
	}
	if err := h(context.Background(), env); err != nil {
		t.Errorf("expected nil for undecodable payload, got %v", err)
	}
}

// ── newMatchFinishedHandler ───────────────────────────────────────────────────

func TestMatchFinishedHandler_MapPayload_CallsScorer(t *testing.T) {
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, nil, nil, nil, noopSnapshotLocker{}, zap.NewNop())

	if err := h(context.Background(), envelopeWithMap(99)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 {
		t.Errorf("expected ScoreMatch called once, got %d", scorer.called)
	}
	if scorer.lastID != 99 {
		t.Errorf("expected ScoreMatch(99), got ScoreMatch(%d)", scorer.lastID)
	}
}

func TestMatchFinishedHandler_StructPayload_CallsScorer(t *testing.T) {
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, nil, nil, nil, noopSnapshotLocker{}, zap.NewNop())

	if err := h(context.Background(), envelopeWithStruct(5)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 || scorer.lastID != 5 {
		t.Errorf("expected ScoreMatch(5), got called=%d id=%d", scorer.called, scorer.lastID)
	}
}

func TestMatchFinishedHandler_ScorerError_PropagatesError(t *testing.T) {
	scorer := &stubScorer{err: errors.New("db down")}
	h := newMatchFinishedHandler(scorer, nil, nil, nil, noopSnapshotLocker{}, zap.NewNop())

	err := h(context.Background(), envelopeWithMap(1))
	if err == nil {
		t.Error("expected error from scorer to be returned, got nil")
	}
}

func TestMatchFinishedHandler_UndecodablePayload_ReturnsNil(t *testing.T) {
	// A channel payload cannot be JSON-marshalled. The handler must log and
	// return nil rather than propagating the error, because retrying a
	// structurally invalid message would burn retry budget uselessly.
	scorer := &stubScorer{}
	h := newMatchFinishedHandler(scorer, nil, nil, nil, noopSnapshotLocker{}, zap.NewNop())

	env := events.Envelope{
		Type:    events.EventMatchFinished,
		Payload: make(chan int),
	}
	if err := h(context.Background(), env); err != nil {
		t.Errorf("expected nil for undecodable payload, got %v", err)
	}
	if scorer.called != 0 {
		t.Error("scorer should not have been called for undecodable payload")
	}
}

// ── postScoringWork ───────────────────────────────────────────────────────────

// stubSnapshotter implements service.Snapshotter for worker snapshot tests.
type stubSnapshotter struct {
	snap *domain.LeaderboardSnapshot
	err  error
}

func (s *stubSnapshotter) Snapshot(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	return s.snap, s.err
}
func (s *stubSnapshotter) SnapshotForMatch(_ context.Context, _, _ int) (*domain.LeaderboardSnapshot, error) {
	return s.snap, s.err
}

// stubWorkerPredRepo implements repository.PredictionRepository.
// Only ListQuinielaIDsByMatch has meaningful behaviour; all other methods
// are no-ops because snapshotAffectedQuinielas does not invoke them.
type stubWorkerPredRepo struct {
	ids []int
	err error
}

func (r *stubWorkerPredRepo) Create(_ context.Context, _ *domain.Prediction) error { return nil }
func (r *stubWorkerPredRepo) Upsert(_ context.Context, _ *domain.Prediction) (bool, error) {
	return true, nil
}
func (r *stubWorkerPredRepo) GetByID(_ context.Context, _ int) (*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) Update(_ context.Context, _ *domain.Prediction) error { return nil }
func (r *stubWorkerPredRepo) UpdateIfUnchanged(_ context.Context, _ *domain.Prediction, _ time.Time) error {
	return nil
}
func (r *stubWorkerPredRepo) GetByUserAndMatch(_ context.Context, _, _ int) (*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) ListByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) ListByMatch(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) UpdateManyPoints(_ context.Context, _ map[int]int) error { return nil }
func (r *stubWorkerPredRepo) TotalPointsByQuiniela(_ context.Context, _ int) (map[int]int, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) TotalPointsByQuinielaAndPhase(_ context.Context, _ int, _ domain.MatchPhase) (map[int]int, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) ListQuinielaIDsByMatch(_ context.Context, _ int) ([]int, error) {
	return r.ids, r.err
}
func (r *stubWorkerPredRepo) ListByUserAndQuiniela(_ context.Context, _, _ int) ([]*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) PredictionStatsByQuiniela(_ context.Context, _ int) (map[int]*domain.UserPredictionStats, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) GetUserPredictionCounts(_ context.Context, _ int) (*domain.UserPredictionCounts, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) GetUserPointsByPhase(_ context.Context, _ int) (map[domain.MatchPhase]int, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) ListUserScoredPointsChronological(_ context.Context, _ int) ([]int, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) ListAdmin(_ context.Context, _ repository.PredictionAdminFilters, _ repository.Pagination) ([]*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) GlobalLeaderboard(_ context.Context, _ int) ([]*domain.GlobalLeaderboardEntry, error) {
	return nil, nil
}

func TestPostScoringWork_NilPredRepo_Noop(t *testing.T) {
	postScoringWork(context.Background(), 1, &stubSnapshotter{}, nil, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_NilSnapshotter_Noop(t *testing.T) {
	postScoringWork(context.Background(), 1, nil, &stubWorkerPredRepo{ids: []int{1}}, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_ListError_LogsWarnAndReturns(t *testing.T) {
	predRepo := &stubWorkerPredRepo{err: errors.New("db down")}
	postScoringWork(context.Background(), 1, &stubSnapshotter{}, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_EmptyList_NoSnapshot(t *testing.T) {
	predRepo := &stubWorkerPredRepo{ids: []int{}}
	postScoringWork(context.Background(), 1, &stubSnapshotter{}, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_SnapshotSuccess_LogsInfo(t *testing.T) {
	predRepo := &stubWorkerPredRepo{ids: []int{10, 20}}
	snap := &stubSnapshotter{snap: &domain.LeaderboardSnapshot{ID: 1}}
	postScoringWork(context.Background(), 5, snap, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_SnapshotError_LogsWarn(t *testing.T) {
	snapshotRetryBase = 0
	t.Cleanup(func() { snapshotRetryBase = 100 * time.Millisecond })

	predRepo := &stubWorkerPredRepo{ids: []int{10}}
	snap := &stubSnapshotter{err: errors.New("snapshot failed")}
	postScoringWork(context.Background(), 5, snap, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
}

func TestPostScoringWork_CallsInvalidators(t *testing.T) {
	called := false
	inv := &stubInvalidator{fn: func(ids []int) { called = true }}
	predRepo := &stubWorkerPredRepo{ids: []int{1}}
	postScoringWork(context.Background(), 5, nil, predRepo, []service.PostScoringInvalidator{inv}, noopSnapshotLocker{}, zap.NewNop())
	if !called {
		t.Error("expected PostScoringInvalidator to be called")
	}
}

func TestMatchFinishedHandler_WithSnapshot_ScoresAndSnapshots(t *testing.T) {
	scorer := &stubScorer{}
	predRepo := &stubWorkerPredRepo{ids: []int{1, 2}}
	snap := &stubSnapshotter{snap: &domain.LeaderboardSnapshot{ID: 1}}
	h := newMatchFinishedHandler(scorer, snap, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())

	if err := h(context.Background(), envelopeWithStruct(10)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 || scorer.lastID != 10 {
		t.Errorf("expected ScoreMatch(10) called once, got called=%d id=%d", scorer.called, scorer.lastID)
	}
}

// ── retrySnapshot ─────────────────────────────────────────────────────────────

func TestRetrySnapshot_FirstAttemptSucceeds_CalledOnce(t *testing.T) {
	snapshotRetryBase = 0
	t.Cleanup(func() { snapshotRetryBase = 100 * time.Millisecond })

	snap := &countingSnapshotter{succeedAt: 1, snap: &domain.LeaderboardSnapshot{ID: 1}}
	retrySnapshot(context.Background(), 5, 10, snap, zap.NewNop())

	if snap.calls != 1 {
		t.Errorf("expected 1 call on immediate success, got %d", snap.calls)
	}
}

func TestRetrySnapshot_SucceedsOnSecondAttempt_RetriesOnce(t *testing.T) {
	snapshotRetryBase = 0
	t.Cleanup(func() { snapshotRetryBase = 100 * time.Millisecond })

	snap := &countingSnapshotter{
		succeedAt: 2,
		snap:      &domain.LeaderboardSnapshot{ID: 1},
		err:       errors.New("transient"),
	}
	retrySnapshot(context.Background(), 5, 10, snap, zap.NewNop())

	if snap.calls != 2 {
		t.Errorf("expected 2 calls (1 fail + 1 success), got %d", snap.calls)
	}
}

func TestRetrySnapshot_AllAttemptsFail_LogsAndReturns(t *testing.T) {
	snapshotRetryBase = 0
	t.Cleanup(func() { snapshotRetryBase = 100 * time.Millisecond })

	snap := &countingSnapshotter{succeedAt: 0, err: errors.New("db down")}
	retrySnapshot(context.Background(), 5, 10, snap, zap.NewNop())

	if snap.calls != maxSnapshotAttempts {
		t.Errorf("expected %d calls on total failure, got %d", maxSnapshotAttempts, snap.calls)
	}
}

func TestRetrySnapshot_ContextCancelled_StopsEarly(t *testing.T) {
	snapshotRetryBase = time.Hour // would block forever without context cancellation
	t.Cleanup(func() { snapshotRetryBase = 100 * time.Millisecond })

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancelled

	snap := &countingSnapshotter{succeedAt: 0, err: errors.New("transient")}
	retrySnapshot(ctx, 5, 10, snap, zap.NewNop())

	// First attempt always runs; retry sleep is skipped because ctx is done.
	if snap.calls < 1 {
		t.Error("expected at least one attempt before context cancellation")
	}
}

// stubInvalidator is a PostScoringInvalidator stub that records whether it was
// called and delegates to an optional fn so tests can inspect the quinielaIDs.
type stubInvalidator struct {
	fn func(ids []int)
}

func (s *stubInvalidator) InvalidateAfterScoring(_ context.Context, ids []int) {
	if s.fn != nil {
		s.fn(ids)
	}
}

// countingSnapshotter counts Snapshot calls and returns success starting at
// the nth call (succeedAt). Zero succeedAt means always return err.
type countingSnapshotter struct {
	calls     int
	succeedAt int
	snap      *domain.LeaderboardSnapshot
	err       error
}

func (s *countingSnapshotter) Snapshot(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	s.calls++
	if s.succeedAt > 0 && s.calls >= s.succeedAt {
		return s.snap, nil
	}
	return nil, s.err
}
func (s *countingSnapshotter) SnapshotForMatch(_ context.Context, _, _ int) (*domain.LeaderboardSnapshot, error) {
	s.calls++
	if s.succeedAt > 0 && s.calls >= s.succeedAt {
		return s.snap, nil
	}
	return nil, s.err
}

// ── snapshotSem (global weighted semaphore) ───────────────────────────────────

func TestPostScoringWork_WithSemaphore_SnapshotsAllQuinielas(t *testing.T) {
	old := snapshotSem
	snapshotSem = &localSnapshotSem{semaphore.NewWeighted(2)}
	t.Cleanup(func() { snapshotSem = old })

	oldBase := snapshotRetryBase
	snapshotRetryBase = 0
	t.Cleanup(func() { snapshotRetryBase = oldBase })

	predRepo := &stubWorkerPredRepo{ids: []int{1, 2, 3}}
	snap := &stubSnapshotter{snap: &domain.LeaderboardSnapshot{ID: 1}}
	postScoringWork(context.Background(), 5, snap, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
	// All 3 quinielas must be snapshotted despite the semaphore limiting concurrency to 2.
}

func TestPostScoringWork_WithSemaphore_ContextCancelled_SkipsSnapshot(t *testing.T) {
	old := snapshotSem
	snapshotSem = &localSnapshotSem{semaphore.NewWeighted(1)}
	t.Cleanup(func() { snapshotSem = old })

	// Pre-cancel the context so gctx (derived inside postScoringWork) is
	// immediately cancelled; snapshotSem.Acquire(gctx, 1) then returns
	// context.Canceled and the goroutine exits via the "return nil" path.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	predRepo := &stubWorkerPredRepo{ids: []int{10, 20}}
	snap := &countingSnapshotter{succeedAt: 0, err: errors.New("should not reach")}
	postScoringWork(ctx, 5, snap, predRepo, nil, noopSnapshotLocker{}, zap.NewNop())
	if snap.calls != 0 {
		t.Errorf("SnapshotForMatch calls: got %d; want 0 (context cancelled before acquire)", snap.calls)
	}
}

// ── runSnapshot locker stubs ──────────────────────────────────────────────────

// errorLocker returns a configurable error from TryLock, simulating a Redis
// failure. Snapshot generation must degrade to single-process behaviour and
// still run.
type errorLocker struct{ err error }

func (l errorLocker) TryLock(_ context.Context, _, _ int) (bool, error) { return false, l.err }
func (l errorLocker) Unlock(_ context.Context, _, _ int) error          { return nil }

// refuseLocker always returns (false, nil), simulating another replica holding
// the distributed snapshot lock. The snapshot must be skipped.
type refuseLocker struct{}

func (refuseLocker) TryLock(_ context.Context, _, _ int) (bool, error) { return false, nil }
func (refuseLocker) Unlock(_ context.Context, _, _ int) error          { return nil }

// acquireFailUnlockLocker acquires the lock but returns an error on Unlock,
// simulating a transient Redis failure during cleanup. The snapshot must
// complete despite the cleanup failure.
type acquireFailUnlockLocker struct{ unlockErr error }

func (l acquireFailUnlockLocker) TryLock(_ context.Context, _, _ int) (bool, error) {
	return true, nil
}
func (l acquireFailUnlockLocker) Unlock(_ context.Context, _, _ int) error { return l.unlockErr }

// ── runSnapshot ───────────────────────────────────────────────────────────────

func TestRunSnapshot_NilLocker_RunsSnapshot(t *testing.T) {
	snap := &countingSnapshotter{succeedAt: 1, snap: &domain.LeaderboardSnapshot{ID: 1}}
	runSnapshot(context.Background(), 5, 10, snap, nil, zap.NewNop())
	if snap.calls != 1 {
		t.Errorf("expected 1 snapshot call with nil locker, got %d", snap.calls)
	}
}

func TestRunSnapshot_LockError_DegradesToLocalAndRunsSnapshot(t *testing.T) {
	// A TryLock error must degrade gracefully: log a warning and still run the
	// snapshot under the in-process semaphore (best-effort distributed lock).
	snap := &countingSnapshotter{succeedAt: 1, snap: &domain.LeaderboardSnapshot{ID: 1}}
	runSnapshot(context.Background(), 5, 10, snap, errorLocker{err: errors.New("redis down")}, zap.NewNop())
	if snap.calls != 1 {
		t.Errorf("expected 1 snapshot call when lock fails, got %d", snap.calls)
	}
}

func TestRunSnapshot_LockNotAcquired_SkipsSnapshot(t *testing.T) {
	// Another replica holds the lock; this replica must not run the snapshot.
	snap := &countingSnapshotter{succeedAt: 1, snap: &domain.LeaderboardSnapshot{ID: 1}}
	runSnapshot(context.Background(), 5, 10, snap, refuseLocker{}, zap.NewNop())
	if snap.calls != 0 {
		t.Errorf("expected 0 snapshot calls when lock refused, got %d", snap.calls)
	}
}

func TestRunSnapshot_UnlockFails_SnapshotStillRuns(t *testing.T) {
	// Unlock failure must be logged and swallowed; the snapshot must complete
	// because scoring already committed before runSnapshot was called.
	snap := &countingSnapshotter{succeedAt: 1, snap: &domain.LeaderboardSnapshot{ID: 1}}
	locker := acquireFailUnlockLocker{unlockErr: errors.New("redis del failed")}
	runSnapshot(context.Background(), 5, 10, snap, locker, zap.NewNop())
	if snap.calls != 1 {
		t.Errorf("expected 1 snapshot call when unlock fails, got %d", snap.calls)
	}
}

// ── redisSnapshotLocker ───────────────────────────────────────────────────────

// newTestRedisLocker starts a miniredis server, wires a redisSnapshotLocker to
// it, and registers cleanup. The Miniredis handle is returned so callers can
// manipulate server state (e.g. Close to simulate Redis downtime).
func newTestRedisLocker(t *testing.T) (*redisSnapshotLocker, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return &redisSnapshotLocker{client: client, ttl: time.Minute}, mr
}

func TestRedisSnapshotLocker_TryLock_AcquiresKey(t *testing.T) {
	locker, _ := newTestRedisLocker(t)
	ok, err := locker.TryLock(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !ok {
		t.Error("expected lock to be acquired on first TryLock")
	}
}

func TestRedisSnapshotLocker_TryLock_AlreadyLocked_ReturnsFalse(t *testing.T) {
	locker, _ := newTestRedisLocker(t)
	_, _ = locker.TryLock(context.Background(), 1, 2)
	ok, err := locker.TryLock(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if ok {
		t.Error("expected second TryLock to be refused while key is held")
	}
}

func TestRedisSnapshotLocker_TryLock_RedisDown_ReturnsError(t *testing.T) {
	locker, mr := newTestRedisLocker(t)
	mr.Close()
	_, err := locker.TryLock(context.Background(), 1, 2)
	if err == nil {
		t.Error("expected error when Redis is unavailable")
	}
}

func TestRedisSnapshotLocker_Unlock_DeletesKey(t *testing.T) {
	locker, _ := newTestRedisLocker(t)
	_, _ = locker.TryLock(context.Background(), 3, 4)
	if err := locker.Unlock(context.Background(), 3, 4); err != nil {
		t.Fatalf("unexpected Unlock error: %v", err)
	}
	// After Unlock the key must be deleted; a subsequent TryLock must succeed.
	ok, err := locker.TryLock(context.Background(), 3, 4)
	if err != nil || !ok {
		t.Errorf("expected re-acquire after Unlock; ok=%v err=%v", ok, err)
	}
}

func TestRedisSnapshotLocker_Unlock_RedisDown_ReturnsError(t *testing.T) {
	locker, mr := newTestRedisLocker(t)
	_, _ = locker.TryLock(context.Background(), 5, 6)
	mr.Close()
	if err := locker.Unlock(context.Background(), 5, 6); err == nil {
		t.Error("expected error when Redis is unavailable during Unlock")
	}
}

func TestRedisPubNotifier_Notify_PublishesAndReturnsNil(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	n := &redisPubNotifier{client: rc}
	if err := n.Notify(context.Background(), "ch", `{"user_id":1}`); err != nil {
		t.Fatalf("Notify: unexpected error: %v", err)
	}
}

func TestRedisPubNotifier_Notify_RedisDown_ReturnsError(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	mr.Close() // take Redis down

	n := &redisPubNotifier{client: rc}
	if err := n.Notify(context.Background(), "ch", "{}"); err == nil {
		t.Fatal("expected error when Redis is unavailable; got nil")
	}
}
