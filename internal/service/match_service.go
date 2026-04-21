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

// matchService is the concrete implementation of MatchService.
type matchService struct {
	repo      repository.MatchRepository
	publisher events.Publisher
	// scorer is called synchronously when the MatchFinished event cannot be
	// published (e.g. Redis is unavailable). This guarantees that predictions
	// are always scored even if the event bus is down at the moment of result
	// confirmation. The fallback does not apply to MatchStarted: that event
	// only triggers notifications, which are best-effort by design.
	scorer MatchScorer
	log    *zap.Logger
}

// NewMatchService constructs a matchService with the given dependencies.
// scorer is invoked as a synchronous fallback when publishing MatchFinished
// fails; it must not be nil.
func NewMatchService(repo repository.MatchRepository, publisher events.Publisher, scorer MatchScorer, log *zap.Logger) MatchService {
	return &matchService{repo: repo, publisher: publisher, scorer: scorer, log: log}
}

func (s *matchService) CreateMatch(ctx context.Context, match *domain.Match) error {
	if err := domain.ValidateMatch(match); err != nil {
		return err
	}
	match.Status = domain.MatchStatusScheduled
	return s.repo.Create(ctx, match)
}

func (s *matchService) GetMatch(ctx context.Context, id int) (*domain.Match, error) {
	m, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, apperrors.NotFound(fmt.Sprintf("match %d not found", id))
	}
	return m, nil
}

func (s *matchService) ListMatches(ctx context.Context) ([]*domain.Match, error) {
	return s.repo.List(ctx)
}

func (s *matchService) ListMatchesByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error) {
	return s.repo.ListByPhase(ctx, phase)
}

func (s *matchService) ListMatchesByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error) {
	return s.repo.ListByStatus(ctx, status)
}

// StartMatch transitions a match from Scheduled to Live and emits MatchStarted.
func (s *matchService) StartMatch(ctx context.Context, id int) (*domain.Match, error) {
	m, err := s.GetMatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status != domain.MatchStatusScheduled {
		return nil, apperrors.Validation("match can only be started from scheduled status")
	}
	m.Status = domain.MatchStatusLive
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	if err := s.publisher.Publish(ctx, events.Envelope{
		Type:       events.EventMatchStarted,
		OccurredAt: time.Now().UTC(),
		Payload: events.MatchStarted{
			MatchID:   m.ID,
			HomeTeam:  m.HomeTeam,
			AwayTeam:  m.AwayTeam,
			KickoffAt: m.KickoffAt,
		},
	}); err != nil {
		s.log.Error("failed to publish MatchStarted event", zap.Int("match_id", id), zap.Error(err))
	}
	return m, nil
}

// UpdateResult sets the final score on a match and emits MatchFinished.
//
// The match must be in Live status. Confirming a result on a Scheduled match
// is rejected because the match must be explicitly started first via StartMatch,
// ensuring the prediction deadline has already closed. Updating a Finished
// match is rejected to prevent silent overwrites of confirmed results — once
// the score is confirmed it is permanent.
func (s *matchService) UpdateResult(ctx context.Context, id int, homeScore, awayScore int) (*domain.Match, error) {
	if err := domain.ValidateMatchResult(&homeScore, &awayScore); err != nil {
		return nil, err
	}
	m, err := s.GetMatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status != domain.MatchStatusLive {
		return nil, apperrors.Validation("match result can only be confirmed while the match is live")
	}
	m.HomeScore = &homeScore
	m.AwayScore = &awayScore
	m.Status = domain.MatchStatusFinished
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	if err := s.publisher.Publish(ctx, events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now().UTC(),
		Payload: events.MatchFinished{
			MatchID:   m.ID,
			HomeTeam:  m.HomeTeam,
			AwayTeam:  m.AwayTeam,
			HomeScore: homeScore,
			AwayScore: awayScore,
		},
	}); err != nil {
		s.log.Error("failed to publish MatchFinished event; falling back to synchronous scoring",
			zap.Int("match_id", id), zap.Error(err))
		if scoreErr := s.scorer.ScoreMatch(ctx, m.ID); scoreErr != nil {
			s.log.Error("synchronous fallback scoring failed — predictions for this match may be unscored",
				zap.Int("match_id", id), zap.Error(scoreErr))
		}
	}
	return m, nil
}
