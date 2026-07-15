package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// ExtraPredictionService defines operations on the ExtraPrediction entity
// (bonus predictions beyond the scoreline, e.g. "first team to score").
//
// Submit shares PredictionService's lock rules: a match must still be
// Scheduled and within the prediction deadline. Unlike PredictionService,
// there is no separate Update method — Submit's Upsert covers both the
// initial guess and any change made before kickoff.
type ExtraPredictionService interface {
	Submit(ctx context.Context, userID, matchID int, extraType domain.ExtraType, answer string) (*domain.ExtraPrediction, error)
	// GetByUserAndMatch returns every extra guess (across all extra types)
	// submitted by userID for matchID.
	GetByUserAndMatch(ctx context.Context, userID, matchID int) ([]*domain.ExtraPrediction, error)
	// ListByUserAndMatches bulk-fetches extras across multiple matches, for
	// rendering a match-list view without N+1 queries.
	ListByUserAndMatches(ctx context.Context, userID int, matchIDs []int) ([]*domain.ExtraPrediction, error)
}

// extraPredictionService is the concrete implementation of ExtraPredictionService.
type extraPredictionService struct {
	extraRepo repository.ExtraPredictionRepository
	matchRepo repository.MatchRepository
	predRepo  repository.PredictionRepository
	params    SystemParamService
	clock     clock.Nower
	log       *zap.Logger
}

// NewExtraPredictionService constructs an extraPredictionService.
func NewExtraPredictionService(
	extraRepo repository.ExtraPredictionRepository,
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	params SystemParamService,
	clk clock.Nower,
	log *zap.Logger,
) ExtraPredictionService {
	return &extraPredictionService{
		extraRepo: extraRepo,
		matchRepo: matchRepo,
		predRepo:  predRepo,
		params:    params,
		clock:     clk,
		log:       log,
	}
}

// deadlineOffset reads the prediction deadline from system params (in
// minutes), the same key PredictionService uses — extras share the same
// lock window as the scoreline prediction.
func (s *extraPredictionService) deadlineOffset(ctx context.Context) time.Duration {
	mins := s.params.GetInt(ctx, domain.ParamKeyPredictionDeadlineMin, domain.DefaultPredictionDeadlineMin)
	return time.Duration(mins) * time.Minute
}

// Submit validates and persists a guess for one match extra. extraType and
// answer are validated against the fixed, known value sets (domain.ParseExtraType
// / domain.ValidateExtraAnswer) before the match lock is checked, so a bad
// request never reaches the repository layer.
//
// The extra answer must also be logically consistent with the user's own
// scoreline prediction for this match (e.g. a team predicted to score 0
// goals cannot be picked as first scorer, and a half-time scoreline can't
// exceed the predicted final score) — see domain.ValidateExtraAnswerAgainstPrediction.
// This requires a Prediction to already exist for (userID, matchID); extras
// are bonus guesses about how the scoreline plays out, so the scoreline
// itself must be submitted first.
func (s *extraPredictionService) Submit(ctx context.Context, userID, matchID int, extraType domain.ExtraType, answer string) (*domain.ExtraPrediction, error) {
	if _, err := domain.ParseExtraType(string(extraType)); err != nil {
		return nil, err
	}
	if err := domain.ValidateExtraAnswer(extraType, answer); err != nil {
		return nil, err
	}

	match, err := s.matchRepo.GetByID(ctx, matchID)
	if err != nil {
		return nil, err
	}
	if match == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("match %d not found", matchID))
	}
	if match.Status != domain.MatchStatusScheduled {
		return nil, apperrors.Validation("cannot submit an extra for a match that has already started")
	}
	if err := domain.ValidatePredictionDeadline(match.KickoffAt, s.clock.Now(), s.deadlineOffset(ctx)); err != nil {
		return nil, err
	}

	existingPrediction, err := s.predRepo.GetByUserAndMatch(ctx, userID, matchID)
	if err != nil {
		return nil, err
	}
	if existingPrediction == nil {
		return nil, apperrors.Validation("submit your scoreline prediction for this match before submitting extras")
	}
	if err := domain.ValidateExtraAnswerAgainstPrediction(extraType, answer, existingPrediction.HomeScore, existingPrediction.AwayScore); err != nil {
		return nil, err
	}

	pred := &domain.ExtraPrediction{UserID: userID, MatchID: matchID, ExtraType: extraType, Answer: answer}
	if _, err := s.extraRepo.Upsert(ctx, pred); err != nil {
		return nil, err
	}
	return pred, nil
}

func (s *extraPredictionService) GetByUserAndMatch(ctx context.Context, userID, matchID int) ([]*domain.ExtraPrediction, error) {
	return s.extraRepo.GetByUserAndMatch(ctx, userID, matchID)
}

func (s *extraPredictionService) ListByUserAndMatches(ctx context.Context, userID int, matchIDs []int) ([]*domain.ExtraPrediction, error) {
	return s.extraRepo.ListByUserAndMatches(ctx, userID, matchIDs)
}

var _ ExtraPredictionService = (*extraPredictionService)(nil)
