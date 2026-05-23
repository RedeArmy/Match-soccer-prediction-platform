package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ScoringRuleService manages per-phase point configuration exposed through the
// admin API. Operators can raise knockout-stage point values mid-tournament
// without a service restart or migration; changes take effect on the next
// ScoreMatch call.
type ScoringRuleService interface {
	// List returns all phase rules ordered by tournament progression.
	List(ctx context.Context) ([]*domain.ScoringRule, error)
	// GetByPhase returns the rule for a specific phase.
	GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.ScoringRule, error)
	// Update persists new point values for a phase and records an audit entry.
	// Returns NotFound when the phase has no seeded row; returns Validation when
	// any point value is negative or the scoring hierarchy is violated.
	Update(ctx context.Context, phase domain.MatchPhase, input domain.ScoringRuleInput, actorID int) (*domain.ScoringRule, error)
}

type scoringRuleService struct {
	repo  repository.ScoringRuleRepository
	audit AuditLogger
	log   *zap.Logger
}

// NewScoringRuleService constructs a scoringRuleService.
func NewScoringRuleService(
	repo repository.ScoringRuleRepository,
	audit AuditLogger,
	log *zap.Logger,
) ScoringRuleService {
	return &scoringRuleService{repo: repo, audit: audit, log: log}
}

func (s *scoringRuleService) List(ctx context.Context) ([]*domain.ScoringRule, error) {
	return s.repo.List(ctx)
}

func (s *scoringRuleService) GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.ScoringRule, error) {
	rule, err := s.repo.GetByPhase(ctx, phase)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, apperrors.NotFound("scoring rule not found for phase: " + string(phase))
	}
	return rule, nil
}

func (s *scoringRuleService) Update(
	ctx context.Context,
	phase domain.MatchPhase,
	input domain.ScoringRuleInput,
	actorID int,
) (*domain.ScoringRule, error) {
	if input.ExactScore < 0 || input.CorrectOutcome < 0 || input.GoalDifference < 0 {
		return nil, apperrors.Validation("point values must be non-negative")
	}
	if input.ExtraTimeBonus < 0 || input.PenaltiesBonus < 0 {
		return nil, apperrors.Validation("bonus values must be non-negative")
	}
	if input.CorrectOutcome >= input.ExactScore && input.ExactScore > 0 {
		return nil, apperrors.Validation("exact_score must be greater than correct_outcome to preserve the scoring incentive hierarchy")
	}

	rule := &domain.ScoringRule{
		Phase:          phase,
		ExactScore:     input.ExactScore,
		CorrectOutcome: input.CorrectOutcome,
		GoalDifference: input.GoalDifference,
		ExtraTimeBonus: input.ExtraTimeBonus,
		PenaltiesBonus: input.PenaltiesBonus,
		IsActive:       input.IsActive,
	}
	updated, err := s.repo.Update(ctx, rule)
	if err != nil {
		return nil, err
	}

	resourceType := "scoring_rule"
	resourceID := updated.ID
	s.audit.Log(ctx, &actorID, nil, domain.AuditActionScoringRuleUpdated,
		&resourceType, &resourceID,
		map[string]any{
			"phase":            string(phase),
			"exact_score":      input.ExactScore,
			"correct_outcome":  input.CorrectOutcome,
			"goal_difference":  input.GoalDifference,
			"extra_time_bonus": input.ExtraTimeBonus,
			"penalties_bonus":  input.PenaltiesBonus,
			"is_active":        input.IsActive,
		},
	)
	s.log.Info("scoring rule updated",
		zap.String("phase", string(phase)),
		zap.Int("exact_score", input.ExactScore),
		zap.Int("correct_outcome", input.CorrectOutcome),
		zap.Int("goal_difference", input.GoalDifference),
		zap.Int("extra_time_bonus", input.ExtraTimeBonus),
		zap.Int("penalties_bonus", input.PenaltiesBonus),
		zap.Bool("is_active", input.IsActive),
		zap.String("actor", fmt.Sprintf("user:%d", actorID)),
	)
	return updated, nil
}

var _ ScoringRuleService = (*scoringRuleService)(nil)
