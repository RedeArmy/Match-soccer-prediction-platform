package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// TestCalculatePoints verifies the full scoring decision table:
//
//	5 pts — exact scoreline
//	3 pts — correct non-draw outcome + correct goal margin
//	2 pts — correct non-draw outcome, wrong goal margin
//	2 pts — correct draw (no goal-difference bonus for draws)
//	0 pts — incorrect outcome
func TestCalculatePoints(t *testing.T) {
	const (
		fmtPoints = "calculatePoints(%d-%d | actual %d-%d): expected %d pts, got %d"
	)

	cases := []struct {
		name     string
		predHome int
		predAway int
		realHome int
		realAway int
		wantPts  int
	}{
		// ── Tier 1: exact scoreline ───────────────────────────────────────────
		{
			name:     "exact_home_win",
			predHome: 2, predAway: 1,
			realHome: 2, realAway: 1,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name:     "exact_away_win",
			predHome: 0, predAway: 3,
			realHome: 0, realAway: 3,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name:     "exact_draw",
			predHome: 1, predAway: 1,
			realHome: 1, realAway: 1,
			wantPts: domain.PointsExactScore, // 5
		},
		{
			name:     "exact_goalless_draw",
			predHome: 0, predAway: 0,
			realHome: 0, realAway: 0,
			wantPts: domain.PointsExactScore, // 5
		},

		// ── Tier 2a: correct non-draw outcome + correct goal margin ───────────
		{
			name:     "home_win_correct_margin_1",
			predHome: 1, predAway: 0,
			realHome: 2, realAway: 1, // both margin = 1
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name:     "home_win_correct_margin_2",
			predHome: 2, predAway: 0,
			realHome: 3, realAway: 1, // both margin = 2
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name:     "away_win_correct_margin_1",
			predHome: 0, predAway: 1,
			realHome: 1, realAway: 2, // both margin = 1
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},
		{
			name:     "away_win_correct_margin_3",
			predHome: 0, predAway: 3,
			realHome: 1, realAway: 4, // both margin = 3
			wantPts: domain.PointsCorrectOutcome + domain.PointsGoalDifference, // 3
		},

		// ── Tier 2b: correct non-draw outcome, wrong goal margin ──────────────
		{
			name:     "home_win_wrong_margin",
			predHome: 3, predAway: 0, // margin 3
			realHome: 2, realAway: 1, // margin 1
			wantPts: domain.PointsCorrectOutcome, // 2
		},
		{
			name:     "away_win_wrong_margin",
			predHome: 0, predAway: 1, // margin 1
			realHome: 0, realAway: 3, // margin 3
			wantPts: domain.PointsCorrectOutcome, // 2
		},

		// ── Tier 2c: correct draw — no goal-difference bonus ──────────────────
		{
			name:     "correct_draw_no_margin_bonus",
			predHome: 2, predAway: 2,
			realHome: 1, realAway: 1, // both draw, both margin 0 — bonus NOT awarded
			wantPts: domain.PointsCorrectOutcome, // 2, not 3
		},
		{
			name:     "correct_draw_different_scores",
			predHome: 3, predAway: 3,
			realHome: 0, realAway: 0,
			wantPts: domain.PointsCorrectOutcome, // 2
		},

		// ── Tier 3: incorrect outcome ─────────────────────────────────────────
		{
			name:     "predicted_home_win_actual_draw",
			predHome: 2, predAway: 0,
			realHome: 1, realAway: 1,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name:     "predicted_draw_actual_home_win",
			predHome: 1, predAway: 1,
			realHome: 2, realAway: 0,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name:     "predicted_home_win_actual_away_win",
			predHome: 2, predAway: 1,
			realHome: 0, realAway: 1,
			wantPts: domain.PointsIncorrectResult, // 0
		},
		{
			name:     "predicted_away_win_actual_draw",
			predHome: 0, predAway: 2,
			realHome: 2, realAway: 2,
			wantPts: domain.PointsIncorrectResult, // 0
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pred := &domain.Prediction{HomeScore: tc.predHome, AwayScore: tc.predAway}
			got := calculatePoints(pred, tc.realHome, tc.realAway)
			if got != tc.wantPts {
				t.Errorf(fmtPoints,
					tc.predHome, tc.predAway,
					tc.realHome, tc.realAway,
					tc.wantPts, got,
				)
			}
		})
	}
}

// ── ScoreMatch ────────────────────────────────────────────────────────────────

