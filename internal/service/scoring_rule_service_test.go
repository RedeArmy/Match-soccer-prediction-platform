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

// scoringRuleFixture returns a seeded ScoringRule for group_stage.
func scoringRuleFixture() *domain.ScoringRule {
	return &domain.ScoringRule{
		ID:             1,
		Phase:          domain.PhaseGroupStage,
		ExactScore:     5,
		CorrectOutcome: 2,
		GoalDifference: 1,
		IsActive:       true,
		UpdatedAt:      time.Now(),
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestScoringRuleService_List_ReturnsList(t *testing.T) {
	rule := scoringRuleFixture()
	repo := &stubScoringRuleRepo{rule: rule}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	rules, err := svc.List(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rules) != 1 || rules[0].Phase != domain.PhaseGroupStage {
		t.Errorf("expected 1 group_stage rule, got %v", rules)
	}
}

func TestScoringRuleService_List_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("db down")
	repo := &stubScoringRuleRepo{err: repoErr}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.List(context.Background())
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}

// ── GetByPhase ────────────────────────────────────────────────────────────────

func TestScoringRuleService_GetByPhase_ReturnsRule(t *testing.T) {
	rule := scoringRuleFixture()
	repo := &stubScoringRuleRepo{rule: rule}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Phase != domain.PhaseGroupStage {
		t.Errorf("expected group_stage, got %s", got.Phase)
	}
}

func TestScoringRuleService_GetByPhase_NilRuleReturnsNotFound(t *testing.T) {
	repo := &stubScoringRuleRepo{rule: nil}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.GetByPhase(context.Background(), domain.MatchPhase("unknown_phase"))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestScoringRuleService_GetByPhase_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("timeout")
	repo := &stubScoringRuleRepo{err: repoErr}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestScoringRuleService_Update_ValidInput_ReturnsUpdatedRule(t *testing.T) {
	rule := scoringRuleFixture()
	repo := &stubScoringRuleRepo{rule: rule}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	got, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 5, CorrectOutcome: 2, GoalDifference: 1, IsActive: true}, 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ExactScore != 5 || got.CorrectOutcome != 2 {
		t.Errorf("unexpected rule values: %+v", got)
	}
}

func TestScoringRuleService_Update_NegativeExactScore_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: -1, CorrectOutcome: 2, GoalDifference: 1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for negative exact_score, got %v", err)
	}
}

func TestScoringRuleService_Update_NegativeCorrectOutcome_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 5, CorrectOutcome: -1, GoalDifference: 1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for negative correct_outcome, got %v", err)
	}
}

func TestScoringRuleService_Update_CorrectOutcomeEqualExact_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	// correctOutcome >= exactScore when exactScore > 0 violates the hierarchy
	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 3, CorrectOutcome: 3, GoalDifference: 1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation when correct_outcome == exact_score, got %v", err)
	}
}

func TestScoringRuleService_Update_CorrectOutcomeGreaterThanExact_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 2, CorrectOutcome: 5, GoalDifference: 1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation when correct_outcome > exact_score, got %v", err)
	}
}

func TestScoringRuleService_Update_NegativeExtraTimeBonus_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseRoundOf16,
		ScoringRuleInput{ExactScore: 8, CorrectOutcome: 4, GoalDifference: 2, ExtraTimeBonus: -1, PenaltiesBonus: 2, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for negative extra_time_bonus, got %v", err)
	}
}

func TestScoringRuleService_Update_NegativePenaltiesBonus_ReturnsValidation(t *testing.T) {
	repo := &stubScoringRuleRepo{}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseRoundOf16,
		ScoringRuleInput{ExactScore: 8, CorrectOutcome: 4, GoalDifference: 2, ExtraTimeBonus: 1, PenaltiesBonus: -1, IsActive: true}, 1)
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for negative penalties_bonus, got %v", err)
	}
}

func TestScoringRuleService_Update_AllZeros_AllowsDisabledRule(t *testing.T) {
	// exact_score == 0 bypasses the hierarchy check — used to disable scoring.
	rule := scoringRuleFixture()
	rule.ExactScore = 0
	rule.CorrectOutcome = 0
	rule.GoalDifference = 0
	repo := &stubScoringRuleRepo{rule: rule}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 0, CorrectOutcome: 0, GoalDifference: 0, IsActive: false}, 1)
	if err != nil {
		t.Errorf("expected nil for all-zero rule, got %v", err)
	}
}

func TestScoringRuleService_Update_PropagatesRepoError(t *testing.T) {
	repoErr := errors.New("phase not found")
	repo := &stubScoringRuleRepo{err: repoErr}
	svc := NewScoringRuleService(repo, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.Update(context.Background(), domain.PhaseGroupStage,
		ScoringRuleInput{ExactScore: 5, CorrectOutcome: 2, GoalDifference: 1, IsActive: true}, 1)
	if !errors.Is(err, repoErr) {
		t.Errorf("expected repo error, got %v", err)
	}
}

// ── configForPhase fallback ───────────────────────────────────────────────────

func TestConfigForPhase_ActiveRule_UsesPhaseValues(t *testing.T) {
	rule := &domain.ScoringRule{
		Phase:          domain.PhaseQuarterFinal,
		ExactScore:     10,
		CorrectOutcome: 5,
		GoalDifference: 2,
		IsActive:       true,
	}
	svc := &scoringService{
		ruleRepo: &stubScoringRuleRepo{rule: rule},
		params:   &noopSystemParamService{},
		log:      zap.NewNop(),
	}

	cfg := svc.configForPhase(context.Background(), domain.PhaseQuarterFinal)
	if cfg.exactScore != 10 || cfg.correctOutcome != 5 || cfg.goalDifference != 2 {
		t.Errorf("expected phase-specific values (10/5/2), got %+v", cfg)
	}
}

func TestConfigForPhase_InactiveRule_FallsBackToGlobal(t *testing.T) {
	rule := &domain.ScoringRule{
		Phase:    domain.PhaseGroupStage,
		IsActive: false,
	}
	svc := &scoringService{
		ruleRepo: &stubScoringRuleRepo{rule: rule},
		params:   &noopSystemParamService{},
		log:      zap.NewNop(),
	}

	cfg := svc.configForPhase(context.Background(), domain.PhaseGroupStage)
	// noopSystemParamService returns 0 for GetInt; domain constants are the fallback
	if cfg.exactScore != domain.PointsExactScore {
		t.Errorf("expected domain constant fallback, got exact_score=%d", cfg.exactScore)
	}
}

func TestConfigForPhase_NilRule_FallsBackToGlobal(t *testing.T) {
	svc := &scoringService{
		ruleRepo: &stubScoringRuleRepo{rule: nil},
		params:   &noopSystemParamService{},
		log:      zap.NewNop(),
	}

	cfg := svc.configForPhase(context.Background(), domain.PhaseGroupStage)
	if cfg.exactScore != domain.PointsExactScore {
		t.Errorf("expected domain constant fallback, got exact_score=%d", cfg.exactScore)
	}
}

func TestConfigForPhase_RepoError_FallsBackToGlobal(t *testing.T) {
	svc := &scoringService{
		ruleRepo: &stubScoringRuleRepo{err: errors.New("db fail")},
		params:   &noopSystemParamService{},
		log:      zap.NewNop(),
	}

	cfg := svc.configForPhase(context.Background(), domain.PhaseGroupStage)
	if cfg.exactScore != domain.PointsExactScore {
		t.Errorf("expected domain constant fallback on repo error, got exact_score=%d", cfg.exactScore)
	}
}
