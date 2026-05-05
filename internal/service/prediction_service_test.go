package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

const (
	pred1PredFmt  = "expected 1 prediction, got %d"
	teamBrazil    = "Brazil"
	teamArgentina = "Argentina"
)

// stubPredRepo implements repository.PredictionRepository with configurable returns.
// byID is returned by GetByID; list is returned by ListByUser and ListByMatch.
// Updated predictions are collected in updated so callers can assert on them.
// err is returned by Create and other mutating calls; set it to apperrors.Conflict
// to simulate the unique-constraint violation that the database raises on duplicate
// submissions (the service no longer does a pre-INSERT existence check).
type stubPredRepo struct {
	byID        *domain.Prediction
	byUserMatch *domain.Prediction // returned by GetByUserAndMatch; kept for interface compliance
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
func (r *stubPredRepo) UpdateIfUnchanged(_ context.Context, p *domain.Prediction, _ time.Time) error {
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
func (r *stubPredRepo) ListByUserAndQuiniela(_ context.Context, _, _ int) ([]*domain.Prediction, error) {
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
func (r *stubPredRepo) PredictionStatsByQuiniela(_ context.Context, _ int) (map[int]*domain.UserPredictionStats, error) {
	return nil, r.err
}
func (r *stubPredRepo) GetUserPredictionCounts(_ context.Context, _ int) (*domain.UserPredictionCounts, error) {
	return nil, r.err
}
func (r *stubPredRepo) GetUserPointsByPhase(_ context.Context, _ int) (map[domain.MatchPhase]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListUserScoredPointsChronological(_ context.Context, _ int) ([]int, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListAdmin(_ context.Context, _ repository.PredictionAdminFilters, _ repository.Pagination) ([]*domain.Prediction, error) {
	return r.list, r.err
}
func (r *stubPredRepo) GlobalLeaderboard(_ context.Context, _ int) ([]*domain.GlobalLeaderboardEntry, error) {
	return nil, r.err
}
func (r *stubPredRepo) ListQuinielaIDsByMatch(_ context.Context, _ int) ([]int, error) {
	return nil, r.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newPredSvc(match *domain.Match) PredictionService {
	matchRepo := &stubMatchRepo{match: match}
	predRepo := &stubPredRepo{}
	return NewPredictionService(predRepo, matchRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())
}

func openMatch() *domain.Match {
	return &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusScheduled,
		KickoffAt: time.Now().Add(30 * time.Minute), // well outside the 5-min lock window
	}
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestSubmit_ValidPrediction_ReturnsNil(t *testing.T) {
	svc := newPredSvc(openMatch())
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 2, AwayScore: 1}

	if err := svc.Submit(context.Background(), p); err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestSubmit_MatchNotFound_ReturnsNotFound(t *testing.T) {
	svc := newPredSvc(nil) // matchRepo returns nil, nil
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
	svc := newPredSvc(match)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for deadline, got %v", err)
	}
}

func TestSubmit_LiveMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusLive,
		KickoffAt: time.Now().Add(30 * time.Minute),
	}
	svc := newPredSvc(match)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for live match, got %v", err)
	}
}

func TestSubmit_FinishedMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusFinished,
		KickoffAt: time.Now().Add(-2 * time.Hour),
	}
	svc := newPredSvc(match)
	p := &domain.Prediction{UserID: 1, MatchID: 1, HomeScore: 0, AwayScore: 0}

	if err := svc.Submit(context.Background(), p); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

// TestSubmit_DuplicatePrediction_ReturnsConflict verifies that the Conflict error
// surfaced by the repository's unique-constraint handler propagates correctly.
// The service no longer does a pre-INSERT existence check; atomicity is delegated
// entirely to the database unique index on (user_id, match_id).
func TestSubmit_DuplicatePrediction_ReturnsConflict(t *testing.T) {
	matchRepo := &stubMatchRepo{match: openMatch()}
	predRepo := &stubPredRepo{err: apperrors.Conflict("a prediction for this match has already been submitted")}
	svc := NewPredictionService(predRepo, matchRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())
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
	svc := NewPredictionService(predRepo, matchRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

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
	svc := NewPredictionService(predRepo, matchRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 99, 1, 0); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestUpdate_MatchNotFound_ReturnsNotFound(t *testing.T) {
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 99}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: nil}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

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
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for deadline, got %v", err)
	}
}

func TestUpdate_OtherUsersPrediction_ReturnsForbidden(t *testing.T) {
	match := openMatch()
	pred := &domain.Prediction{ID: 1, UserID: 2, MatchID: match.ID, HomeScore: 1, AwayScore: 0}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected forbidden error for ownership mismatch, got %v", err)
	}
	if len(predRepo.updated) != 0 {
		t.Errorf("expected no repository update on forbidden change, got %d updates", len(predRepo.updated))
	}
}

func TestUpdate_LiveMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusLive,
		KickoffAt: time.Now().Add(30 * time.Minute),
	}
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for live match, got %v", err)
	}
}

func TestUpdate_FinishedMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusFinished,
		KickoffAt: time.Now().Add(-2 * time.Hour),
	}
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0}
	predRepo := &stubPredRepo{byID: pred}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

func TestUpdate_ConcurrentModification_ReturnsConflict(t *testing.T) {
	match := openMatch()
	pred := &domain.Prediction{ID: 1, UserID: 1, MatchID: 1, HomeScore: 1, AwayScore: 0, UpdatedAt: time.Now()}
	predRepo := &stubPredRepo{
		byID: pred,
		err:  apperrors.Conflict("prediction was modified by another request; please retry"),
	}
	svc := NewPredictionService(predRepo, &stubMatchRepo{match: match}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	if _, err := svc.Update(context.Background(), 1, 1, 2, 1); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error for concurrent modification, got %v", err)
	}
}

// ── GetByUser / GetByMatch ────────────────────────────────────────────────────

func TestGetByUser_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 1, UserID: 1, MatchID: 1}}
	predRepo := &stubPredRepo{list: preds}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	got, err := svc.GetByUser(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf(pred1PredFmt, len(got))
	}
}

func TestGetByMatch_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 2, UserID: 2, MatchID: 5}}
	predRepo := &stubPredRepo{list: preds}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	got, err := svc.GetByMatch(context.Background(), 5)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf(pred1PredFmt, len(got))
	}
}

// ── GetByUserAndQuiniela ──────────────────────────────────────────────────────

func TestGetByUserAndQuiniela_MemberWithPredictions_ReturnsSlice(t *testing.T) {
	preds := []*domain.Prediction{{ID: 1, UserID: 1, MatchID: 3}}
	predRepo := &stubPredRepo{list: preds}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	got, err := svc.GetByUserAndQuiniela(context.Background(), 1, 7)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf(pred1PredFmt, len(got))
	}
}

func TestGetByUserAndQuiniela_NonMember_ReturnsEmptySlice(t *testing.T) {
	// Repository returns [] when the EXISTS membership check fails.
	predRepo := &stubPredRepo{list: []*domain.Prediction{}}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	got, err := svc.GetByUserAndQuiniela(context.Background(), 1, 99)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice for non-member, got %d", len(got))
	}
}

func TestGetByUserAndQuiniela_RepoError_Propagated(t *testing.T) {
	predRepo := &stubPredRepo{err: errors.New("db error")}
	svc := NewPredictionService(predRepo, &stubMatchRepo{}, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	_, err := svc.GetByUserAndQuiniela(context.Background(), 1, 7)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}
