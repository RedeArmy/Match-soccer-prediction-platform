package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ExtraRuleService manages the point configuration for match extras (bonus
// predictions) exposed through the admin API. Operators can adjust points or
// soft-disable an extra type mid-tournament without a service restart or
// migration; changes take effect on the next ScoreExtras call. Mirrors
// ScoringRuleService's shape without the per-phase dimension.
type ExtraRuleService interface {
	// List returns all extra rules ordered by extra_type.
	List(ctx context.Context) ([]*domain.ExtraRule, error)
	// GetByType returns the rule for a specific extra type.
	GetByType(ctx context.Context, extraType domain.ExtraType) (*domain.ExtraRule, error)
	// Update persists a new point value and is_active flag for an extra type
	// and records an audit entry. Returns NotFound when the type has no seeded
	// row; returns Validation when points is negative.
	Update(ctx context.Context, extraType domain.ExtraType, input domain.ExtraRuleInput, actorID int) (*domain.ExtraRule, error)
}

type extraRuleService struct {
	repo  repository.ExtraRuleRepository
	audit AuditLogger
	log   *zap.Logger
}

// NewExtraRuleService constructs an extraRuleService.
func NewExtraRuleService(
	repo repository.ExtraRuleRepository,
	audit AuditLogger,
	log *zap.Logger,
) ExtraRuleService {
	return &extraRuleService{repo: repo, audit: audit, log: log}
}

func (s *extraRuleService) List(ctx context.Context) ([]*domain.ExtraRule, error) {
	return s.repo.List(ctx)
}

func (s *extraRuleService) GetByType(ctx context.Context, extraType domain.ExtraType) (*domain.ExtraRule, error) {
	rule, err := s.repo.GetByType(ctx, extraType)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, apperrors.NotFound("extra rule not found for type: " + string(extraType))
	}
	return rule, nil
}

func (s *extraRuleService) Update(
	ctx context.Context,
	extraType domain.ExtraType,
	input domain.ExtraRuleInput,
	actorID int,
) (*domain.ExtraRule, error) {
	if _, err := domain.ParseExtraType(string(extraType)); err != nil {
		return nil, err
	}
	if input.Points < 0 {
		return nil, apperrors.Validation("points must be non-negative")
	}

	rule := &domain.ExtraRule{
		ExtraType: extraType,
		Points:    input.Points,
		IsActive:  input.IsActive,
	}
	updated, err := s.repo.Update(ctx, rule)
	if err != nil {
		return nil, err
	}

	resourceType := "extra_rule"
	resourceID := updated.ID
	s.audit.Log(ctx, &actorID, nil, domain.AuditActionExtraRuleUpdated,
		&resourceType, &resourceID,
		map[string]any{
			"extra_type": string(extraType),
			"points":     input.Points,
			"is_active":  input.IsActive,
		},
	)
	s.log.Info("extra rule updated",
		zap.String("extra_type", string(extraType)),
		zap.Int("points", input.Points),
		zap.Bool("is_active", input.IsActive),
		zap.String("actor", fmt.Sprintf("user:%d", actorID)),
	)
	return updated, nil
}

var _ ExtraRuleService = (*extraRuleService)(nil)
