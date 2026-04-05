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

// ── stubs ─────────────────────────────────────────────────────────────────────

// stubMatchRepo implements repository.MatchRepository with configurable returns.
type stubMatchRepo struct {
	match *domain.Match
	err   error
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
	return nil, r.err
}
func (r *stubMatchRepo) ListByStatus(_ context.Context, _ domain.MatchStatus) ([]*domain.Match, error) {
	return nil, r.err
}

// stubPublisher records published envelopes without delivering them.
type stubPublisher struct {
	published []events.Envelope
}

func (p *stubPublisher) Publish(_ context.Context, env events.Envelope) error {
	p.published = append(p.published, env)
	return nil
}

func newMatchSvc(match *domain.Match) (MatchService, *stubPublisher) {
	pub := &stubPublisher{}
	svc := NewMatchService(&stubMatchRepo{match: match}, pub, zap.NewNop())
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
		t.Fatalf("expected nil error, got %v", err)
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
		Status: domain.MatchStatusFinished,
		HomeScore: &home, AwayScore: &away,
		KickoffAt: time.Now().Add(-2 * time.Hour)}
	svc, _ := newMatchSvc(match)

	_, err := svc.UpdateResult(context.Background(), 1, 3, 0)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

// ── StartMatch — status guard ─────────────────────────────────────────────────

func TestStartMatch_ScheduledMatch_TransitionsToLiveAndEmitsEvent(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: "Brazil", AwayTeam: "Argentina",
		Status: domain.MatchStatusScheduled, KickoffAt: time.Now()}
	svc, pub := newMatchSvc(match)

	result, err := svc.StartMatch(context.Background(), 1)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
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
