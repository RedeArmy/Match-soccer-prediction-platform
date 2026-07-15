package service

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// ExtraScorer calculates and persists points for every extra prediction
// (bonus predictions beyond the scoreline, e.g. "first team to score") on a
// finished match. It is the extras counterpart to MatchScorer, invoked from
// the same MatchFinished event handlers, but deliberately kept as a separate
// interface: extras are a bonus feature, and a failure here must never block
// or roll back the core prediction scoring that MatchScorer performs.
type ExtraScorer interface {
	ScoreExtras(ctx context.Context, matchID int) error
}

// extraScoringService is the concrete implementation of ExtraScorer.
type extraScoringService struct {
	matchRepo repository.MatchRepository
	extraRepo repository.ExtraPredictionRepository
	ruleRepo  repository.ExtraRuleRepository
	log       *zap.Logger
}

// NewExtraScoringService constructs an extraScoringService.
func NewExtraScoringService(
	matchRepo repository.MatchRepository,
	extraRepo repository.ExtraPredictionRepository,
	ruleRepo repository.ExtraRuleRepository,
	log *zap.Logger,
) ExtraScorer {
	return &extraScoringService{matchRepo: matchRepo, extraRepo: extraRepo, ruleRepo: ruleRepo, log: log}
}

// pointsForType resolves the effective point value for extraType: an active
// extra_rules row, falling back to the compile-time domain default when the
// row is absent or inactive. This is the same IsActive soft-disable pattern
// scoringService.configForPhase uses, minus the intermediate system_params
// layer — see migration 000225's header comment for why that layer is
// skipped for these two fixed, global knobs.
func (s *extraScoringService) pointsForType(ctx context.Context, extraType domain.ExtraType) int {
	rule, err := s.ruleRepo.GetByType(ctx, extraType)
	if err != nil {
		s.log.Warn("extra_rules lookup failed — falling back to default points",
			append([]zap.Field{
				zap.String("extra_type", string(extraType)),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...,
		)
		return defaultExtraPoints(extraType)
	}
	if rule == nil || !rule.IsActive {
		return defaultExtraPoints(extraType)
	}
	return rule.Points
}

func defaultExtraPoints(extraType domain.ExtraType) int {
	switch extraType {
	case domain.ExtraTypeFirstScorer:
		return domain.DefaultExtraFirstScorerPoints
	case domain.ExtraTypeHalftimeResult:
		return domain.DefaultExtraHalftimeResultPoints
	case domain.ExtraTypeHomeTeamScores, domain.ExtraTypeAwayTeamScores:
		return domain.DefaultExtraTeamScoresPoints
	default:
		return 0
	}
}

// teamScoresAnswer derives "first_half"/"second_half"/"both_halves"/"none"
// for one team from its half-time and full-time goal tallies. Both signals
// are derivable from data the sync worker already captures — no separate
// goal-event timeline is needed:
//   - the team scored in the first half iff its half-time tally is > 0
//   - the team scored in the second half iff its full-time tally exceeds its
//     half-time tally
//
// Returns nil (unresolved — leave the prediction unscored) when halftimeGoals
// is nil, or when fulltimeGoals is somehow less than halftimeGoals, a data
// inconsistency that should never occur for a correctly-synced match and is
// treated as unresolved rather than guessed at.
func teamScoresAnswer(halftimeGoals, fulltimeGoals *int) *string {
	if halftimeGoals == nil || fulltimeGoals == nil || *fulltimeGoals < *halftimeGoals {
		return nil
	}
	scoredFirstHalf := *halftimeGoals > 0
	scoredSecondHalf := *fulltimeGoals > *halftimeGoals
	var result string
	switch {
	case scoredFirstHalf && scoredSecondHalf:
		result = "both_halves"
	case scoredFirstHalf:
		result = "first_half"
	case scoredSecondHalf:
		result = "second_half"
	default:
		result = "none"
	}
	return &result
}

// correctExtraAnswers derives the resolved "correct answer" for each extra
// type from the match's result fields. A nil value for a type means it could
// not be resolved for this match (the sync worker never supplied the data,
// or the match finished before this feature existed) — predictions for that
// type are left unscored rather than guessed at.
func correctExtraAnswers(m *domain.Match) map[domain.ExtraType]*string {
	answers := map[domain.ExtraType]*string{
		domain.ExtraTypeFirstScorer:    m.FirstScoringTeam,
		domain.ExtraTypeHomeTeamScores: teamScoresAnswer(m.HalftimeHomeScore, m.HomeScore),
		domain.ExtraTypeAwayTeamScores: teamScoresAnswer(m.HalftimeAwayScore, m.AwayScore),
	}
	if m.HalftimeHomeScore != nil && m.HalftimeAwayScore != nil {
		result := domain.FormatScorelineAnswer(*m.HalftimeHomeScore, *m.HalftimeAwayScore)
		answers[domain.ExtraTypeHalftimeResult] = &result
	}
	return answers
}

// ScoreExtras calculates and persists points for every extra prediction on
// matchID. It is idempotent: the underlying UPDATE only touches rows where
// scored_at IS NULL, so re-running it (a DLQ replay, or after an admin
// resolves a previously-missing extras field via CorrectResult) is always
// safe and only scores what was not already scored.
func (s *extraScoringService) ScoreExtras(ctx context.Context, matchID int) error {
	ctx, span := otel.Tracer("scoring").Start(ctx, "scoring.score_extras")
	span.SetAttributes(attribute.Int("match_id", matchID))
	defer span.End()

	match, err := loadFinishedMatch(ctx, s.matchRepo, span, matchID, "extras scoring requires a finished match")
	if err != nil {
		return err
	}

	answers := correctExtraAnswers(match)
	points := make(map[domain.ExtraType]int, len(domain.AllExtraTypes))
	for _, et := range domain.AllExtraTypes {
		points[et] = s.pointsForType(ctx, et)
	}

	var scoredCount int
	err = s.extraRepo.ScoreMatchBatch(ctx, matchID, func(preds []*domain.ExtraPrediction) (map[int]int, error) {
		result := make(map[int]int)
		for _, pred := range preds {
			correct := answers[pred.ExtraType]
			if correct == nil {
				continue // not yet resolved for this match — leave unscored
			}
			if pred.Answer == *correct {
				result[pred.ID] = points[pred.ExtraType]
			} else {
				result[pred.ID] = 0
			}
		}
		scoredCount = len(result)
		return result, nil
	}, domain.DefaultScoringUpdateChunkSize)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "score extras batch failed")
		return err
	}
	span.SetAttributes(attribute.Int("extra_predictions_scored", scoredCount))
	return nil
}

var _ ExtraScorer = (*extraScoringService)(nil)
