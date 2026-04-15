package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// predictionService is the concrete implementation of PredictionService.
type predictionService struct {
	predRepo  repository.PredictionRepository
	matchRepo repository.MatchRepository
	publisher events.Publisher
	log       *zap.Logger
}

// NewPredictionService constructs a predictionService with the given dependencies.
func NewPredictionService(
	predRepo repository.PredictionRepository,
	matchRepo repository.MatchRepository,
	publisher events.Publisher,
	log *zap.Logger,
) PredictionService {
	return &predictionService{
		predRepo:  predRepo,
		matchRepo: matchRepo,
		publisher: publisher,
		log:       log,
	}
}

// Submit validates and persists a new prediction.
//
// It enforces uniqueness (one prediction per user per match) and deadline rules
// before creating the record. A PredictionMade event is emitted on success.
func (s *predictionService) Submit(ctx context.Context, prediction *domain.Prediction) error {
	match, err := s.matchRepo.GetByID(ctx, prediction.MatchID)
	if err != nil {
		return err
	}
	if match == nil {
		return apperrors.NotFound(fmt.Sprintf("match %d not found", prediction.MatchID))
	}
	if err := domain.ValidatePrediction(prediction, match.KickoffAt, time.Now().UTC()); err != nil {
		return err
	}
	existing, err := s.predRepo.GetByUserAndMatch(ctx, prediction.UserID, prediction.MatchID)
	if err != nil {
		return err
	}
	if existing != nil {
		return apperrors.Conflict("a prediction for this match has already been submitted")
	}
	if err := s.predRepo.Create(ctx, prediction); err != nil {
		return err
	}
	if err := s.publisher.Publish(ctx, events.Envelope{
		Type:       events.EventPredictionMade,
		OccurredAt: time.Now().UTC(),
		Payload: events.PredictionMade{
			PredictionID: prediction.ID,
			UserID:       prediction.UserID,
			MatchID:      prediction.MatchID,
			HomeScore:    prediction.HomeScore,
			AwayScore:    prediction.AwayScore,
		},
	}); err != nil {
		s.log.Error("failed to publish PredictionMade event", zap.Int("prediction_id", prediction.ID), zap.Error(err))
	}
	return nil
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
	updated := &domain.Prediction{
		ID:        pred.ID,
		UserID:    pred.UserID,
		MatchID:   pred.MatchID,
		HomeScore: homeScore,
		AwayScore: awayScore,
	}
	if err := domain.ValidatePrediction(updated, match.KickoffAt, time.Now().UTC()); err != nil {
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

func (s *predictionService) GetByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error) {
	return s.predRepo.ListByMatch(ctx, matchID)
}
