package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	teamBrazil    = "Brazil"
	teamArgentina = "Argentina"
)

// stubPredRepo implements repository.PredictionRepository with configurable returns.
// byID is returned by GetByID; byUserMatch is returned by GetByUserAndMatch;
// list is returned by ListByUser and ListByMatch.
// Updated predictions are collected in updated so callers can assert on them.
type stubPredRepo struct {
	byID        *domain.Prediction
	byUserMatch *domain.Prediction
	list        []*domain.Prediction
	err         error
	updated     []*domain.Prediction
}

func (r *stubPredRepo) Create(_ context.Context, _ *domain.Prediction) error { return r.err }
func (r *stubPredRepo) GetByID(_ context.Context, _ int) (*domain.Prediction, error) {
	return r.byID, r.err
}
func (r *stubPredRepo) Update(_ context.Context, p *domain.Prediction) error {
	r.updated = append(r.updated, p)
	return r.err
}
func (r *stubPredRepo) GetByUserAndMatch(_ context.Context, _, _ int) (*domain.Prediction, error) {
	return r.byUserMatch, r.err
}
func (r *stubPredRepo) ListByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.list, r.err
}
func (r *stubPredRepo) ListByMatch(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.list, r.err
}
func (r *stubPredRepo) UpdateManyPoints(_ context.Context, points map[int]int) error {
	for _, p := range r.list {
		if pts, ok := points[p.ID]; ok {
			p.Points = &pts
			r.updated = append(r.updated, p)
		}
	}
	return r.err
}
func (r *stubPredRepo) TotalPointsByQuiniela(_ context.Context, _ int) (map[int]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) TotalPointsByQuinielaAndPhase(_ context.Context, _ int, _ domain.MatchPhase) (map[int]int, error) {
	return nil, r.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newPredSvc(match *domain.Match, existingPred *domain.Prediction) PredictionService {
	matchRepo := &stubMatchRepo{match: match}
	predRepo := &stubPredRepo{byUserMatch: existingPred}
	return NewPredictionService(predRepo, matchRepo, &stubPublisher{}, zap.NewNop())
}

func openMatch() *domain.Match {
	return &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusScheduled,
		KickoffAt: time.Now().Add(30 * time.Minute), // well outside the 5-min lock window
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestSubmit_ValidPrediction_ReturnsNil(t *testing.T) {
	svc := newPredSvc(openMatch(), nil)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 2, AwayScore: 1}

	if err := svc.Submit(context.Background(), p); err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestSubmit_MatchNotFound_ReturnsNotFound(t *testing.T) {
	svc := newPredSvc(nil, nil) // matchRepo returns nil, nil
	p := &domain.Prediction{UserID: 1, MatchID: 99, HomeScore: 1, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestSubmit_PastDeadline_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusScheduled,
		KickoffAt: time.Now().Add(3 * time.Minute), // inside the 5-min lock window
	}
	svc := newPredSvc(match, nil)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for deadline, got %v", err)
	}
}

func TestSubmit_DuplicatePrediction_ReturnsConflict(t *testing.T) {
	existing := &domain.Prediction{ID: 5, UserID: 1, MatchID: 1}
	svc := newPredSvc(openMatch(), existing)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 2, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_ValidPrediction_ReturnsUpdated(t *testing.T) {
	match := openMatch()
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: match.ID, HomeScore: 1, AwayScore: 0}
	matchRepo := &stubMatchRepo{match: match}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, matchRepo, &stubPublisher{}, zap.NewNop())

	got, err := svc.Update(context.Background(), 1, 1, 2, 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.HomeScore != 2 || got.AwayScore != 1 {
		t.Errorf("scores not updated: got %d-%d", got.HomeScore, got.AwayScore)
	}
}

func TestUpdate_PredictionNotFound_ReturnsNotFound(t *testing.T) {
	matchRepo := &stubMatchRepo{match: openMatch()}
	predRepo := &stubPredRepo{byID: nil}
	svc := NewPredictionService(predRepo, matchRepo, &stubPublisher{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 99, 1, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUpdate_MatchNotFound_ReturnsNotFound(t *testing.T) {
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 99}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: nil}, &stubPublisher{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 1, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUpdate_PastDeadline_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusScheduled,
		KickoffAt: time.Now().Add(2 * time.Minute), // inside the 5-min lock window
	}
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &stubPublisher{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for deadline, got %v", err)
	}
}

func TestUpdate_OtherUsersPrediction_ReturnsForbidden(t *testing.T) {
	match := openMatch()
	pred := &domain.Prediction{ID: 1, UserID: 2, MatchID: match.ID, HomeScore: 1, AwayScore: 0}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &stubPublisher{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected forbidden error for ownership mismatch, got %v", err)
	}
	if len(predRepo.updated) != 0 {
		t.Errorf("expected no repository update on forbidden change, got %d updates", len(predRepo.updated))
	}
}

// ── GetByUser / GetByMatch ────────────────────────────────────────────────────

func TestGetByUser_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 1, UserID: 1, MatchID: 1}}
	predRepo := &stubPredRepo{list: preds}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &stubPublisher{}, zap.NewNop())

	got, err := svc.GetByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(got))
	}
}

func TestGetByMatch_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 2, UserID: 2, MatchID: 5}}
	predRepo := &stubPredRepo{list: preds}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &stubPublisher{}, zap.NewNop())

	got, err := svc.GetByMatch(context.Background(), 5)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 prediction, got %d", len(got))
	}
}
