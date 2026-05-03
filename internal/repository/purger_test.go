package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

func TestPostgresPurger_PurgeDeletedUsers_DeletesExpiredRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	if _, err := testDB.Exec(ctx,
		`UPDATE users SET deleted_at = NOW() - INTERVAL '2 days' WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedUsers(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged user, got %d", n)
	}

	var count int
	_ = testDB.QueryRow(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, u.ID).Scan(&count)
	if count != 0 {
		t.Errorf("expected row deleted, still found %d", count)
	}
}

func TestPostgresPurger_PurgeDeletedUsers_PreservesActiveRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	seedUser(t)

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedUsers(ctx, time.Now())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged users for active row, got %d", n)
	}
}

func TestPostgresPurger_PurgeDeletedUsers_PreservesRecentlyDeletedRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	if _, err := testDB.Exec(ctx,
		`UPDATE users SET deleted_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, u.ID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedUsers(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged users (within retention), got %d", n)
	}
}

func TestPostgresPurger_PurgeDeletedQuinielas_DeletesExpiredRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)

	if _, err := testDB.Exec(ctx,
		`UPDATE quinielas SET deleted_at = NOW() - INTERVAL '2 days' WHERE id = $1`, q.ID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedQuinielas(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged quiniela, got %d", n)
	}
}

func TestPostgresPurger_PurgeDeletedQuinielas_PreservesActiveRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	seedQuiniela(t, u.ID)

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedQuinielas(ctx, time.Now())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged quinielas for active row, got %d", n)
	}
}

func TestPostgresPurger_PurgeDeletedQuinielas_PreservesRecentlyDeletedRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)

	if _, err := testDB.Exec(ctx,
		`UPDATE quinielas SET deleted_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, q.ID); err != nil {
		t.Fatalf("mark deleted: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeDeletedQuinielas(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged quinielas (within retention), got %d", n)
	}
}
