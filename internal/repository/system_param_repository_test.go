package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── SystemParamRepository ─────────────────────────────────────────────────────

func TestSystemParamRepository_Set_NewKey(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Set(context.Background(), repoScoringExact, "5", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p.Key != repoScoringExact || p.Value != "5" {
		t.Errorf("param mismatch: got key=%q value=%q", p.Key, p.Value)
	}
}

func TestSystemParamRepository_Set_ExistingKeyUpdatesValue(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	_, _ = repo.Set(context.Background(), repoScoringExact, "5", 0)
	updated, err := repo.Set(context.Background(), repoScoringExact, "7", 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if updated.Value != "7" {
		t.Errorf("expected value %q, got %q", "7", updated.Value)
	}
}

func TestSystemParamRepository_Get_Found(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "feature.x", "true", "general")
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Get(context.Background(), "feature.x")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p == nil || p.Value != "true" {
		t.Errorf("expected param with value %q, got %v", "true", p)
	}
}

func TestSystemParamRepository_Get_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	p, err := repo.Get(context.Background(), "does.not.exist")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if p != nil {
		t.Errorf(fmtExpectNilGot, p)
	}
}

func TestSystemParamRepository_GetAll(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "a.key", "1", "general")
	seedSystemParam(t, "b.key", "2", repoScoringCategory)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	all, err := repo.GetAll(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(all) != 2 {
		t.Errorf("expected 2 params, got %d", len(all))
	}
}

func TestSystemParamRepository_GetByCategory(t *testing.T) {
	cleanTables(t)
	seedSystemParam(t, "scoring.a", "1", repoScoringCategory)
	seedSystemParam(t, "scoring.b", "2", repoScoringCategory)
	seedSystemParam(t, "payment.x", "3", "payment")
	repo := repository.NewPostgresSystemParamRepository(testDB)

	results, err := repo.GetByCategory(context.Background(), repoScoringCategory)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 scoring params, got %d", len(results))
	}
}

func TestSystemParamRepository_BulkSet(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	err := repo.BulkSet(context.Background(), map[string]string{
		"bulk.a": "alpha",
		"bulk.b": "beta",
	}, 0)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	all, _ := repo.GetAll(context.Background())
	if len(all) != 2 {
		t.Errorf("expected 2 params after BulkSet, got %d", len(all))
	}
}

func TestSystemParamRepository_BulkSet_EmptyIsNoop(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresSystemParamRepository(testDB)

	if err := repo.BulkSet(context.Background(), nil, 0); err != nil {
		t.Fatalf("empty BulkSet should not error: %v", err)
	}
}