func TestScoreMatch_FinishedMatch_CalculatesAndPersistsPoints(t *testing.T) {
	home, away := 2, 1
	match := &domain.Match{
		ID: 1, Status: domain.MatchStatusFinished,
		HomeScore: &home, AwayScore: &away,
	}
	preds := []*domain.Prediction{
		{ID: 1, HomeScore: 2, AwayScore: 1}, // exact → 5
		{ID: 2, HomeScore: 1, AwayScore: 0}, // correct outcome + margin 1 → 3
		{ID: 3, HomeScore: 0, AwayScore: 1}, // wrong outcome → 0
	}
	predRepo := &stubPredRepo{list: preds}
	svc := NewScoringService(&stubMatchRepo{match: match}, predRepo, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(predRepo.updated) != 3 {
		t.Errorf("expected 3 updated predictions, got %d", len(predRepo.updated))
	}
}

func TestScoreMatch_MatchNotFound_ReturnsNotFound(t *testing.T) {
	svc := NewScoringService(&stubMatchRepo{match: nil}, &stubPredRepo{}, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestScoreMatch_MatchNotFinished_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, Status: domain.MatchStatusLive}
	svc := NewScoringService(&stubMatchRepo{match: match}, &stubPredRepo{}, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for non-finished match, got %v", err)
	}
}

func TestScoreMatch_NilScores_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, Status: domain.MatchStatusFinished} // HomeScore/AwayScore are nil
	svc := NewScoringService(&stubMatchRepo{match: match}, &stubPredRepo{}, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for nil scores, got %v", err)
	}
}

func TestScoreMatch_MatchRepoError_PropagatesError(t *testing.T) {
	repoErr := errors.New("db timeout")
	svc := NewScoringService(&stubMatchRepo{err: repoErr}, &stubPredRepo{}, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); !errors.Is(err, repoErr) {
		t.Errorf("expected repo error to propagate, got %v", err)
	}
}

func TestScoreMatch_NoPredictions_ReturnsNil(t *testing.T) {
	home, away := 1, 0
	match := &domain.Match{ID: 1, Status: domain.MatchStatusFinished, HomeScore: &home, AwayScore: &away}
	predRepo := &stubPredRepo{list: nil} // empty — no predictions for this match
	svc := NewScoringService(&stubMatchRepo{match: match}, predRepo, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); err != nil {
		t.Errorf("expected nil when no predictions exist, got %v", err)
	}
}

func TestScoreMatch_PredListError_PropagatesError(t *testing.T) {
	home, away := 2, 0
	match := &domain.Match{ID: 1, Status: domain.MatchStatusFinished, HomeScore: &home, AwayScore: &away}
	repoErr := errors.New("query failed")
	predRepo := &stubPredRepo{err: repoErr}
	svc := NewScoringService(&stubMatchRepo{match: match}, predRepo, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); !errors.Is(err, repoErr) {
		t.Errorf("expected pred repo error to propagate, got %v", err)
	}
}

func TestScoreMatch_UpdateManyPointsError_PropagatesError(t *testing.T) {
	home, away := 1, 1
	match := &domain.Match{ID: 1, Status: domain.MatchStatusFinished, HomeScore: &home, AwayScore: &away}
	updateErr := errors.New("tx rollback")
	predRepo := &failOnUpdateRepo{
		list:      []*domain.Prediction{{ID: 1, HomeScore: 1, AwayScore: 1}},
		updateErr: updateErr,
	}
	svc := NewScoringService(&stubMatchRepo{match: match}, predRepo, zap.NewNop())

	if err := svc.ScoreMatch(context.Background(), 1); !errors.Is(err, updateErr) {
		t.Errorf("expected update error to propagate, got %v", err)
	}
}

// failOnUpdateRepo succeeds for all reads but fails UpdateManyPoints.
type failOnUpdateRepo struct {
	list      []*domain.Prediction
	updateErr error
}

func (r *failOnUpdateRepo) Create(_ context.Context, _ *domain.Prediction) error { return nil }
func (r *failOnUpdateRepo) GetByID(_ context.Context, _ int) (*domain.Prediction, error) {
	return nil, nil
}
func (r *failOnUpdateRepo) Update(_ context.Context, _ *domain.Prediction) error { return nil }
func (r *failOnUpdateRepo) GetByUserAndMatch(_ context.Context, _, _ int) (*domain.Prediction, error) {
	return nil, nil
}
func (r *failOnUpdateRepo) ListByUser(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.list, nil
}
func (r *failOnUpdateRepo) ListByMatch(_ context.Context, _ int) ([]*domain.Prediction, error) {
	return r.list, nil
}
func (r *failOnUpdateRepo) UpdateManyPoints(_ context.Context, _ map[int]int) error {
	return r.updateErr
}
func (r *failOnUpdateRepo) TotalPointsByQuiniela(_ context.Context, _ int) (map[int]int, error) {
	return nil, nil
}
func (r *failOnUpdateRepo) TotalPointsByQuinielaAndPhase(_ context.Context, _ int, _ domain.MatchPhase) (map[int]int, error) {
	return nil, nil
}

// ── TestGoalDiff verifies the absolute-value goal-margin helper.
func TestGoalDiff(t *testing.T) {
	cases := []struct{ home, away, want int }{
		{3, 1, 2},
		{1, 3, 2},
		{0, 0, 0},
		{1, 1, 0},
		{5, 0, 5},
	}
	for _, tc := range cases {
		if got := goalDiff(tc.home, tc.away); got != tc.want {
			t.Errorf("goalDiff(%d, %d) = %d, want %d", tc.home, tc.away, got, tc.want)
		}
	}
}
