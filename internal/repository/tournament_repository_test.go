package repository_test

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── TournamentRepository ──────────────────────────────────────────────────────

func TestTournamentRepository_CreateSlot_ReturnsSlot(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slot, err := repo.CreateSlot(context.Background(), "winner_group_a")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if slot.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if slot.Label != "winner_group_a" {
		t.Errorf("label: want winner_group_a, got %s", slot.Label)
	}
	if slot.Team != nil {
		t.Errorf("team: want nil on creation, got %v", slot.Team)
	}
}

func TestTournamentRepository_CreateSlot_DuplicateLabel_ReturnsConflict(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	if _, err := repo.CreateSlot(context.Background(), "winner_group_b"); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := repo.CreateSlot(context.Background(), "winner_group_b")
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate label, got %v", err)
	}
}

func TestTournamentRepository_GetSlot_Found(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "runner_up_group_a")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, err := repo.GetSlot(context.Background(), created.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got == nil {
		t.Fatal("expected slot, got nil")
	}
	if got.ID != created.ID {
		t.Errorf(fmtIDMismatch, got.ID, created.ID)
	}
}

func TestTournamentRepository_GetSlot_NotFoundReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	got, err := repo.GetSlot(context.Background(), 99999)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got != nil {
		t.Errorf(fmtExpectNilGot, got)
	}
}

func TestTournamentRepository_ListSlots_ReturnsList(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	if _, err := repo.CreateSlot(context.Background(), "winner_group_b"); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if _, err := repo.CreateSlot(context.Background(), "runner_up_group_b"); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	slots, err := repo.ListSlots(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(slots) != 2 {
		t.Errorf("slots: want 2, got %d", len(slots))
	}
}

func TestTournamentRepository_ListSlots_EmptyWhenNone(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slots, err := repo.ListSlots(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(slots) != 0 {
		t.Errorf("slots: want 0, got %d", len(slots))
	}
}

func TestTournamentRepository_ConfirmSlot_SetsTeam(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "winner_group_c")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	confirmed, err := repo.ConfirmSlot(context.Background(), created.ID, u.ID, repoMexico)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if confirmed.Team == nil || *confirmed.Team != repoMexico {
		t.Errorf("team: want Mexico, got %v", confirmed.Team)
	}
	if confirmed.ConfirmedAt == nil {
		t.Error("confirmed_at: want non-nil after confirmation")
	}
	if confirmed.ConfirmedByUserID == nil || *confirmed.ConfirmedByUserID != u.ID {
		t.Errorf("confirmed_by_user_id: want %d, got %v", u.ID, confirmed.ConfirmedByUserID)
	}
}

func TestTournamentRepository_ConfirmSlot_NotFoundWhenMissing(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	_, err := repo.ConfirmSlot(context.Background(), 99999, u.ID, repoMexico)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}
