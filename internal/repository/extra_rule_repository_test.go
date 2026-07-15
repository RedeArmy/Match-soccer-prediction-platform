package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── ExtraRuleRepository ───────────────────────────────────────────────────────
//
// The extra_rules table is seeded by migration 000225 and is NOT truncated by
// cleanTables (it is reference data, not test data — same treatment as
// scoring_rules). Tests that call Update must restore the original values via
// t.Cleanup to avoid cross-test pollution.

func TestExtraRuleRepository_List_ReturnsBothTypes(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	rules, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 extra-type rows, got %d", len(rules))
	}
}

func TestExtraRuleRepository_GetByType_FirstScorer_SeedDefault(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	rule, err := repo.GetByType(context.Background(), domain.ExtraTypeFirstScorer)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rule == nil {
		t.Fatal("expected rule for first_scorer, got nil")
	}
	if rule.Points != domain.DefaultExtraFirstScorerPoints {
		t.Errorf("points: got %d, want %d (seed default)", rule.Points, domain.DefaultExtraFirstScorerPoints)
	}
	if !rule.IsActive {
		t.Error("expected is_active=true by default")
	}
}

func TestExtraRuleRepository_GetByType_HalftimeResult_SeedDefault(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	rule, err := repo.GetByType(context.Background(), domain.ExtraTypeHalftimeResult)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if rule == nil {
		t.Fatal("expected rule for halftime_result, got nil")
	}
	if rule.Points != domain.DefaultExtraHalftimeResultPoints {
		t.Errorf("points: got %d, want %d (seed default)", rule.Points, domain.DefaultExtraHalftimeResultPoints)
	}
}

func TestExtraRuleRepository_GetByType_UnknownTypeReturnsNil(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	rule, err := repo.GetByType(context.Background(), domain.ExtraType("unknown_type"))
	if err != nil {
		t.Fatalf("expected nil error for missing type, got %v", err)
	}
	if rule != nil {
		t.Errorf("expected nil rule for unknown type, got %+v", rule)
	}
}

func TestExtraRuleRepository_Update_PersistsNewValues(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)
	extraType := domain.ExtraTypeFirstScorer

	original, err := repo.GetByType(context.Background(), extraType)
	if err != nil || original == nil {
		t.Fatalf("pre-read first_scorer: %v", err)
	}
	t.Cleanup(func() {
		if _, err := repo.Update(context.Background(), original); err != nil {
			t.Errorf("cleanup: restore first_scorer values: %v", err)
		}
	})

	updated, err := repo.Update(context.Background(), &domain.ExtraRule{
		ExtraType: extraType,
		Points:    7,
		IsActive:  false,
	})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Points != 7 {
		t.Errorf("points: got %d, want 7", updated.Points)
	}
	if updated.IsActive {
		t.Error("is_active: expected false after update")
	}

	fetched, err := repo.GetByType(context.Background(), extraType)
	if err != nil {
		t.Fatalf("re-read after update: %v", err)
	}
	if fetched.Points != 7 {
		t.Errorf("round-trip points: got %d, want 7", fetched.Points)
	}
}

func TestExtraRuleRepository_Update_UnknownTypeReturnsNotFound(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	_, err := repo.Update(context.Background(), &domain.ExtraRule{
		ExtraType: domain.ExtraType("invalid_type"),
		Points:    3,
		IsActive:  true,
	})
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestExtraRuleRepository_List_CancelledContextReturnsError(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.List(ctx)
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}

func TestExtraRuleRepository_GetByType_CancelledContextReturnsError(t *testing.T) {
	repo := repository.NewPostgresExtraRuleRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.GetByType(ctx, domain.ExtraTypeFirstScorer)
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}
