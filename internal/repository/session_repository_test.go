package repository_test

import (
	"context"
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
