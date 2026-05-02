package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── TiebreakerRepository ──────────────────────────────────────────────────────

func TestTiebreakerRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 42}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestTiebreakerRepository_GetByUser_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 10}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected tiebreaker, got nil")
	}
	if got.Prediction != 10 {
		t.Errorf("prediction: got %d, want 10", got.Prediction)
	}
}

func TestTiebreakerRepository_GetByUser_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	got, err := repo.GetByUser(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestTiebreakerRepository_Update_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 7}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	tb.Prediction = 9
	if err := repo.Update(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.Prediction != 9 {
		t.Errorf("prediction not updated: got %d", tb.Prediction)
	}
}

func TestTiebreakerRepository_Update_NotFound_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	ghost := &domain.Tiebreaker{ID: 99999, Prediction: 5}

	if err := repo.Update(context.Background(), ghost); !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestTiebreakerRepository_ListByUserIDs_ReturnsRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, Prediction: 3}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	tbs, err := repo.ListByUserIDs(context.Background(), []int{u.ID})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(tbs) != 1 {
		t.Errorf("expected 1 tiebreaker, got %d", len(tbs))
	}
}

// ── TiebreakerConfigRepository ────────────────────────────────────────────────

func TestTiebreakerConfigRepository_Get_ReturnsNilWhenEmpty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg != nil {
		t.Errorf("expected nil before any question is set, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_Upsert_CreatesAndUpdates(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Upsert(context.Background(), "Total goals in the Final")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg.ID != 1 {
		t.Errorf("id: want 1, got %d", cfg.ID)
	}
	if cfg.Question != "Total goals in the Final" {
		t.Errorf("question: want 'Total goals in the Final', got %q", cfg.Question)
	}
	if cfg.Result != nil {
		t.Errorf("result: want nil before confirmation, got %v", cfg.Result)
	}

	cfg2, err := repo.Upsert(context.Background(), "Total goals in the tournament")
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if cfg2.Question != "Total goals in the tournament" {
		t.Errorf("updated question: want updated text, got %q", cfg2.Question)
	}
}

func TestTiebreakerConfigRepository_Get_ReturnsAfterUpsert(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	_, err := repo.Upsert(context.Background(), repoTotalGoals)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config after upsert, got nil")
	}
	if cfg.Question != repoTotalGoals {
		t.Errorf("question: want 'Total goals', got %q", cfg.Question)
	}
}

func TestTiebreakerConfigRepository_SetResult_SetsResult(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.Upsert(context.Background(), repoTotalGoals); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.SetResult(context.Background(), 42); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg.Result == nil || *cfg.Result != 42 {
		t.Errorf("result: want 42, got %v", cfg.Result)
	}
}

func TestTiebreakerConfigRepository_SetResult_NotFoundWhenNoConfig(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	err := repo.SetResult(context.Background(), 10)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

// ── TiebreakerRepository admin extensions ────────────────────────────────────

func TestTiebreakerRepository_ListAll_ReturnsList(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Pagination{})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 tiebreakers, got %d", len(results))
	}
}

func TestTiebreakerRepository_ListAll_PaginationLimit(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Pagination{Limit: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 tiebreaker with limit=1, got %d", len(results))
	}
}
