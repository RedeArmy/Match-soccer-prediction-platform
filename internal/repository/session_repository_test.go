package repository_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/repository"
)

func newSessionRepo(t *testing.T) *repository.PostgresSessionRepository {
	t.Helper()
	skipIfNoDB(t)
	return repository.NewPostgresSessionRepository(testDB)
}

func TestPostgresSessionRepository_RevokeSession_IsRevoked(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()

	sid := "test_session_revoke_" + t.Name()
	// Not revoked before insertion.
	revoked, err := repo.IsRevoked(ctx, sid)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if revoked {
		t.Fatal("expected not-revoked before RevokeSession")
	}

	if err := repo.RevokeSession(ctx, sid, "user_test_subject"); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}

	revoked, err = repo.IsRevoked(ctx, sid)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !revoked {
		t.Fatal("expected revoked after RevokeSession")
	}

	// Cleanup.
	_, _ = testDB.Exec(ctx, `DELETE FROM revoked_sessions WHERE sid = $1`, sid)
}

func TestPostgresSessionRepository_RevokeSession_Idempotent(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()

	sid := "test_session_idem_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM revoked_sessions WHERE sid = $1`, sid)
	})

	if err := repo.RevokeSession(ctx, sid, "user_a"); err != nil {
		t.Fatalf("first RevokeSession: %v", err)
	}
	if err := repo.RevokeSession(ctx, sid, "user_b"); err != nil {
		t.Fatalf("second RevokeSession (idempotent): %v", err)
	}
}

func TestPostgresSessionRepository_IsRevoked_UnknownSID(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()

	revoked, err := repo.IsRevoked(ctx, "sid_that_does_not_exist_ever")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if revoked {
		t.Fatal("unknown sid must not be reported as revoked")
	}
}

func TestPostgresSessionRepository_PruneRevoked(t *testing.T) {
	repo := newSessionRepo(t)
	ctx := context.Background()

	// Insert one old record and one recent record.
	old := "test_session_prune_old_" + t.Name()
	recent := "test_session_prune_recent_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM revoked_sessions WHERE sid IN ($1, $2)`, old, recent)
	})

	// Insert with explicit old timestamp.
	_, err := testDB.Exec(ctx,
		`INSERT INTO revoked_sessions (sid, user_id, revoked_at) VALUES ($1, 'u', NOW() - INTERVAL '10 days')`,
		old,
	)
	if err != nil {
		t.Fatalf("insert old record: %v", err)
	}
	if err := repo.RevokeSession(ctx, recent, "user_recent"); err != nil {
		t.Fatalf("insert recent record: %v", err)
	}

	// Prune entries older than 8 days — removes old, keeps recent.
	cutoff := time.Now().Add(-8 * 24 * time.Hour)
	n, err := repo.PruneRevoked(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n < 1 {
		t.Fatalf("expected ≥1 row pruned, got %d", n)
	}

	// old must be gone; recent must still be revoked.
	revoked, _ := repo.IsRevoked(ctx, old)
	if revoked {
		t.Error("old record should have been pruned")
	}
	revoked, _ = repo.IsRevoked(ctx, recent)
	if !revoked {
		t.Error("recent record should still be present")
	}
}

// ── PostgresSessionStartRepository integration tests ─────────────────────────

func newSessionStartRepo(t *testing.T) *repository.PostgresSessionStartRepository {
	t.Helper()
	skipIfNoDB(t)
	return repository.NewPostgresSessionStartRepository(testDB)
}

// TestPostgresSessionStartRepository_UpsertSessionStart_FirstCall_RecordsNow
// verifies that the first call creates a row with a timestamp close to NOW().
func TestPostgresSessionStartRepository_UpsertSessionStart_FirstCall_RecordsNow(t *testing.T) {
	repo := newSessionStartRepo(t)
	ctx := context.Background()

	sid := "test_start_first_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM session_starts WHERE sid = $1`, sid)
	})

	before := time.Now().Add(-time.Second)
	got, err := repo.UpsertSessionStart(ctx, sid)
	after := time.Now().Add(time.Second)

	if err != nil {
		t.Fatalf("UpsertSessionStart: %v", err)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("started_at %v is outside expected window [%v, %v]", got, before, after)
	}
}

