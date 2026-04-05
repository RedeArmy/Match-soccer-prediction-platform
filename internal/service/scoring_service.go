package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// scoringService is the concrete implementation of MatchScorer.
type scoringService struct {
	matchRepo  repository.MatchRepository
	predRepo   repository.PredictionRepository
	log        *zap.Logger
}

// NewScoringService constructs a scoringService with the given dependencies.
func NewScoringService(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	log *zap.Logger,
) MatchScorer {
	return &scoringService{matchRepo: matchRepo, predRepo: predRepo, log: log}
}

// ScoreMatch calculates and persists points for every prediction on the given
// match. It is idempotent: calling it a second time on an already-scored match
// overwrites the existing points with the same values.
func (s *scoringService) ScoreMatch(ctx context.Context, matchID int) error {
	match, err := s.matchRepo.GetByID(ctx, matchID)
	if err != nil {
		return err
	}
	if match == nil {
		return apperrors.NotFound(fmt.Sprintf("match %d not found", matchID))
	}
	if match.Status != domain.MatchStatusFinished {
		return apperrors.Validation("scoring requires a finished match with a confirmed result")
	}
	if match.HomeScore == nil || match.AwayScore == nil {
		return apperrors.Validation("match result is missing home or away score")
	}

	predictions, err := s.predRepo.ListByMatch(ctx, matchID)
	if err != nil {
		return err
	}

	for _, pred := range predictions {
		points := calculatePoints(pred, *match.HomeScore, *match.AwayScore)
		pred.Points = &points
		if err := s.predRepo.Update(ctx, pred); err != nil {
			s.log.Error("failed to update prediction points",
				zap.Int("prediction_id", pred.ID),
				zap.Error(err),
			)
		}
	}
	return nil
}

// calculatePoints applies the scoring rules defined in domain/constants.go.
//
// Decision table (evaluated top-to-bottom, first match wins):
//
//	Exact scoreline                          → PointsExactScore (5)
//	Correct outcome (non-draw) + same margin → PointsCorrectOutcome + PointsGoalDifference (3)
//	Correct outcome only                     → PointsCorrectOutcome (2)
//	Wrong outcome / no prediction            → PointsIncorrectResult (0)
//
// The goal-difference bonus is intentionally excluded for draws. Every draw
// has a margin of 0, so the check is trivially true for any correct-draw
// prediction; awarding the bonus would give draws a structural advantage over
// wins/losses where a matching margin requires real predictive accuracy.
func calculatePoints(pred *domain.Prediction, actualHome, actualAway int) int {
	// Tier 1 — exact scoreline.
	if pred.HomeScore == actualHome && pred.AwayScore == actualAway {
		return domain.PointsExactScore
	}

	predOutcome := outcome(pred.HomeScore, pred.AwayScore)
	actualOutcome := outcome(actualHome, actualAway)

	if predOutcome != actualOutcome {
		return domain.PointsIncorrectResult
	}

	// Tier 2 — correct outcome. Add the goal-difference bonus for non-draw
	// results where the predicted margin matches the actual margin.
	points := domain.PointsCorrectOutcome
	if actualOutcome != outcomeDraw && goalDiff(pred.HomeScore, pred.AwayScore) == goalDiff(actualHome, actualAway) {
		points += domain.PointsGoalDifference
	}
	return points
}

type matchOutcome int

const (
	outcomeHomeWin matchOutcome = iota
	outcomeDraw
	outcomeAwayWin
)

func outcome(home, away int) matchOutcome {
	switch {
	case home > away:
		return outcomeHomeWin
	case home < away:
		return outcomeAwayWin
	default:
		return outcomeDraw
	}
}

// goalDiff returns the absolute goal margin between home and away scores.
func goalDiff(home, away int) int {
	d := home - away
	if d < 0 {
		return -d
	}
	return d
}
