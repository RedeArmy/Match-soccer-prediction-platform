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

// predictionService is the concrete implementation of PredictionService.
type predictionService struct {
	predRepo  repository.PredictionRepository
	matchRepo repository.MatchRepository
	params    SystemParamService
	clock     clock.Nower
	log       *zap.Logger
}

// NewPredictionService constructs a predictionService with the given dependencies.
func NewPredictionService(
	predRepo repository.PredictionRepository,
	matchRepo repository.MatchRepository,
	params SystemParamService,
	clk clock.Nower,
	log *zap.Logger,
) PredictionService {
	return &predictionService{
		predRepo:  predRepo,
		matchRepo: matchRepo,
		params:    params,
		clock:     clk,
		log:       log,
	}
}

// deadlineOffset reads the prediction deadline from system params (in minutes),
// falling back to the domain constant when the key is absent or unparseable.
func (s *predictionService) deadlineOffset(ctx context.Context) time.Duration {
	mins := s.params.GetInt(ctx, domain.ParamKeyPredictionDeadlineMin, int(domain.PredictionDeadlineOffset/time.Minute))
	return time.Duration(mins) * time.Minute
}

// Submit validates and persists a new prediction.
//
// Two independent guards prevent predictions on a closed match:
//   - Time-based: predictions are rejected after KickoffAt minus the
//     configurable deadline offset (default 5 minutes from system params).
//   - Status-based: a match in Live or Finished status is rejected regardless
//     of the scheduled kickoff time, closing the race window that exists when
//     an admin calls StartMatch before the time-based deadline expires.
//
// Uniqueness (one prediction per user per match) is enforced atomically by the
// database unique index on (user_id, match_id). A Conflict error is surfaced
// to the caller when the constraint fires, avoiding the check-then-act race
// that a pre-INSERT SELECT would introduce.
func (s *predictionService) Submit(ctx context.Context, prediction *domain.Prediction) error {
	match, err := s.matchRepo.GetByID(ctx, prediction.MatchID)
	if err != nil {
		return err
	}
	if match == nil {
		return apperrors.NotFound(fmt.Sprintf("match %d not found", prediction.MatchID))
	}
	if match.Status != domain.MatchStatusScheduled {
		return apperrors.Validation("cannot predict on a match that has already started")
	}
	if err := domain.ValidatePrediction(prediction, match.KickoffAt, s.clock.Now(), s.deadlineOffset(ctx)); err != nil {
		return err
	}
	return s.predRepo.Create(ctx, prediction)
}

// Update modifies the scores on an existing prediction subject to the same
// deadline rules as Submit. The caller must own the prediction.
func (s *predictionService) Update(ctx context.Context, callerUserID, id int, homeScore, awayScore int) (*domain.Prediction, error) {
	pred, err := s.predRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if pred == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("prediction %d not found", id))
	}
	if pred.UserID != callerUserID {
		return nil, apperrors.Forbidden("cannot modify another user's prediction")
	}
	match, err := s.matchRepo.GetByID(ctx, pred.MatchID)
	if err != nil {
		return nil, err
	}
	if match == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("match %d not found", pred.MatchID))
	}
	if match.Status != domain.MatchStatusScheduled {
		return nil, apperrors.Validation("cannot modify a prediction for a match that has already started")
	}
	updated := &domain.Prediction{
		ID:        pred.ID,
		UserID:    pred.UserID,
		MatchID:   pred.MatchID,
		HomeScore: homeScore,
		AwayScore: awayScore,
	}
	if err := domain.ValidatePrediction(updated, match.KickoffAt, s.clock.Now(), s.deadlineOffset(ctx)); err != nil {
		return nil, err
	}
	pred.HomeScore = homeScore
	pred.AwayScore = awayScore
	if err := s.predRepo.Update(ctx, pred); err != nil {
		return nil, err
	}
	return pred, nil
}

func (s *predictionService) GetByUser(ctx context.Context, userID int) ([]*domain.Prediction, error) {
	return s.predRepo.ListByUser(ctx, userID)
}

func (s *predictionService) GetByUserAndQuiniela(ctx context.Context, userID, quinielaID int) ([]*domain.Prediction, error) {
	return s.predRepo.ListByUserAndQuiniela(ctx, userID, quinielaID)
}

func (s *predictionService) GetByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error) {
	return s.predRepo.ListByMatch(ctx, matchID)
}

var _ PredictionService = (*predictionService)(nil)
