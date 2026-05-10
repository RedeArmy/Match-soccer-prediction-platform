package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── ScoringRuleRepository ─────────────────────────────────────────────────────
//
// The scoring_rules table is seeded by migration 000063 and is NOT truncated
// by cleanTables (it is reference data, not test data). Tests that call Update
// must restore the original values via t.Cleanup to avoid cross-test pollution.

func TestScoringRuleRepository_List_ReturnsAllSevenPhases(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rules) != 7 {
		t.Fatalf("expected 7 phase rows, got %d", len(rules))
	}
}

func TestScoringRuleRepository_List_OrderedByTournamentProgression(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	want := []domain.MatchPhase{
		domain.PhaseGroupStage,
		domain.PhaseRoundOf32,
		domain.PhaseRoundOf16,
		domain.PhaseQuarterFinal,
		domain.PhaseSemiFinal,
		domain.PhaseThirdPlace,
		domain.PhaseFinal,
	}
	for i, r := range rules {
		if r.Phase != want[i] {
			t.Errorf("index %d: got phase %q, want %q", i, r.Phase, want[i])
		}
	}
}

func TestScoringRuleRepository_List_SeedValuesEscalate(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// group_stage is the baseline; every knockout phase must score ≥ group_stage.
	baseline := rules[0]
	for _, r := range rules[1:] {
		if r.ExactScore < baseline.ExactScore {
			t.Errorf("phase %q: exact_score %d is less than group_stage baseline %d",
				r.Phase, r.ExactScore, baseline.ExactScore)
		}
	}
}

func TestScoringRuleRepository_GetByPhase_Found(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rule, err := repo.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rule == nil {
		t.Fatal("expected rule for group_stage, got nil")
	}
	if rule.Phase != domain.PhaseGroupStage {
		t.Errorf("phase: got %q, want group_stage", rule.Phase)
	}
	if rule.ExactScore != 5 {
		t.Errorf("exact_score: got %d, want 5 (seed default)", rule.ExactScore)
	}
}

func TestScoringRuleRepository_GetByPhase_FinalSeedValues(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rule, err := repo.GetByPhase(context.Background(), domain.PhaseFinal)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rule == nil {
		t.Fatal("expected rule for final, got nil")
	}
	if rule.ExactScore != 15 || rule.CorrectOutcome != 8 || rule.GoalDifference != 3 {
		t.Errorf("final seed: got %d/%d/%d, want 15/8/3",
			rule.ExactScore, rule.CorrectOutcome, rule.GoalDifference)
	}
	if rule.ExtraTimeBonus != 1 || rule.PenaltiesBonus != 2 {
		t.Errorf("final win-method bonuses: got %d/%d, want 1/2",
			rule.ExtraTimeBonus, rule.PenaltiesBonus)
	}
}

func TestScoringRuleRepository_GetByPhase_GroupStageBonusesAreZero(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rule, err := repo.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rule == nil {
		t.Fatal("expected rule for group_stage, got nil")
	}
	if rule.ExtraTimeBonus != 0 || rule.PenaltiesBonus != 0 {
		t.Errorf("group_stage bonuses must be 0, got extra_time=%d penalties=%d",
			rule.ExtraTimeBonus, rule.PenaltiesBonus)
	}
}

func TestScoringRuleRepository_List_KnockoutPhasesHaveBonuses(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	for _, r := range rules {
		if r.Phase == domain.PhaseGroupStage {
			continue
		}
		if r.ExtraTimeBonus != 1 || r.PenaltiesBonus != 2 {
			t.Errorf("phase %q: expected bonuses 1/2, got %d/%d",
				r.Phase, r.ExtraTimeBonus, r.PenaltiesBonus)
		}
	}
}

func TestScoringRuleRepository_GetByPhase_UnknownPhaseReturnsNil(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	rule, err := repo.GetByPhase(context.Background(), domain.MatchPhase("unknown_phase"))
	if err != nil {
		t.Fatalf("expected nil error for missing phase, got %v", err)
	}
	if rule != nil {
		t.Errorf("expected nil rule for unknown phase, got %+v", rule)
	}
}

func TestScoringRuleRepository_Update_PersistsNewValues(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)
	phase := domain.PhaseRoundOf16

	// Read original so we can restore it.
	original, err := repo.GetByPhase(context.Background(), phase)
	if err != nil || original == nil {
		t.Fatalf("pre-read round_of_16: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Update(context.Background(), original); err != nil {
			t.Errorf("cleanup: restore round_of_16 values: %v", err)
		}
	})

	updated, err := repo.Update(context.Background(), &domain.ScoringRule{
		Phase:          phase,
		ExactScore:     20,
		CorrectOutcome: 10,
		GoalDifference: 4,
		ExtraTimeBonus: 3,
		PenaltiesBonus: 5,
		IsActive:       false,
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.ExactScore != 20 || updated.CorrectOutcome != 10 || updated.GoalDifference != 4 {
		t.Errorf("updated values: got %d/%d/%d, want 20/10/4",
			updated.ExactScore, updated.CorrectOutcome, updated.GoalDifference)
	}
	if updated.ExtraTimeBonus != 3 || updated.PenaltiesBonus != 5 {
		t.Errorf("bonus values: got %d/%d, want 3/5",
			updated.ExtraTimeBonus, updated.PenaltiesBonus)
	}
	if updated.IsActive {
		t.Error("is_active: expected false after update")
	}

	// Round-trip: GetByPhase must reflect the persisted change.
	fetched, err := repo.GetByPhase(context.Background(), phase)
	if err != nil {
		t.Fatalf("re-read after update: %v", err)
	}
	if fetched.ExactScore != 20 {
		t.Errorf("round-trip exact_score: got %d, want 20", fetched.ExactScore)
	}
}

func TestScoringRuleRepository_Update_IsActiveToggle(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)
	phase := domain.PhaseQuarterFinal

	original, err := repo.GetByPhase(context.Background(), phase)
	if err != nil || original == nil {
		t.Fatalf("pre-read quarter_final: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Update(context.Background(), original); err != nil {
			t.Errorf("cleanup: restore quarter_final is_active: %v", err)
		}
	})

	toggled, err := repo.Update(context.Background(), &domain.ScoringRule{
		Phase:          phase,
		ExactScore:     original.ExactScore,
		CorrectOutcome: original.CorrectOutcome,
		GoalDifference: original.GoalDifference,
		IsActive:       !original.IsActive,
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if toggled.IsActive == original.IsActive {
		t.Error("is_active: expected toggle to flip the flag")
	}
}

func TestScoringRuleRepository_Update_UnknownPhaseReturnsNotFound(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	_, err := repo.Update(context.Background(), &domain.ScoringRule{
		Phase:          domain.MatchPhase("invalid_phase"),
		ExactScore:     5,
		CorrectOutcome: 2,
		GoalDifference: 1,
		IsActive:       true,
	})
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestScoringRuleRepository_List_CancelledContextReturnsError(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.List(ctx)
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}

func TestScoringRuleRepository_GetByPhase_CancelledContextReturnsError(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.GetByPhase(ctx, domain.PhaseGroupStage)
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}

func TestScoringRuleRepository_Update_CancelledContextReturnsError(t *testing.T) {
	repo := repository.NewPostgresScoringRuleRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Update(ctx, &domain.ScoringRule{
		Phase:          domain.PhaseGroupStage,
		ExactScore:     5,
		CorrectOutcome: 2,
		GoalDifference: 1,
		IsActive:       true,
	})
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}
