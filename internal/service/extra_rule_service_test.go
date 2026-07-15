package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// extraRuleFixture returns a seeded ExtraRule for first_scorer.
func extraRuleFixture() *domain.ExtraRule {
	return &domain.ExtraRule{
		ID:        1,
		ExtraType: domain.ExtraTypeFirstScorer,
		Points:    domain.DefaultExtraFirstScorerPoints,
		IsActive:  true,
		UpdatedAt: time.Now(),
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestExtraRuleService_List_ReturnsList(t *testing.T) {
	rule := extraRuleFixture()
	repo := &stubExtraRuleRepo{rule: rule}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	rules, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 || rules[0].ExtraType != domain.ExtraTypeFirstScorer {
		t.Errorf("expected 1 first_scorer rule, got %v", rules)
	}
}

func TestExtraRuleService_List_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	repo := &stubExtraRuleRepo{err: repoErr}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.List(context.Background())
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}

// ── GetByType ─────────────────────────────────────────────────────────────────

func TestExtraRuleService_GetByType_ReturnsRule(t *testing.T) {
	rule := extraRuleFixture()
	repo := &stubExtraRuleRepo{rule: rule}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.GetByType(context.Background(), domain.ExtraTypeFirstScorer)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ExtraType != domain.ExtraTypeFirstScorer {
		t.Errorf("expected first_scorer, got %s", got.ExtraType)
	}
}

func TestExtraRuleService_GetByType_NilRuleReturnsNotFound(t *testing.T) {
	repo := &stubExtraRuleRepo{rule: nil}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.GetByType(context.Background(), domain.ExtraType("unknown_type"))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestExtraRuleService_GetByType_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("timeout")
	repo := &stubExtraRuleRepo{err: repoErr}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.GetByType(context.Background(), domain.ExtraTypeFirstScorer)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestExtraRuleService_Update_ValidInput_ReturnsUpdatedRule(t *testing.T) {
	rule := extraRuleFixture()
	repo := &stubExtraRuleRepo{rule: rule}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.Update(context.Background(), domain.ExtraTypeFirstScorer,
		domain.ExtraRuleInput{Points: 7, IsActive: true}, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Points != 7 {
		t.Errorf("unexpected points: got %d, want 7", got.Points)
	}
}

func TestExtraRuleService_Update_NegativePoints_ReturnsValidation(t *testing.T) {
	repo := &stubExtraRuleRepo{}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.ExtraTypeFirstScorer,
		domain.ExtraRuleInput{Points: -1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for negative points, got %v", err)
	}
}

func TestExtraRuleService_Update_UnknownExtraType_ReturnsValidation(t *testing.T) {
	repo := &stubExtraRuleRepo{}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.ExtraType("bogus_type"),
		domain.ExtraRuleInput{Points: 3, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for unknown extra_type, got %v", err)
	}
}

func TestExtraRuleService_Update_ZeroPointsAllowsDisabledRule(t *testing.T) {
	rule := extraRuleFixture()
	rule.Points = 0
	repo := &stubExtraRuleRepo{rule: rule}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.ExtraTypeFirstScorer,
		domain.ExtraRuleInput{Points: 0, IsActive: false}, 1)
	if err != nil {
		t.Errorf("expected nil for zero-point disabled rule, got %v", err)
	}
}

func TestExtraRuleService_Update_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("type not found")
	repo := &stubExtraRuleRepo{err: repoErr}
	svc := NewExtraRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.ExtraTypeFirstScorer,
		domain.ExtraRuleInput{Points: 3, IsActive: true}, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}
