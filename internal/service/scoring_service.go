package service

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// MatchScorer calculates and persists points for all predictions on a
// finished match.
//
// ScoreMatch is intended to be called from a MatchFinished event handler, not
// directly from an HTTP handler, which is why it does not return a full list
// of updated predictions - the caller's context is asynchronous.
type MatchScorer interface {
	ScoreMatch(ctx context.Context, matchID int) error
}

// scoringOption is a functional option for NewScoringService.
type scoringOption func(*scoringService)

// WithScoringMeter registers OTel metric instruments on meter.
// Two counters are emitted after each successful ScoreMatch call:
//
//   - scoring.predictions_scored — number of predictions updated
//   - scoring.batch_chunks       — number of 500-row UPDATE chunks executed
//
// Passing a nil meter is a no-op; no metrics are recorded in that case.
func WithScoringMeter(meter metric.Meter) scoringOption {
	return func(s *scoringService) {
		if meter == nil {
			return
		}
		m, err := newScoringMetrics(meter)
		if err != nil {
			s.log.Warn("scoring: metric registration failed (metrics disabled)",
				zap.Error(err),
			)
			return
		}
		s.metrics = m
	}
}

// scoringService is the concrete implementation of MatchScorer.
type scoringService struct {
	matchRepo repository.MatchRepository
	predRepo  repository.PredictionRepository
	ruleRepo  repository.ScoringRuleRepository
	params    SystemParamService
	log       *zap.Logger
	metrics   *scoringMetrics // nil when not wired
}

