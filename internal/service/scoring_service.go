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
	matchRepo repository.MatchRepository
	predRepo  repository.PredictionRepository
	params    SystemParamService
	log       *zap.Logger
}

// NewScoringService constructs a scoringService with the given dependencies.
func NewScoringService(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	params SystemParamService,
	log *zap.Logger,
) MatchScorer {
	return &scoringService{matchRepo: matchRepo, predRepo: predRepo, params: params, log: log}
}

// scoringConfig holds the point values read from SystemParamService (or their
// domain constant defaults). Reading once per ScoreMatch call avoids N cache
// lookups inside the per-prediction loop.
type scoringConfig struct {
	exactScore     int
	correctOutcome int
	goalDifference int
}

func (s *scoringService) loadScoringConfig(ctx context.Context) scoringConfig {
	return scoringConfig{
		exactScore:     s.params.GetInt(ctx, domain.ParamKeyScoringExactScore, domain.PointsExactScore),
		correctOutcome: s.params.GetInt(ctx, domain.ParamKeyScoringCorrectOutcome, domain.PointsCorrectOutcome),
		goalDifference: s.params.GetInt(ctx, domain.ParamKeyScoringGoalDiff, domain.PointsGoalDifference),
	}
}

// ScoreMatch calculates and persists points for every prediction on the given
// match. It is idempotent: calling it a second time on an already-scored match
// overwrites the existing points with the same values.
//
// All point updates are committed as a single atomic transaction via
// UpdateManyPoints. If the process crashes mid-flight the entire batch is
// rolled back, preventing the partial-scoring state where some predictions on
// the same match show a score and others show nil.
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
	if len(predictions) == 0 {
		return nil
	}

	cfg := s.loadScoringConfig(ctx)
	points := make(map[int]int, len(predictions))
	for _, pred := range predictions {
		points[pred.ID] = calculatePoints(pred, *match.HomeScore, *match.AwayScore, cfg)
	}
	return s.predRepo.UpdateManyPoints(ctx, points)
}

// calculatePoints applies the scoring rules from scoringConfig.
//
// Decision table (evaluated top-to-bottom, first match wins):
//
//	Exact scoreline                          -> cfg.exactScore (default 5)
//	Correct outcome (non-draw) + same margin -> cfg.correctOutcome + cfg.goalDifference (default 3)
//	Correct outcome only                     -> cfg.correctOutcome (default 2)
//	Wrong outcome / no prediction            -> 0
func calculatePoints(pred *domain.Prediction, actualHome, actualAway int, cfg scoringConfig) int {
	if pred.HomeScore == actualHome && pred.AwayScore == actualAway {
		return cfg.exactScore
	}

	predOutcome := outcome(pred.HomeScore, pred.AwayScore)
	actualOutcome := outcome(actualHome, actualAway)

	if predOutcome != actualOutcome {
		return domain.PointsIncorrectResult
	}

	points := cfg.correctOutcome
	if actualOutcome != outcomeDraw && goalDiff(pred.HomeScore, pred.AwayScore) == goalDiff(actualHome, actualAway) {
		points += cfg.goalDifference
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

var _ MatchScorer = (*scoringService)(nil)
