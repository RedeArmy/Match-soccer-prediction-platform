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
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// MatchService defines operations on the Match entity.
//
// UpdateResult enforces the transition rules: a result may only be set when
// the match is in the Live or Finished status. After confirming the result
// the implementation must emit a MatchFinished domain event so that downstream
// consumers (MatchScorer, Notifier) react without being called
// directly.
type MatchService interface {
	CreateMatch(ctx context.Context, match *domain.Match) error
	GetMatch(ctx context.Context, id int) (*domain.Match, error)
	ListMatches(ctx context.Context) ([]*domain.Match, error)
	ListMatchesByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error)
	ListMatchesByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error)
	UpdateResult(ctx context.Context, id int, homeScore, awayScore int, winMethod *domain.WinMethod) (*domain.Match, error)
	StartMatch(ctx context.Context, id int) (*domain.Match, error)
	// CorrectResult overwrites the score on a finished match and re-emits
	// MatchFinished so the scoring engine re-scores all predictions. Use this
	// when the API-Football feed provided a wrong score that was already confirmed.
	CorrectResult(ctx context.Context, id int, homeScore, awayScore int, winMethod *domain.WinMethod) (*domain.Match, error)
	// CancelMatch marks a scheduled or live match as cancelled. Cancelled matches
	// are excluded from prediction scoring and cannot transition to any other status.
	CancelMatch(ctx context.Context, id int) (*domain.Match, error)
}

