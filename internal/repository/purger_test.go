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

// seedSnapshots inserts count leaderboard_snapshot rows for q, spacing
// taken_at one minute apart so each row has a unique rank within its partition.
func seedSnapshots(t *testing.T, quinielaID int, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if _, err := testDB.Exec(context.Background(), `
			INSERT INTO leaderboard_snapshots (quiniela_id, taken_at, entries, schema_version)
			VALUES ($1, NOW() - ($2 * INTERVAL '1 minute'), '[]'::jsonb, 1)`,
			quinielaID, i); err != nil {
			t.Fatalf("seedSnapshots: %v", err)
		}
	}
}

func snapshotCount(t *testing.T, quinielaID int) int {
	t.Helper()
	var n int
	if err := testDB.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM leaderboard_snapshots WHERE quiniela_id = $1`, quinielaID,
	).Scan(&n); err != nil {
		t.Fatalf("snapshotCount: %v", err)
	}
	return n
}

func TestPostgresPurger_PurgeOldSnapshots_RemovesTail(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedSnapshots(t, q.ID, 8)

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeOldSnapshots(ctx, 3)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 5 {
		t.Errorf("expected 5 removed, got %d", n)
	}
	if got := snapshotCount(t, q.ID); got != 3 {
		t.Errorf("expected 3 remaining, got %d", got)
	}
}

func TestPostgresPurger_PurgeOldSnapshots_PreservesLatestWhenBelowThreshold(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedSnapshots(t, q.ID, 2)

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeOldSnapshots(ctx, 5) // keep 5, only 2 exist
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 removed (below threshold), got %d", n)
	}
	if got := snapshotCount(t, q.ID); got != 2 {
		t.Errorf("expected 2 remaining, got %d", got)
	}
}

func TestPostgresPurger_PurgeOldSnapshots_IsolatesPerQuiniela(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q1 := seedQuiniela(t, u.ID)
	q2 := seedQuiniela(t, u.ID)
	seedSnapshots(t, q1.ID, 6)
	seedSnapshots(t, q2.ID, 2)

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeOldSnapshots(ctx, 3)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	// q1: 6−3=3 removed; q2: 2 < 3, none removed
	if n != 3 {
		t.Errorf("expected 3 removed, got %d", n)
	}
	if got := snapshotCount(t, q1.ID); got != 3 {
		t.Errorf("q1: expected 3 remaining, got %d", got)
	}
	if got := snapshotCount(t, q2.ID); got != 2 {
		t.Errorf("q2: expected 2 remaining (untouched), got %d", got)
	}
}

func TestPostgresPurger_PurgeOldSnapshots_EmptyTable_ReturnsZero(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	purger := repository.NewPostgresPurger(testDB)
	n, err := purger.PurgeOldSnapshots(ctx, 5)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 removed on empty table, got %d", n)
	}
}

func TestPostgresPurger_PurgeOldSnapshots_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled — pool rejects the query before hitting the DB

	purger := repository.NewPostgresPurger(testDB)
	_, err := purger.PurgeOldSnapshots(ctx, 5)
	if err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}
