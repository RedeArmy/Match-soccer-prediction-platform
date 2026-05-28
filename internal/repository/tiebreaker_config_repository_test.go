package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── TiebreakerConfigRepository ────────────────────────────────────────────────

func TestTiebreakerConfigRepository_Get_ReturnsNilWhenNoQuestion(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil when no question configured, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_Upsert_CreatesAndRetrievesGlobalQuestion(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	const question = "How many goals will be scored in the final?"
	cfg, err := repo.Upsert(context.Background(), question)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if cfg.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if cfg.Question != question {
		t.Errorf("question: got %q, want %q", cfg.Question, question)
	}

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after Upsert: %v", err)
	}
	if got == nil {
		t.Fatal("expected config after Upsert, got nil")
	}
	if got.Question != question {
		t.Errorf("persisted question: got %q, want %q", got.Question, question)
	}
}

func TestTiebreakerConfigRepository_Upsert_UpdatesExistingQuestion(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.Upsert(context.Background(), "original question"); err != nil {
		t.Fatalf("first Upsert: %v", err)
	}

	const updated = "updated question"
	cfg, err := repo.Upsert(context.Background(), updated)
	if err != nil {
		t.Fatalf("second Upsert: %v", err)
	}
	if cfg.Question != updated {
		t.Errorf("question after update: got %q, want %q", cfg.Question, updated)
	}
}

func TestTiebreakerConfigRepository_UpsertForPhase_CreatesAndRetrieves(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	const question = "Group stage tiebreaker question"
	cfg, err := repo.UpsertForPhase(context.Background(), domain.PhaseGroupStage, question)
	if err != nil {
		t.Fatalf("UpsertForPhase: %v", err)
	}
	if cfg.Question != question {
		t.Errorf("question: got %q, want %q", cfg.Question, question)
	}
	if cfg.Phase == nil || *cfg.Phase != domain.PhaseGroupStage {
		t.Errorf("phase: got %v, want %v", cfg.Phase, domain.PhaseGroupStage)
	}

	got, err := repo.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf("GetByPhase: %v", err)
	}
	if got == nil {
		t.Fatal("expected config, got nil")
	}
	if got.Question != question {
		t.Errorf("persisted question: got %q, want %q", got.Question, question)
	}
}

func TestTiebreakerConfigRepository_GetByPhase_ReturnsNilWhenAbsent(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf("GetByPhase: %v", err)
	}
	if cfg != nil {
		t.Errorf("expected nil for unconfigured phase, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_SetResult_UpdatesGlobalConfig(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.Upsert(context.Background(), "final question"); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	const result = 5
	if err := repo.SetResult(context.Background(), result); err != nil {
		t.Fatalf("SetResult: %v", err)
	}

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("Get after SetResult: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Result == nil || *cfg.Result != result {
		t.Errorf("result: got %v, want %d", cfg.Result, result)
	}
}

func TestTiebreakerConfigRepository_SetResult_ReturnsNotFoundWhenNoConfig(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	err := repo.SetResult(context.Background(), 3)
	if !isNotFound(err) {
		t.Errorf("expected NotFound, got %v", err)
	}
}