// NewScoringService constructs a scoringService with the given dependencies.
// Optional functional options (e.g. WithScoringMeter) are applied after the
// base struct is initialised so they have access to the logger.
func NewScoringService(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	ruleRepo repository.ScoringRuleRepository,
	params SystemParamService,
	log *zap.Logger,
	opts ...scoringOption,
) MatchScorer {
	svc := &scoringService{
		matchRepo: matchRepo,
		predRepo:  predRepo,
		ruleRepo:  ruleRepo,
		params:    params,
		log:       log,
	}
	for _, o := range opts {
		o(svc)
	}
	return svc
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

// resolveConfig returns the scoring config to use for matchID. On first-time
// scoring (no prediction_score_log entries for the match) it delegates to
// configForPhase so current rules apply. On re-runs (DLQ replays, manual
// rescores) it returns the config recorded in the earliest log entry, ensuring
// that replays produce identical point values regardless of rule changes that
// occurred after the original scoring run.
//
// A lookup error is non-fatal: it falls back to configForPhase and logs a
// warning. This preserves today's behaviour for the transient-error case while
// still locking to the snapshot on the happy replay path.
func (s *scoringService) resolveConfig(ctx context.Context, matchID int, phase domain.MatchPhase) scoringConfig {
	snapshot, err := s.predRepo.GetScoringCfgSnapshot(ctx, matchID)
	if err != nil {
		s.log.Warn("scoring: GetScoringCfgSnapshot failed — using current rules",
			append([]zap.Field{
				zap.Int("match_id", matchID),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...,
		)
		return s.configForPhase(ctx, phase)
	}
	if snapshot != nil {
		return scoringConfig{
			exactScore:     snapshot.ExactScore,
			correctOutcome: snapshot.CorrectOutcome,
			goalDifference: snapshot.GoalDifference,
			extraTimeBonus: snapshot.ExtraTimeBonus,
			penaltiesBonus: snapshot.PenaltiesBonus,
		}
	}
	return s.configForPhase(ctx, phase)
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
			append([]zap.Field{
				zap.String("phase", string(phase)),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...,
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
	ctx, span := otel.Tracer("scoring").Start(ctx, "scoring.score_match")
	span.SetAttributes(attribute.Int("match_id", matchID))
	defer span.End()

	match, err := s.matchRepo.GetByID(ctx, matchID)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "match lookup failed")
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

	cfg := s.resolveConfig(ctx, matchID, match.Phase)
	chunkSize := s.params.GetInt(ctx, domain.ParamKeyScoringUpdateChunkSize, domain.DefaultScoringUpdateChunkSize)

	// ScoreMatchBatch reads predictions and writes points in a single transaction,
	// eliminating the silent-failure window that existed when ListByMatch and
	// UpdateManyPoints were called in separate round-trips. A network error no
	// longer produces an inconsistent state: the transaction is rolled back and
	// the event bus re-delivers the MatchFinished event for a clean retry.
	var (
		capturedPreds []*domain.Prediction
		capturedOld   map[int]*int
		capturedNew   map[int]int
	)
	if err := s.predRepo.ScoreMatchBatch(ctx, matchID, func(preds []*domain.Prediction) (map[int]int, error) {
		if len(preds) == 0 {
			return nil, nil
		}
		capturedPreds = preds
		capturedOld = make(map[int]*int, len(preds))
		capturedNew = make(map[int]int, len(preds))
		for _, pred := range preds {
			capturedOld[pred.ID] = pred.Points
			capturedNew[pred.ID] = calculatePoints(pred, *match.HomeScore, *match.AwayScore, match.WinMethod, match.PenaltyWinner, cfg)
		}
		return capturedNew, nil
	}, chunkSize); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "score match batch failed")
		return err
	}

	if len(capturedPreds) == 0 {
		return nil
	}
	chunkCount := (len(capturedPreds) + chunkSize - 1) / chunkSize
	span.SetAttributes(
		attribute.Int("prediction_count", len(capturedPreds)),
		attribute.String("match_phase", string(match.Phase)),
		attribute.Int("batch_chunks", chunkCount),
	)
	s.metrics.observe(ctx, int64(len(capturedPreds)), int64(chunkCount))
	s.writeScoringLog(ctx, capturedPreds, capturedOld, capturedNew, match, cfg)
	return nil
}

// writeScoringLog inserts one audit record per prediction into prediction_score_log.
// It is called after UpdateManyPoints succeeds; a failure here is logged and
// swallowed so the scoring outcome is never rolled back for a logging error.
func (s *scoringService) writeScoringLog(
	ctx context.Context,
	predictions []*domain.Prediction,
	oldPts map[int]*int,
	newPoints map[int]int,
	match *domain.Match,
	cfg scoringConfig,
) {
	entries := make([]domain.PredictionScoreLog, 0, len(predictions))
	for _, pred := range predictions {
		entries = append(entries, domain.PredictionScoreLog{
			PredictionID:      pred.ID,
			MatchID:           pred.MatchID,
			UserID:            pred.UserID,
			OldPoints:         oldPts[pred.ID],
			NewPoints:         newPoints[pred.ID],
			MatchHomeScore:    *match.HomeScore,
			MatchAwayScore:    *match.AwayScore,
			MatchWinMethod:    match.WinMethod,
			MatchPhase:        match.Phase,
			PredHomeScore:     pred.HomeScore,
			PredAwayScore:     pred.AwayScore,
			PredWinMethod:     pred.PredictedWinMethod,
			CfgExactScore:     cfg.exactScore,
			CfgCorrectOutcome: cfg.correctOutcome,
			CfgGoalDiff:       cfg.goalDifference,
			CfgExtraTimeBonus: cfg.extraTimeBonus,
			CfgPenaltiesBonus: cfg.penaltiesBonus,
		})
	}
	if err := s.predRepo.InsertScoringBatch(ctx, entries); err != nil {
		s.log.Warn("scoring log: batch insert failed (non-fatal)",
			append([]zap.Field{
				zap.Int("match_id", match.ID),
				zap.Int("entry_count", len(entries)),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...,
		)
	}
}

// calculatePoints applies the scoring rules from scoringConfig.
//
// Decision table (evaluated top-to-bottom, first match wins):
//
//	Exact scoreline                          -> cfg.exactScore
//	Correct outcome (non-draw) + same margin -> cfg.correctOutcome + cfg.goalDifference
//	Correct outcome only                     -> cfg.correctOutcome
//	Wrong outcome + same goal margin         -> cfg.goalDifference   (no win-method bonus)
//	Wrong outcome / no prediction            -> 0
//
// A win-method bonus is added on top of the base points when the prediction
// earned a correct-outcome tier (rules 1–3) and the user correctly predicted
// extra_time or penalties. The bonus is NOT awarded for the margin-only tier
// (rule 4): the user did not identify the correct winner. The bonus is
// exclusive: at most one of extraTimeBonus or penaltiesBonus per prediction.
func calculatePoints(pred *domain.Prediction, actualHome, actualAway int, actualWinMethod *domain.WinMethod, actualPenaltyWinner *string, cfg scoringConfig) int {
	base, bonusEligible := basePoints(pred, actualHome, actualAway, cfg)
	if bonusEligible && base > 0 {
		base += winMethodBonus(pred.PredictedWinMethod, pred.PredictedPenaltyWinner, actualWinMethod, actualPenaltyWinner, cfg)
	}
	return base
}

// basePoints returns the core score and whether the win-method bonus may apply.
//
// bonusEligible is true only for correct-outcome tiers (exact, outcome+margin,
// outcome-only). It is false for the margin-only tier and for wrong outcomes,
// because the win-method bonus is tied to correctly identifying the winner.
func basePoints(pred *domain.Prediction, actualHome, actualAway int, cfg scoringConfig) (points int, bonusEligible bool) {
	if pred.HomeScore == actualHome && pred.AwayScore == actualAway {
		return cfg.exactScore, true
	}
	predOutcome := outcome(pred.HomeScore, pred.AwayScore)
	actualOutcome := outcome(actualHome, actualAway)
	if predOutcome != actualOutcome {
		// Margin-only tier: the goal difference matches despite the wrong winner.
		// Award goalDifference points without win-method bonus eligibility.
		if goalDiff(pred.HomeScore, pred.AwayScore) == goalDiff(actualHome, actualAway) {
			return cfg.goalDifference, false
		}
		return domain.PointsIncorrectResult, false
	}
	pts := cfg.correctOutcome
	if actualOutcome != outcomeDraw && goalDiff(pred.HomeScore, pred.AwayScore) == goalDiff(actualHome, actualAway) {
		pts += cfg.goalDifference
	}
	return pts, true
}

// winMethodBonus returns the bonus when the predicted win method matches the actual one.
// Returns 0 when either side did not declare a win method.
// For penalties, if the match records which team won (actualPenaltyWinner non-nil),
// the user's predicted penalty winner must also match to earn the bonus.
// When actualPenaltyWinner is nil (older matches or sync-driven finalisations),
// only the win-method match is required, preserving backward compatibility.
func winMethodBonus(predicted *domain.WinMethod, predPenaltyWinner *string, actual *domain.WinMethod, actualPenaltyWinner *string, cfg scoringConfig) int {
	if predicted == nil || actual == nil {
		return 0
	}
	switch *actual {
	case domain.WinMethodNormal:
		return 0
	case domain.WinMethodExtraTime:
		if *predicted == domain.WinMethodExtraTime {
			return cfg.extraTimeBonus
		}
	case domain.WinMethodPenalties:
		if *predicted != domain.WinMethodPenalties {
			return 0
		}
		if actualPenaltyWinner != nil {
			if predPenaltyWinner == nil || *predPenaltyWinner != *actualPenaltyWinner {
				return 0
			}
		}
		return cfg.penaltiesBonus
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
