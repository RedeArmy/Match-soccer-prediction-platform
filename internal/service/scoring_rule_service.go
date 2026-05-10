package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

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
	exactScore, correctOutcome, goalDifference int,
	isActive bool,
	actorID int,
) (*domain.ScoringRule, error) {
	if exactScore < 0 || correctOutcome < 0 || goalDifference < 0 {
		return nil, apperrors.Validation("point values must be non-negative")
	}
	if correctOutcome >= exactScore && exactScore > 0 {
		return nil, apperrors.Validation("exact_score must be greater than correct_outcome to preserve the scoring incentive hierarchy")
	}

	rule := &domain.ScoringRule{
		Phase:          phase,
		ExactScore:     exactScore,
		CorrectOutcome: correctOutcome,
		GoalDifference: goalDifference,
		IsActive:       isActive,
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
			"phase":           string(phase),
			"exact_score":     exactScore,
			"correct_outcome": correctOutcome,
			"goal_difference": goalDifference,
			"is_active":       isActive,
		},
	)
	s.log.Info("scoring rule updated",
		zap.String("phase", string(phase)),
		zap.Int("exact_score", exactScore),
		zap.Int("correct_outcome", correctOutcome),
		zap.Int("goal_difference", goalDifference),
		zap.Bool("is_active", isActive),
		zap.String("actor", fmt.Sprintf("user:%d", actorID)),
	)
	return updated, nil
}

var _ ScoringRuleService = (*scoringRuleService)(nil)
