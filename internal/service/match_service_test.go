package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	fmtExpectNil    = "expected nil, got %v"
	fmtExpectNilErr = "expected nil error, got %v"
	fmtExpect1Match = "expected 1 match, got %d"
	fmtNotFoundErr  = "expected not-found error, got %v"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

// stubMatchRepo implements repository.MatchRepository with configurable returns.
type stubMatchRepo struct {
	match   *domain.Match
	matches []*domain.Match
	err     error
}

func (r *stubMatchRepo) Create(_ context.Context, _ *domain.Match) error { return r.err }
func (r *stubMatchRepo) GetByID(_ context.Context, _ int) (*domain.Match, error) {
	return r.match, r.err
}
func (r *stubMatchRepo) Update(_ context.Context, m *domain.Match) error {
	if r.err != nil {
		return r.err
	}
	r.match = m
	return nil
}
func (r *stubMatchRepo) List(_ context.Context) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) ListByPhase(_ context.Context, _ domain.MatchPhase) ([]*domain.Match, error) {
	return r.matches, r.err
}
func (r *stubMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return r.matches, r.err
}

// stubPublisher records published envelopes without delivering them.
type stubPublisher struct {
	published []events.Envelope
	err       error
}

func (p *stubPublisher) Publish(_ context.Context, env events.Envelope) error {
	if p.err != nil {
		return p.err
	}
	p.published = append(p.published, env)
	return nil
}

// stubScorer records ScoreMatch calls.
type stubScorer struct {
	called []int
	err    error
}

func (s *stubScorer) ScoreMatch(_ context.Context, matchID int) error {
	s.called = append(s.called, matchID)
	return s.err
}

func newMatchSvc(match *domain.Match) (MatchService, *stubPublisher) {
	pub := &stubPublisher{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, &stubScorer{}, zap.NewNop())
	return svc, pub
}

// ── UpdateResult — status guard ───────────────────────────────────────────────

// TestUpdateResult_LiveMatch_ConfirmsResultAndEmitsEvent is the happy path:
// a live match can receive a final score and emits MatchFinished.
func TestUpdateResult_LiveMatch_ConfirmsResultAndEmitsEvent(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}
	svc, pub := newMatchSvc(match)

	result, err := svc.UpdateResult(context.Background(), 1, 2, 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("expected status finished, got %s", result.Status)
	}
	if len(pub.published) != 1 || pub.published[0].Type != events.EventMatchFinished {
		t.Errorf("expected one MatchFinished event, got %v", pub.published)
	}
}

// TestUpdateResult_ScheduledMatch_ReturnsValidationError enforces that a result
// cannot be set before the match is started. The admin must call StartMatch first,
// which closes the prediction window.
func TestUpdateResult_ScheduledMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "France", AwayTeam: "Germany",
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now().Add(time.Hour)}
	svc, _ := newMatchSvc(match)

	_, err := svc.UpdateResult(context.Background(), 1, 1, 0)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for scheduled match, got %v", err)
	}
}

