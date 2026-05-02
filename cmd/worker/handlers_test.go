package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
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
	h := newMatchFinishedHandler(scorer, nil, nil, zap.NewNop())

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
	h := newMatchFinishedHandler(scorer, nil, nil, zap.NewNop())

	if err := h(context.Background(), envelopeWithStruct(5)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 || scorer.lastID != 5 {
		t.Errorf("expected ScoreMatch(5), got called=%d id=%d", scorer.called, scorer.lastID)
	}
}

func TestMatchFinishedHandler_ScorerError_PropagatesError(t *testing.T) {
	scorer := &stubScorer{err: errors.New("db down")}
	h := newMatchFinishedHandler(scorer, nil, nil, zap.NewNop())

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
	h := newMatchFinishedHandler(scorer, nil, nil, zap.NewNop())

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

// ── snapshotAffectedQuinielas ─────────────────────────────────────────────────

// stubSnapshotter implements service.Snapshotter for worker snapshot tests.
type stubSnapshotter struct {
	snap *domain.LeaderboardSnapshot
	err  error
}

func (s *stubSnapshotter) Snapshot(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
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
func (r *stubWorkerPredRepo) GetByID(_ context.Context, _ int) (*domain.Prediction, error) {
	return nil, nil
}
func (r *stubWorkerPredRepo) Update(_ context.Context, _ *domain.Prediction) error { return nil }
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

func TestSnapshotAffectedQuinielas_NilPredRepo_Noop(t *testing.T) {
	snapshotAffectedQuinielas(context.Background(), 1, &stubSnapshotter{}, nil, zap.NewNop())
}

func TestSnapshotAffectedQuinielas_NilSnapshotter_Noop(t *testing.T) {
	snapshotAffectedQuinielas(context.Background(), 1, nil, &stubWorkerPredRepo{ids: []int{1}}, zap.NewNop())
}

func TestSnapshotAffectedQuinielas_ListError_LogsWarnAndReturns(t *testing.T) {
	predRepo := &stubWorkerPredRepo{err: errors.New("db down")}
	snapshotAffectedQuinielas(context.Background(), 1, &stubSnapshotter{}, predRepo, zap.NewNop())
}

func TestSnapshotAffectedQuinielas_EmptyList_NoSnapshot(t *testing.T) {
	predRepo := &stubWorkerPredRepo{ids: []int{}}
	snapshotAffectedQuinielas(context.Background(), 1, &stubSnapshotter{}, predRepo, zap.NewNop())
}

func TestSnapshotAffectedQuinielas_SnapshotSuccess_LogsInfo(t *testing.T) {
	predRepo := &stubWorkerPredRepo{ids: []int{10, 20}}
	snap := &stubSnapshotter{snap: &domain.LeaderboardSnapshot{ID: 1}}
	snapshotAffectedQuinielas(context.Background(), 5, snap, predRepo, zap.NewNop())
}

func TestSnapshotAffectedQuinielas_SnapshotError_LogsWarn(t *testing.T) {
	predRepo := &stubWorkerPredRepo{ids: []int{10}}
	snap := &stubSnapshotter{err: errors.New("snapshot failed")}
	snapshotAffectedQuinielas(context.Background(), 5, snap, predRepo, zap.NewNop())
}

func TestMatchFinishedHandler_WithSnapshot_ScoresAndSnapshots(t *testing.T) {
	scorer := &stubScorer{}
	predRepo := &stubWorkerPredRepo{ids: []int{1, 2}}
	snap := &stubSnapshotter{snap: &domain.LeaderboardSnapshot{ID: 1}}
	h := newMatchFinishedHandler(scorer, snap, predRepo, zap.NewNop())

	if err := h(context.Background(), envelopeWithStruct(10)); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if scorer.called != 1 || scorer.lastID != 10 {
		t.Errorf("expected ScoreMatch(10) called once, got called=%d id=%d", scorer.called, scorer.lastID)
	}
}
