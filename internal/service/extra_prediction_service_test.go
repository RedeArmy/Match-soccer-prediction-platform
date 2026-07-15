package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// newExtraPredSvc defaults the user's own scoreline prediction to 1-0, which
// satisfies most existing first_scorer="home" test cases under the new
// prediction cross-validation. Tests needing a different scoreline (e.g. a
// 0-0 prediction to allow first_scorer="none") should use
// newExtraPredSvcWithPrediction directly.
func newExtraPredSvc(match *domain.Match) (ExtraPredictionService, *stubExtraPredRepo) {
	return newExtraPredSvcWithPrediction(match, &domain.Prediction{HomeScore: 1, AwayScore: 0})
}

func newExtraPredSvcWithPrediction(match *domain.Match, prediction *domain.Prediction) (ExtraPredictionService, *stubExtraPredRepo) {
	matchRepo := &stubMatchRepo{match: match}
	extraRepo := &stubExtraPredRepo{}
	predRepo := &stubPredRepo{byUserMatch: prediction}
	return NewExtraPredictionService(extraRepo, matchRepo, predRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop()), extraRepo
}

// ── Submit ────────────────────────────────────────────────────────────────────

func TestExtraSubmit_ValidGuess_ReturnsNil(t *testing.T) {
	svc, _ := newExtraPredSvc(openMatch())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestExtraSubmit_UnknownExtraType_ReturnsValidation(t *testing.T) {
	svc, _ := newExtraPredSvc(openMatch())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraType("bogus_type"), "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for unknown extra_type, got %v", err)
	}
}

func TestExtraSubmit_InvalidAnswerForType_ReturnsValidation(t *testing.T) {
	svc, _ := newExtraPredSvc(openMatch())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeHalftimeResult, "home_team_wins") // not a valid "home-away" scoreline
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for invalid answer, got %v", err)
	}
}

func TestExtraSubmit_FirstScorerAllowsNone_ValidAnswer(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 0, AwayScore: 0})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "none")
	if err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestExtraSubmit_MatchNotFound_ReturnsNotFound(t *testing.T) {
	svc, _ := newExtraPredSvc(nil)

	_, err := svc.Submit(context.Background(), 1, 99, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestExtraSubmit_MatchRepoError_Propagates(t *testing.T) {
	matchRepo := &stubMatchRepo{err: errors.New("db down")}
	extraRepo := &stubExtraPredRepo{}
	predRepo := &stubPredRepo{byUserMatch: &domain.Prediction{HomeScore: 1, AwayScore: 0}}
	svc := NewExtraPredictionService(extraRepo, matchRepo, predRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExtraSubmit_PastDeadline_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusScheduled,
		KickoffAt: time.Now().Add(3 * time.Minute), // inside the 5-min lock window
	}
	svc, _ := newExtraPredSvc(match)

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for deadline, got %v", err)
	}
}

func TestExtraSubmit_LiveMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusLive,
		KickoffAt: time.Now().Add(30 * time.Minute),
	}
	svc, _ := newExtraPredSvc(match)

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for live match, got %v", err)
	}
}

func TestExtraSubmit_FinishedMatch_ReturnsValidation(t *testing.T) {
	match := &domain.Match{ID: 1, HomeTeam: teamBrazil, AwayTeam: teamArgentina,
		Status:    domain.MatchStatusFinished,
		KickoffAt: time.Now().Add(-2 * time.Hour),
	}
	svc, _ := newExtraPredSvc(match)

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for finished match, got %v", err)
	}
}

func TestExtraSubmit_UpsertError_Propagates(t *testing.T) {
	matchRepo := &stubMatchRepo{match: openMatch()}
	extraRepo := &stubExtraPredRepo{err: errors.New("db down")}
	predRepo := &stubPredRepo{byUserMatch: &domain.Prediction{HomeScore: 1, AwayScore: 0}}
	svc := NewExtraPredictionService(extraRepo, matchRepo, predRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── Cross-validation against the user's own scoreline prediction ────────────

func TestExtraSubmit_NoSavedPrediction_ReturnsValidation(t *testing.T) {
	matchRepo := &stubMatchRepo{match: openMatch()}
	extraRepo := &stubExtraPredRepo{}
	predRepo := &stubPredRepo{byUserMatch: nil}
	svc := NewExtraPredictionService(extraRepo, matchRepo, predRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error when no prediction exists yet, got %v", err)
	}
}

func TestExtraSubmit_PredictionRepoError_Propagates(t *testing.T) {
	matchRepo := &stubMatchRepo{match: openMatch()}
	extraRepo := &stubExtraPredRepo{}
	predRepo := &stubPredRepo{err: errors.New("db down")}
	svc := NewExtraPredictionService(extraRepo, matchRepo, predRepo, &noopSystemParamService{}, clock.Real{}, zap.NewNop())

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestExtraSubmit_FirstScorerInconsistentWithZeroZeroPrediction_ReturnsValidation(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 0, AwayScore: 0})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeFirstScorer, "home")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for first_scorer=home against a 0-0 prediction, got %v", err)
	}
}

func TestExtraSubmit_TeamScoresInconsistentWithPredictedZeroGoals_ReturnsValidation(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 1, AwayScore: 0})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeAwayTeamScores, "first_half")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for away_team_scores=first_half against a predicted 0 away goals, got %v", err)
	}
}

func TestExtraSubmit_TeamScoresConsistentWithPredictedGoals_Succeeds(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 2, AwayScore: 0})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeHomeTeamScores, "both_halves")
	if err != nil {
		t.Errorf(fmtExpectNil, err)
	}
	_, err = svc.Submit(context.Background(), 1, 1, domain.ExtraTypeAwayTeamScores, "none")
	if err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestExtraSubmit_HalftimeExceedsPredictedFinalScore_ReturnsValidation(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 1, AwayScore: 0})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeHalftimeResult, "2-0")
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for halftime 2-0 exceeding predicted final 1-0, got %v", err)
	}
}

func TestExtraSubmit_HalftimeWithinPredictedFinalScore_Succeeds(t *testing.T) {
	svc, _ := newExtraPredSvcWithPrediction(openMatch(), &domain.Prediction{HomeScore: 2, AwayScore: 1})

	_, err := svc.Submit(context.Background(), 1, 1, domain.ExtraTypeHalftimeResult, "1-1")
	if err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

// ── GetByUserAndMatch / ListByUserAndMatches ─────────────────────────────────

func TestExtraGetByUserAndMatch_ReturnsSlice(t *testing.T) {
	svc, extraRepo := newExtraPredSvc(openMatch())
	extraRepo.list = []*domain.ExtraPrediction{
		{ID: 1, UserID: 1, MatchID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
	}

	got, err := svc.GetByUserAndMatch(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 extra prediction, got %d", len(got))
	}
}

func TestExtraListByUserAndMatches_ReturnsSlice(t *testing.T) {
	svc, extraRepo := newExtraPredSvc(openMatch())
	extraRepo.list = []*domain.ExtraPrediction{
		{ID: 1, UserID: 1, MatchID: 1, ExtraType: domain.ExtraTypeFirstScorer, Answer: "home"},
		{ID: 2, UserID: 1, MatchID: 2, ExtraType: domain.ExtraTypeHalftimeResult, Answer: "1-1"},
	}

	got, err := svc.ListByUserAndMatches(context.Background(), 1, []int{1, 2})
	if err != nil {
		t.Fatalf(fmtExpectNilErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 extra predictions, got %d", len(got))
	}
}
