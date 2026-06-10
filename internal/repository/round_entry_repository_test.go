package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── QuinielaRoundEntryRepository ─────────────────────────────────────────────

func TestRoundEntryRepository_Upsert_CreatesEntry(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	entry, err := repo.Upsert(context.Background(), q.ID, u.ID, "1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if entry == nil {
		t.Fatal("expected entry, got nil")
	}
	if entry.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if entry.QuinielaID != q.ID {
		t.Errorf("quiniela_id: got %d, want %d", entry.QuinielaID, q.ID)
	}
	if entry.UserID != u.ID {
		t.Errorf("user_id: got %d, want %d", entry.UserID, u.ID)
	}
	if entry.RoundKey != "1" {
		t.Errorf("round_key: got %q, want %q", entry.RoundKey, "1")
	}
}

func TestRoundEntryRepository_Upsert_Idempotent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	first, err := repo.Upsert(context.Background(), q.ID, u.ID, "2")
	if err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second, err := repo.Upsert(context.Background(), q.ID, u.ID, "2")
	if err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	if second.ID != first.ID {
		t.Errorf("expected same id on idempotent upsert; got %d, want %d", second.ID, first.ID)
	}
}

func TestRoundEntryRepository_ListByRound_ReturnsEntries(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	q := seedQuiniela(t, u1.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	if _, err := repo.Upsert(context.Background(), q.ID, u1.ID, "3"); err != nil {
		t.Fatalf("upsert u1: %v", err)
	}
	if _, err := repo.Upsert(context.Background(), q.ID, u2.ID, "3"); err != nil {
		t.Fatalf("upsert u2: %v", err)
	}
	// Different round — should not appear.
	if _, err := repo.Upsert(context.Background(), q.ID, u1.ID, "4"); err != nil {
		t.Fatalf("upsert u1 round 4: %v", err)
	}

	entries, err := repo.ListByRound(context.Background(), q.ID, "3")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries for round 3, got %d", len(entries))
	}
}

func TestRoundEntryRepository_ListByRound_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	entries, err := repo.ListByRound(context.Background(), q.ID, "1")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}
}

func TestRoundEntryRepository_HasEntry_TrueAfterUpsert(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	if _, err := repo.Upsert(context.Background(), q.ID, u.ID, "5"); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	ok, err := repo.HasEntry(context.Background(), q.ID, u.ID, "5")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !ok {
		t.Error("expected HasEntry=true after upsert")
	}
}

func TestRoundEntryRepository_HasEntry_FalseWhenAbsent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	ok, err := repo.HasEntry(context.Background(), q.ID, u.ID, "5")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if ok {
		t.Error("expected HasEntry=false for missing entry")
	}
}

func TestRoundEntryRepository_ListByQuiniela_ReturnsAllRounds(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	for _, rk := range []string{"1", "2", "3"} {
		if _, err := repo.Upsert(context.Background(), q.ID, u.ID, rk); err != nil {
			t.Fatalf("upsert round %s: %v", rk, err)
		}
	}

	entries, err := repo.ListByQuiniela(context.Background(), q.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestRoundEntryRepository_Upsert_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Upsert(ctx, q.ID, u.ID, "9")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestRoundEntryRepository_ListByRound_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ListByRound(ctx, q.ID, "1")
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}

func TestRoundEntryRepository_ListByQuiniela_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	repo := repository.NewPostgresQuinielaRoundEntryRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ListByQuiniela(ctx, q.ID)
	if err == nil {
		t.Error(repoMsgCancelledCtx)
	}
}