// TestUpdateResult_FinishedMatch_ReturnsValidationError enforces that a confirmed
// result is permanent and cannot be overwritten.
func TestUpdateResult_FinishedMatch_ReturnsValidationError(t *testing.T) {
	home, away := 2, 1
	match := &domain.Match{ID: 1, HomeTeam: "Spain", AwayTeam: "England",
		Status:    domain.MatchStatusFinished,
		HomeScore: &home, AwayScore: &away,
		KickoffAt: time.Now().Add(-2 * time.Hour)}
	svc, _ := newMatchSvc(match)

	_, err := svc.UpdateResult(context.Background(), 1, 3, 0)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

// TestUpdateResult_PublishFails_FallsBackToSynchronousScoring verifies that when
// the event bus is unavailable, predictions are still scored synchronously so
// no match is ever left unscored due to a transient Redis outage.
func TestUpdateResult_PublishFails_FallsBackToSynchronousScoring(t *testing.T) {
	match := &domain.Match{ID: 42, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}

	pub := &stubPublisher{err: errors.New("redis unavailable")}
	scorer := &stubScorer{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, zap.NewNop())

	result, err := svc.UpdateResult(context.Background(), 42, 2, 1)
	if err != nil {
		t.Fatalf("UpdateResult must succeed even when publish fails: %v", err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("status: want finished, got %s", result.Status)
	}
	if len(pub.published) != 0 {
		t.Errorf("publish must not record events when it returns an error")
	}
	if len(scorer.called) != 1 || scorer.called[0] != 42 {
		t.Errorf("fallback scorer must be called with match ID 42, got %v", scorer.called)
	}
}

// TestUpdateResult_PublishFails_ScorerAlsoFails_StillReturnsResult verifies that
// a double failure (bus down + DB error in scorer) returns the confirmed match
// result to the caller. Both failures are logged; the HTTP response remains 200.
func TestUpdateResult_PublishFails_ScorerAlsoFails_StillReturnsResult(t *testing.T) {
	match := &domain.Match{ID: 7, HomeTeam: "France", AwayTeam: "Germany",
		Status: domain.MatchStatusLive, KickoffAt: time.Now().Add(-time.Hour)}

	pub := &stubPublisher{err: errors.New("redis unavailable")}
	scorer := &stubScorer{err: errors.New("db timeout")}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, scorer, zap.NewNop())

	result, err := svc.UpdateResult(context.Background(), 7, 1, 0)
	if err != nil {
		t.Fatalf("UpdateResult must succeed regardless of scorer failure: %v", err)
	}
	if result.Status != domain.MatchStatusFinished {
		t.Errorf("status: want finished, got %s", result.Status)
	}
}

// ── StartMatch — status guard ─────────────────────────────────────────────────

func TestStartMatch_ScheduledMatch_TransitionsToLiveAndEmitsEvent(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now()}
	svc, pub := newMatchSvc(match)

	result, err := svc.StartMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if result.Status != domain.MatchStatusLive {
		t.Errorf("expected status live, got %s", result.Status)
	}
	if len(pub.published) != 1 || pub.published[0].Type != events.EventMatchStarted {
		t.Errorf("expected one MatchStarted event, got %v", pub.published)
	}
}

func TestStartMatch_LiveMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusLive}
	svc, _ := newMatchSvc(match)

	_, err := svc.StartMatch(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for already-live match, got %v", err)
	}
}

func TestStartMatch_FinishedMatch_ReturnsValidationError(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusFinished}
	svc, _ := newMatchSvc(match)

	_, err := svc.StartMatch(context.Background(), 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

// ── CreateMatch ───────────────────────────────────────────────────────────────

func TestCreateMatch_ValidMatch_ReturnsNil(t *testing.T) {
	svc, _ := newMatchSvc(nil)
	m := &domain.Match{HomeTeam: "Brazil", AwayTeam: "Argentina", KickoffAt: time.Now().Add(24 * time.Hour)}
	if err := svc.CreateMatch(context.Background(), m); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestCreateMatch_InvalidMatch_ReturnsValidation(t *testing.T) {
	svc, _ := newMatchSvc(nil)
	m := &domain.Match{HomeTeam: "", AwayTeam: "Argentina", KickoffAt: time.Now().Add(time.Hour)}
	if err := svc.CreateMatch(context.Background(), m); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for empty home team, got %v", err)
	}
}

// ── GetMatch ──────────────────────────────────────────────────────────────────

func TestGetMatch_Found_ReturnsMatch(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina", Status: domain.MatchStatusScheduled}
	svc, _ := newMatchSvc(match)

	got, err := svc.GetMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if got.ID != 1 {
		t.Errorf("expected match ID 1, got %d", got.ID)
	}
}

func TestGetMatch_NotFound_ReturnsNotFound(t *testing.T) {
	svc, _ := newMatchSvc(nil) // repo returns nil, nil
	_, err := svc.GetMatch(context.Background(), 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── ListMatches / ListMatchesByStatus ─────────────────────────────────────────

func TestListMatches_ReturnsSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina", Status: domain.MatchStatusScheduled},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, zap.NewNop())

	got, err := svc.ListMatches(context.Background())
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}

func TestListMatchesByPhase_ReturnsFilteredSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina", Phase: domain.PhaseGroupStage},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, zap.NewNop())

	got, err := svc.ListMatchesByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}

func TestListMatchesByStatus_ReturnsFilteredSlice(t *testing.T) {
	pub := &stubPublisher{}
	matches := []*domain.Match{
		{ID: 1, HomeTeam: "France", AwayTeam: "Germany", Status: domain.MatchStatusLive},
	}
	svc := NewMatchService(&stubMatchRepo{matches: matches}, pub, &stubScorer{}, zap.NewNop())

	got, err := svc.ListMatchesByStatus(context.Background(), domain.MatchStatusLive)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf(fmtExpect1Match, len(got))
	}
}
