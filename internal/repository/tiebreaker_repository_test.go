package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── TiebreakerRepository ──────────────────────────────────────────────────────

func TestTiebreakerRepository_Create_HydratesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	tb := &domain.Tiebreaker{UserID: u.ID, TiebreakerConfigID: cfg.ID, Prediction: 42}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if tb.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if tb.TiebreakerConfigID != cfg.ID {
		t.Errorf("config_id: got %d, want %d", tb.TiebreakerConfigID, cfg.ID)
	}
}

func TestTiebreakerRepository_GetByUser_Found(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, TiebreakerConfigID: cfg.ID, Prediction: 10}
	if err := repo.Create(context.Background(), tb); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	got, err := repo.GetByUser(context.Background(), u.ID, cfg.ID)
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
	seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	got, err := repo.GetByUser(context.Background(), 99999, 1)
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
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, TiebreakerConfigID: cfg.ID, Prediction: 7}
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
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)
	tb := &domain.Tiebreaker{UserID: u.ID, TiebreakerConfigID: cfg.ID, Prediction: 3}
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
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, TiebreakerConfigID: cfg.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, TiebreakerConfigID: cfg.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Unbounded())
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
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, TiebreakerConfigID: cfg.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, TiebreakerConfigID: cfg.ID, Prediction: 5}); err != nil {
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

func TestTiebreakerRepository_ListAll_ZeroLimitReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	_, err := repo.ListAll(context.Background(), repository.Pagination{Limit: 0})
	if err == nil {
		t.Fatal("expected validation error for zero limit, got nil")
	}
	if !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error, got %v", err)
	}
}

func TestTiebreakerRepository_ListAll_WithOffset(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, TiebreakerConfigID: cfg.ID, Prediction: 3}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, TiebreakerConfigID: cfg.ID, Prediction: 5}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListAll(context.Background(), repository.Pagination{Limit: 10, Offset: 1})
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 {
		t.Errorf("expected 1 tiebreaker with offset=1 from 2 total, got %d", len(results))
	}
}

func TestTiebreakerRepository_ListByUserIDsForConfig_ReturnsFiltered(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	cfg := seedTiebreakerConfig(t)
	repo := repository.NewPostgresTiebreakerRepository(testDB)

	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u1.ID, TiebreakerConfigID: cfg.ID, Prediction: 7}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if err := repo.Create(context.Background(), &domain.Tiebreaker{UserID: u2.ID, TiebreakerConfigID: cfg.ID, Prediction: 9}); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	results, err := repo.ListByUserIDsForConfig(context.Background(), []int{u1.ID}, cfg.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(results) != 1 || results[0].UserID != u1.ID {
		t.Errorf("expected 1 tiebreaker for u1 only, got %v", results)
	}
}

// ── TiebreakerConfigRepository — new multi-config API ────────────────────────

func TestTiebreakerConfigRepository_GetByPhase_ReturnsNilWhenEmpty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.GetByPhase(context.Background(), domain.PhaseGroupStage)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg != nil {
		t.Errorf("expected nil before any phase question is set, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_UpsertForPhase_CreatesAndUpdates(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.UpsertForPhase(context.Background(), domain.PhaseGroupStage, "Goals in group stage?")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Question != "Goals in group stage?" {
		t.Errorf("question: want 'Goals in group stage?', got %q", cfg.Question)
	}
	if cfg.Phase == nil || *cfg.Phase != domain.PhaseGroupStage {
		t.Errorf("phase: want %q, got %v", domain.PhaseGroupStage, cfg.Phase)
	}
	if cfg.QuinielaID != nil {
		t.Errorf("quiniela_id: want nil, got %v", cfg.QuinielaID)
	}

	cfg2, err := repo.UpsertForPhase(context.Background(), domain.PhaseGroupStage, "Updated question?")
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if cfg2.Question != "Updated question?" {
		t.Errorf("updated question: got %q", cfg2.Question)
	}
}

func TestTiebreakerConfigRepository_GetByPhase_ReturnsAfterUpsert(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.UpsertForPhase(context.Background(), domain.PhaseRoundOf16, "Round of 16 goals?"); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.GetByPhase(context.Background(), domain.PhaseRoundOf16)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config after upsert, got nil")
	}
	if cfg.Phase == nil || *cfg.Phase != domain.PhaseRoundOf16 {
		t.Errorf("phase: want %q, got %v", domain.PhaseRoundOf16, cfg.Phase)
	}
}

func TestTiebreakerConfigRepository_GetByQuiniela_ReturnsNilWhenEmpty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.GetByQuiniela(context.Background(), 9999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg != nil {
		t.Errorf("expected nil before any group question is set, got %+v", cfg)
	}
}

func TestTiebreakerConfigRepository_UpsertForQuiniela_CreatesAndUpdates(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.UpsertForQuiniela(context.Background(), q.ID, "Group-specific question?")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.Question != "Group-specific question?" {
		t.Errorf("question: want 'Group-specific question?', got %q", cfg.Question)
	}
	if cfg.QuinielaID == nil || *cfg.QuinielaID != q.ID {
		t.Errorf("quiniela_id: want %d, got %v", q.ID, cfg.QuinielaID)
	}
	if cfg.Phase != nil {
		t.Errorf("phase: want nil, got %v", cfg.Phase)
	}

	cfg2, err := repo.UpsertForQuiniela(context.Background(), q.ID, "Updated group question?")
	if err != nil {
		t.Fatalf("upsert update: %v", err)
	}
	if cfg2.Question != "Updated group question?" {
		t.Errorf("updated question: got %q", cfg2.Question)
	}
}

func TestTiebreakerConfigRepository_GetByQuiniela_ReturnsAfterUpsert(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	if _, err := repo.UpsertForQuiniela(context.Background(), q.ID, "My group question?"); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	cfg, err := repo.GetByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if cfg == nil {
		t.Fatal("expected config after upsert, got nil")
	}
	if cfg.QuinielaID == nil || *cfg.QuinielaID != q.ID {
		t.Errorf("quiniela_id: want %d, got %v", q.ID, cfg.QuinielaID)
	}
}

func TestTiebreakerConfigRepository_SetResultByID_SetsResult(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	cfg, err := repo.Upsert(context.Background(), repoTotalGoals)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	if err := repo.SetResultByID(context.Background(), cfg.ID, 77); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.Get(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Result == nil || *got.Result != 77 {
		t.Errorf("result: want 77, got %v", got.Result)
	}
}

func TestTiebreakerConfigRepository_SetResultByID_NotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTiebreakerConfigRepository(testDB)

	err := repo.SetResultByID(context.Background(), 99999, 10)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}
