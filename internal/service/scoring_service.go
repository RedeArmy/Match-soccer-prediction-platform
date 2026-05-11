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
	ruleRepo  repository.ScoringRuleRepository
	params    SystemParamService
	log       *zap.Logger
}

// NewScoringService constructs a scoringService with the given dependencies.
func NewScoringService(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	ruleRepo repository.ScoringRuleRepository,
	params SystemParamService,
	log *zap.Logger,
) MatchScorer {
	return &scoringService{
		matchRepo: matchRepo,
		predRepo:  predRepo,
		ruleRepo:  ruleRepo,
		params:    params,
		log:       log,
	}
}

// scoringConfig holds the effective point values for one ScoreMatch call.
// It is populated from the scoring_rules table (phase-specific) with a
// transparent fallback to system_params (global defaults) when the phase row
// is absent or marked inactive.
type scoringConfig struct {
	exactScore     int
	correctOutcome int
	goalDifference int
	extraTimeBonus int // bonus points when the correct win method is extra_time
	penaltiesBonus int // bonus points when the correct win method is penalties
}

// loadGlobalConfig reads the flat, phase-agnostic scoring parameters from
// system_params. Used as a fallback when no active phase-specific rule exists.
// Win-method bonuses default to 0 in the global fallback; the per-phase rule
// is the canonical source for those values.
func (s *scoringService) loadGlobalConfig(ctx context.Context) scoringConfig {
	return scoringConfig{
		exactScore:     s.params.GetInt(ctx, domain.ParamKeyScoringExactScore, domain.PointsExactScore),
		correctOutcome: s.params.GetInt(ctx, domain.ParamKeyScoringCorrectOutcome, domain.PointsCorrectOutcome),
		goalDifference: s.params.GetInt(ctx, domain.ParamKeyScoringGoalDiff, domain.PointsGoalDifference),
		extraTimeBonus: s.params.GetInt(ctx, domain.ParamKeyScoringExtraTimeBonus, domain.DefaultScoringExtraTimeBonus),
		penaltiesBonus: s.params.GetInt(ctx, domain.ParamKeyScoringPenaltiesBonus, domain.DefaultScoringPenaltiesBonus),
	}
}

// configForPhase returns the effective scoring configuration for the given
// tournament phase. Resolution order:
//  1. Active scoring_rules row for the phase → phase-specific values.
//  2. system_params (global flat config) → operator-tuned defaults.
//  3. Domain constants → compile-time safe defaults.
//
// This three-level fallback ensures the scorer never fails due to a missing or
// inactive rule: the system degrades gracefully rather than refusing to score.
func (s *scoringService) configForPhase(ctx context.Context, phase domain.MatchPhase) scoringConfig {
	rule, err := s.ruleRepo.GetByPhase(ctx, phase)
	if err != nil {
		s.log.Warn("scoring_rules lookup failed — falling back to global config",
			zap.String("phase", string(phase)),
			zap.Error(err),
		)
		return s.loadGlobalConfig(ctx)
	}
	if rule == nil || !rule.IsActive {
		return s.loadGlobalConfig(ctx)
	}
	return scoringConfig{
		exactScore:     rule.ExactScore,
		correctOutcome: rule.CorrectOutcome,
		goalDifference: rule.GoalDifference,
		extraTimeBonus: rule.ExtraTimeBonus,
		penaltiesBonus: rule.PenaltiesBonus,
	}
}

// ScoreMatch calculates and persists points for every prediction on the given
// match. It is idempotent: calling it a second time on an already-scored match
// overwrites the existing points with the same values.
//
// Point values are resolved per tournament phase via the scoring_rules table,
// with transparent fallback to global system_params when no active rule is
// found for the phase. This means group-stage fixtures continue to use the
// historic 5/2/1 split, while knockout-phase fixtures automatically use the
// higher values configured in the scoring_rules table.
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

	cfg := s.configForPhase(ctx, match.Phase)
	points := make(map[int]int, len(predictions))
	for _, pred := range predictions {
		points[pred.ID] = calculatePoints(pred, *match.HomeScore, *match.AwayScore, match.WinMethod, cfg)
	}
	return s.predRepo.UpdateManyPoints(ctx, points)
}

// calculatePoints applies the scoring rules from scoringConfig.
//
// Decision table (evaluated top-to-bottom, first match wins):
//
//	Exact scoreline                          -> cfg.exactScore
//	Correct outcome (non-draw) + same margin -> cfg.correctOutcome + cfg.goalDifference
//	Correct outcome only                     -> cfg.correctOutcome
//	Wrong outcome / no prediction            -> 0
//
// On top of the base points, a win-method bonus is added when the user
// predicted the correct win method (extra_time or penalties) and obtained
// a correct outcome (base points > 0). The bonus is exclusive: at most one
// of extraTimeBonus or penaltiesBonus is awarded per prediction.
func calculatePoints(pred *domain.Prediction, actualHome, actualAway int, actualWinMethod *domain.WinMethod, cfg scoringConfig) int {
	base := basePoints(pred, actualHome, actualAway, cfg)
	if base > 0 {
		base += winMethodBonus(pred.PredictedWinMethod, actualWinMethod, cfg)
	}
	return base
}

// basePoints returns the core score for a prediction ignoring win-method bonuses.
func basePoints(pred *domain.Prediction, actualHome, actualAway int, cfg scoringConfig) int {
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

// winMethodBonus returns the bonus when the predicted win method matches the actual one.
// Returns 0 when either side did not declare a win method.
func winMethodBonus(predicted, actual *domain.WinMethod, cfg scoringConfig) int {
	if predicted == nil || actual == nil {
		return 0
	}
	switch *actual {
	case domain.WinMethodExtraTime:
		if *predicted == domain.WinMethodExtraTime {
			return cfg.extraTimeBonus
		}
	case domain.WinMethodPenalties:
		if *predicted == domain.WinMethodPenalties {
			return cfg.penaltiesBonus
		}
	}
	return 0
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
