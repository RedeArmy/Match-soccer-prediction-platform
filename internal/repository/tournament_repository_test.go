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

	slot, err := repo.CreateSlot(context.Background(), "winner_group_a", "")
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

	if _, err := repo.CreateSlot(context.Background(), "winner_group_b", ""); err != nil {
		t.Fatalf("first create: %v", err)
	}

	_, err := repo.CreateSlot(context.Background(), "winner_group_b", "")
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate label, got %v", err)
	}
}

func TestTournamentRepository_GetSlot_Found(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "runner_up_group_a", "")
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

	if _, err := repo.CreateSlot(context.Background(), "winner_group_b", ""); err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if _, err := repo.CreateSlot(context.Background(), "runner_up_group_b", ""); err != nil {
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

	created, err := repo.CreateSlot(context.Background(), "winner_group_c", "")
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

// ── FindSlotByAutoSource ──────────────────────────────────────────────────────

func TestTournamentRepository_FindSlotByAutoSource_Found(t *testing.T) {
	cleanTables(t)
	skipIfNoDB(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slot, err := repo.CreateSlot(context.Background(), "winner_group_d", "1er Grupo D")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if _, err := testDB.Exec(context.Background(),
		`UPDATE tournament_slots SET auto_source='1D' WHERE id=$1`, slot.ID,
	); err != nil {
		t.Fatalf("set auto_source: %v", err)
	}

	found, err := repo.FindSlotByAutoSource(context.Background(), "1D")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if found == nil {
		t.Fatal("expected slot, got nil")
	}
	if found.ID != slot.ID {
		t.Errorf(fmtIDMismatch, found.ID, slot.ID)
	}
	if found.AutoSource == nil || *found.AutoSource != "1D" {
		t.Errorf("auto_source: want 1D, got %v", found.AutoSource)
	}
}

func TestTournamentRepository_FindSlotByAutoSource_NotFound_ReturnsNil(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	found, err := repo.FindSlotByAutoSource(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if found != nil {
		t.Errorf("expected nil for unknown auto_source, got %+v", found)
	}
}

// ── AutoConfirmSlot ───────────────────────────────────────────────────────────

func TestTournamentRepository_AutoConfirmSlot_SetsTeam(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "winner_group_e", "")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	confirmed, err := repo.AutoConfirmSlot(context.Background(), created.ID, repoMexico)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if confirmed.Team == nil || *confirmed.Team != repoMexico {
		t.Errorf("team: want %s, got %v", repoMexico, confirmed.Team)
	}
	if confirmed.ConfirmedAt == nil {
		t.Error("confirmed_at: want non-nil after auto-confirmation")
	}
}

func TestTournamentRepository_AutoConfirmSlot_AlreadyConfirmed_ReturnsExisting(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	created, err := repo.CreateSlot(context.Background(), "winner_group_f", "")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}
	if _, err := repo.AutoConfirmSlot(context.Background(), created.ID, repoMexico); err != nil {
		t.Fatalf("first confirm: %v", err)
	}

	// Second call: slot already has a team — must return the existing slot without error.
	got, err := repo.AutoConfirmSlot(context.Background(), created.ID, repoBrazil)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.Team == nil || *got.Team != repoMexico {
		t.Errorf("team: want original %s, got %v", repoMexico, got.Team)
	}
}

func TestTournamentRepository_AutoConfirmSlot_NotFound_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	_, err := repo.AutoConfirmSlot(context.Background(), 99999, repoMexico)
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestTournamentRepository_AutoConfirmSlot_PropagatesTeamToMatch(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresTournamentRepository(testDB)

	slot, err := repo.CreateSlot(context.Background(), "r32_01_a", "")
	if err != nil {
		t.Fatalf(fmtCreateErr, err)
	}

	var matchID int
	if err := testDB.QueryRow(context.Background(),
		`INSERT INTO matches (home_team, away_team, status, phase, kickoff_at, home_slot_id)
		 VALUES ($1, $2, 'scheduled', 'round_of_32', NOW()+INTERVAL '1 day', $3)
		 RETURNING id`,
		"1A", repoArgentina, slot.ID,
	).Scan(&matchID); err != nil {
		t.Fatalf("seed knockout match: %v", err)
	}

	if _, err := repo.AutoConfirmSlot(context.Background(), slot.ID, repoMexico); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	var homeTeam string
	if err := testDB.QueryRow(context.Background(),
		`SELECT home_team FROM matches WHERE id=$1`, matchID,
	).Scan(&homeTeam); err != nil {
		t.Fatalf("get match home_team: %v", err)
	}
	if homeTeam != repoMexico {
		t.Errorf("match home_team: want %s, got %s", repoMexico, homeTeam)
	}
}