// TestPostgresSessionStartRepository_UpsertSessionStart_Idempotent verifies
// that subsequent calls for the same sid return the original timestamp unchanged.
// This is the core correctness property: the upsert must never overwrite
// started_at on conflict, ensuring the session origin is stable across requests.
func TestPostgresSessionStartRepository_UpsertSessionStart_Idempotent(t *testing.T) {
	repo := newSessionStartRepo(t)
	ctx := context.Background()

	sid := "test_start_idem_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM session_starts WHERE sid = $1`, sid)
	})

	first, err := repo.UpsertSessionStart(ctx, sid)
	if err != nil {
		t.Fatalf("first UpsertSessionStart: %v", err)
	}

	// Simulate the next JWT refresh request: same sid, called slightly later.
	time.Sleep(5 * time.Millisecond)

	second, err := repo.UpsertSessionStart(ctx, sid)
	if err != nil {
		t.Fatalf("second UpsertSessionStart: %v", err)
	}

	if !first.Equal(second) {
		t.Errorf("idempotency violation: first=%v second=%v (started_at must not change on conflict)", first, second)
	}
}

// TestPostgresSessionStartRepository_UpsertSessionStart_ConcurrentInsert
// verifies that concurrent upserts for the same new sid produce the same
// timestamp without a unique-constraint error. This exercises the ON CONFLICT
// path under concurrency, which is the most common production scenario when
// multiple replicas handle the first request from an OAuth session simultaneously.
func TestPostgresSessionStartRepository_UpsertSessionStart_ConcurrentInsert(t *testing.T) {
	repo := newSessionStartRepo(t)
	ctx := context.Background()

	sid := "test_start_concurrent_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM session_starts WHERE sid = $1`, sid)
	})

	const goroutines = 5
	results := make([]time.Time, goroutines)
	errs := make([]error, goroutines)

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func(idx int) {
			defer wg.Done()
			results[idx], errs[idx] = repo.UpsertSessionStart(ctx, sid)
		}(i)
	}
	wg.Wait()

	// All goroutines must succeed without error.
	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, err)
		}
	}

	// All goroutines must return the same started_at (the winner of the INSERT race).
	for i, ts := range results {
		if !ts.Equal(results[0]) {
			t.Errorf("goroutine %d returned different started_at: got %v, want %v", i, ts, results[0])
		}
	}
}

// TestPostgresSessionStartRepository_PruneSessionStarts verifies that rows
// older than the cutoff are deleted and recent rows are retained.
func TestPostgresSessionStartRepository_PruneSessionStarts(t *testing.T) {
	repo := newSessionStartRepo(t)
	ctx := context.Background()

	old := "test_start_prune_old_" + t.Name()
	recent := "test_start_prune_recent_" + t.Name()
	t.Cleanup(func() {
		_, _ = testDB.Exec(ctx, `DELETE FROM session_starts WHERE sid IN ($1, $2)`, old, recent)
	})

	_, err := testDB.Exec(ctx,
		`INSERT INTO session_starts (sid, started_at) VALUES ($1, NOW() - INTERVAL '10 days')`,
		old,
	)
	if err != nil {
		t.Fatalf("insert old row: %v", err)
	}
	if _, err := repo.UpsertSessionStart(ctx, recent); err != nil {
		t.Fatalf("insert recent row: %v", err)
	}

	// Cutoff at 8 days: removes old, keeps recent.
	n, err := repo.PruneSessionStarts(ctx, time.Now().Add(-8*24*time.Hour))
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n < 1 {
		t.Fatalf("expected ≥1 row pruned, got %d", n)
	}

	var count int
	_ = testDB.QueryRow(ctx, `SELECT COUNT(*) FROM session_starts WHERE sid = $1`, old).Scan(&count)
	if count != 0 {
		t.Error("old row should have been pruned")
	}
	_ = testDB.QueryRow(ctx, `SELECT COUNT(*) FROM session_starts WHERE sid = $1`, recent).Scan(&count)
	if count != 1 {
		t.Error("recent row should still be present")
	}
}