// matchService is the concrete implementation of MatchService.
type matchService struct {
	repo  repository.MatchRepository
	pub   events.Publisher
	audit AuditLogger
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
func NewMatchService(
	repo repository.MatchRepository,
	pub events.Publisher,
	scorer MatchScorer,
	audit AuditLogger,
	log *zap.Logger,
) MatchService {
	return &matchService{repo: repo, pub: pub, scorer: scorer, audit: audit, log: log}
}

func (s *matchService) CreateMatch(ctx context.Context, match *domain.Match) error {
	if err := domain.ValidateMatch(match); err != nil {
		return err
	}
	match.Status = domain.MatchStatusScheduled
	if err := s.repo.Create(ctx, match); err != nil {
		return err
	}

	resType := "match"
	s.audit.Log(ctx, nil, nil, domain.AuditActionMatchCreated, &resType, &match.ID, map[string]any{
		"home_team": match.HomeTeam,
		"away_team": match.AwayTeam,
	})
	return nil
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

	resType := "match"
	s.audit.Log(ctx, nil, nil, domain.AuditActionMatchStarted, &resType, &id, nil)

	if err := s.pub.Publish(ctx, events.Envelope{
		Type:       events.EventMatchStarted,
		OccurredAt: time.Now().UTC(),
		Payload: events.MatchStarted{
			MatchID:   m.ID,
			HomeTeam:  m.HomeTeam,
			AwayTeam:  m.AwayTeam,
			KickoffAt: m.KickoffAt,
		},
	}); err != nil {
		s.log.Error("failed to publish MatchStarted event",
			append([]zap.Field{zap.Int("match_id", id), zap.Error(err)}, tracing.LogFields(ctx)...)...)
	}
	return m, nil
}

// UpdateResult sets the final score on a match and emits MatchFinished.
//
// The match must be in Live status. Confirming a result on a Scheduled match
// is rejected because the match must be explicitly started first via StartMatch,
// ensuring the prediction deadline has already closed. Updating a Finished
// match is rejected to prevent silent overwrites of confirmed results - once
// the score is confirmed it is permanent.
//
// winMethod is optional for group-stage matches (pass nil). For knockout phases
// it should be provided so the scoring engine can apply win-method bonuses.
func (s *matchService) UpdateResult(ctx context.Context, id int, homeScore, awayScore int, winMethod *domain.WinMethod) (*domain.Match, error) {
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
	return s.applyScoreAndPublish(ctx, m, homeScore, awayScore, winMethod,
		domain.AuditActionMatchResultSet,
		"failed to publish MatchFinished event; falling back to synchronous scoring",
		"synchronous fallback scoring failed - predictions for this match may be unscored",
	)
}

// CorrectResult overwrites the score on a match that is already finished (or
// still live if the API confirmed the score early) and re-emits MatchFinished
// so the scoring engine recomputes predictions. It is the admin escape-hatch
// for when API-Football delivered a wrong final score.
func (s *matchService) CorrectResult(ctx context.Context, id int, homeScore, awayScore int, winMethod *domain.WinMethod) (*domain.Match, error) {
	if err := domain.ValidateMatchResult(&homeScore, &awayScore); err != nil {
		return nil, err
	}
	m, err := s.GetMatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status != domain.MatchStatusFinished && m.Status != domain.MatchStatusLive {
		return nil, apperrors.Validation("result can only be corrected on a live or finished match")
	}
	return s.applyScoreAndPublish(ctx, m, homeScore, awayScore, winMethod,
		domain.AuditActionMatchResultCorrected,
		"failed to publish MatchFinished event (correction); falling back to synchronous scoring",
		"synchronous fallback scoring failed after correction",
	)
}

// applyScoreAndPublish writes the final score to the repository, logs the audit
// entry, and publishes MatchFinished. If publishing fails, it falls back to
// synchronous scoring via the scorer. Both UpdateResult and CorrectResult share
// this path; they differ only in their pre-conditions and audit action.
func (s *matchService) applyScoreAndPublish(
	ctx context.Context,
	m *domain.Match,
	homeScore, awayScore int,
	winMethod *domain.WinMethod,
	auditAction string,
	publishErrMsg, fallbackErrMsg string,
) (*domain.Match, error) {
	m.HomeScore = &homeScore
	m.AwayScore = &awayScore
	m.WinMethod = winMethod
	m.Status = domain.MatchStatusFinished
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}

	auditMeta := map[string]any{"home_score": homeScore, "away_score": awayScore}
	if winMethod != nil {
		auditMeta["win_method"] = string(*winMethod)
	}
	resType := "match"
	s.audit.Log(ctx, nil, nil, auditAction, &resType, &m.ID, auditMeta)

	if err := s.pub.Publish(ctx, events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now().UTC(),
		Payload: events.MatchFinished{
			MatchID:   m.ID,
			HomeTeam:  m.HomeTeam,
			AwayTeam:  m.AwayTeam,
			HomeScore: homeScore,
			AwayScore: awayScore,
			WinMethod: winMethodString(winMethod),
		},
	}); err != nil {
		s.log.Error(publishErrMsg,
			append([]zap.Field{zap.Int("match_id", m.ID), zap.Error(err)}, tracing.LogFields(ctx)...)...)
		if scoreErr := s.scorer.ScoreMatch(ctx, m.ID); scoreErr != nil {
			s.log.Error(fallbackErrMsg,
				append([]zap.Field{zap.Int("match_id", m.ID), zap.Error(scoreErr)}, tracing.LogFields(ctx)...)...)
		}
	}
	return m, nil
}

// CancelMatch marks a scheduled or live match as cancelled.
func (s *matchService) CancelMatch(ctx context.Context, id int) (*domain.Match, error) {
	m, err := s.GetMatch(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status == domain.MatchStatusFinished || m.Status == domain.MatchStatusCancelled {
		return nil, apperrors.Validation("only scheduled or live matches can be cancelled")
	}
	m.Status = domain.MatchStatusCancelled
	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	resType := "match"
	s.audit.Log(ctx, nil, nil, domain.AuditActionMatchCancelled, &resType, &id, nil)
	return m, nil
}

// winMethodString converts a nullable WinMethod pointer to its string
// representation for the MatchFinished event. Returns empty string when nil
// (group-stage matches or normal-time results that were not explicitly tagged).
func winMethodString(wm *domain.WinMethod) string {
	if wm == nil {
		return ""
	}
	return string(*wm)
}

var _ MatchService = (*matchService)(nil)
