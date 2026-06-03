package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
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

// ── EraseUserPII ──────────────────────────────────────────────────────────────

func TestPostgresPurger_EraseUserPII_ClearsAuditLog(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	if _, err := testDB.Exec(ctx,
		`INSERT INTO audit_log (actor_id, action) VALUES ($1, 'test.action')`, u.ID); err != nil {
		t.Fatalf("seed audit log: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("EraseUserPII: %v", err)
	}

	var actorID *int
	if err := testDB.QueryRow(ctx,
		`SELECT actor_id FROM audit_log LIMIT 1`).Scan(&actorID); err != nil {
		t.Fatalf("query audit log: %v", err)
	}
	if actorID != nil {
		t.Errorf("expected actor_id = NULL after erasure, got %d", *actorID)
	}
}

func TestPostgresPurger_EraseUserPII_ClearsPaymentRecords(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	q := seedQuiniela(t, u.ID)
	seedPaymentRecord(t, q.ID, u.ID)

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("EraseUserPII: %v", err)
	}

	var userID *int
	if err := testDB.QueryRow(ctx,
		`SELECT user_id FROM payment_records LIMIT 1`).Scan(&userID); err != nil {
		t.Fatalf("query payment records: %v", err)
	}
	if userID != nil {
		t.Errorf("expected user_id = NULL after erasure, got %d", *userID)
	}
}

func TestPostgresPurger_EraseUserPII_DeletesPredictions(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	m := seedMatch(t)

	predRepo := repository.NewPostgresPredictionRepository(testDB)
	if err := predRepo.Create(ctx, &domain.Prediction{
		UserID:    u.ID,
		MatchID:   m.ID,
		HomeScore: 1,
		AwayScore: 0,
	}); err != nil {
		t.Fatalf("seed prediction: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("EraseUserPII: %v", err)
	}

	var count int
	if err := testDB.QueryRow(ctx,
		`SELECT COUNT(*) FROM predictions WHERE user_id = $1`, u.ID).Scan(&count); err != nil {
		t.Fatalf("query predictions: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 predictions after erasure, got %d", count)
	}
}

func TestPostgresPurger_EraseUserPII_DeletesTiebreakers(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)
	cfg := seedTiebreakerConfig(t)

	tbRepo := repository.NewPostgresTiebreakerRepository(testDB)
	if err := tbRepo.Upsert(ctx, &domain.Tiebreaker{
		UserID:             u.ID,
		TiebreakerConfigID: cfg.ID,
		Prediction:         42,
	}); err != nil {
		t.Fatalf("seed tiebreaker: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("EraseUserPII: %v", err)
	}

	var count int
	if err := testDB.QueryRow(ctx,
		`SELECT COUNT(*) FROM tiebreakers WHERE user_id = $1`, u.ID).Scan(&count); err != nil {
		t.Fatalf("query tiebreakers: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 tiebreakers after erasure, got %d", count)
	}
}

func TestPostgresPurger_EraseUserPII_IsIdempotent(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("first EraseUserPII: %v", err)
	}
	if err := purger.EraseUserPII(ctx, u.ID); err != nil {
		t.Fatalf("second EraseUserPII (idempotent): %v", err)
	}
}

func TestPostgresPurger_EraseUserPII_CancelledContext_ReturnsError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	purger := repository.NewPostgresPurger(testDB)
	if err := purger.EraseUserPII(ctx, 1); err == nil {
		t.Error("expected error with cancelled context, got nil")
	}
}

// ── PurgeOldFXHistory ─────────────────────────────────────────────────────────

func TestPostgresPurger_PurgeOldFXHistory_DeletesExpiredRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	if _, err := testDB.Exec(ctx,
		`INSERT INTO exchange_rate_history
		   (reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct, source, effective_at)
		 VALUES (7.75, 7.87, 7.63, 0.015, 0.015, 'banguat', NOW() - INTERVAL '91 days')`,
	); err != nil {
		t.Fatalf("insert old fx history row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	n, err := purger.PurgeOldFXHistory(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged fx history row, got %d", n)
	}
}

func TestPostgresPurger_PurgeOldFXHistory_PreservesRecentRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	if _, err := testDB.Exec(ctx,
		`INSERT INTO exchange_rate_history
		   (reference_rate, buy_rate, sell_rate, buy_margin_pct, sell_margin_pct, source, effective_at)
		 VALUES (7.75, 7.87, 7.63, 0.015, 0.015, 'banguat', NOW())`,
	); err != nil {
		t.Fatalf("insert recent fx history row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	n, err := purger.PurgeOldFXHistory(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged rows (recent row within retention), got %d", n)
	}
}

// ── PurgeOldOutboxEntries ─────────────────────────────────────────────────────

func TestPostgresPurger_PurgeOldOutboxEntries_DeletesTerminalExpiredRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	if _, err := testDB.Exec(ctx,
		`INSERT INTO domain_outbox
		   (event_type, aggregate_id, aggregate_type, payload, status, created_at)
		 VALUES ('test.event', '1', 'test', '{}', 'done', NOW() - INTERVAL '31 days')`,
	); err != nil {
		t.Fatalf("insert old outbox row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := purger.PurgeOldOutboxEntries(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged outbox entry, got %d", n)
	}
}

func TestPostgresPurger_PurgeOldOutboxEntries_PreservesRecentRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	if _, err := testDB.Exec(ctx,
		`INSERT INTO domain_outbox
		   (event_type, aggregate_id, aggregate_type, payload, status, created_at)
		 VALUES ('test.event', '1', 'test', '{}', 'done', NOW())`,
	); err != nil {
		t.Fatalf("insert recent outbox row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := purger.PurgeOldOutboxEntries(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged rows (row is recent), got %d", n)
	}
}

func TestPostgresPurger_PurgeExpiredPaymentIntents_DeletesExpiredPendingRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	// Insert a pending intent whose expires_at is 8 days in the past.
	intent := seedPaymentIntent(t, u.ID, 1000)
	if _, err := testDB.Exec(ctx,
		`UPDATE payment_intents SET expires_at = NOW() - INTERVAL '8 days' WHERE id = $1`, intent.ID,
	); err != nil {
		t.Fatalf("backdate expires_at: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-7 * 24 * time.Hour) // 7-day post-expiry window
	n, err := purger.PurgeExpiredPaymentIntents(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("expected 1 purged intent, got %d", n)
	}
}

func TestPostgresPurger_PurgeExpiredPaymentIntents_PreservesRecentlyExpiredRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	// Intent expired yesterday — within the 7-day retention window.
	intent := seedPaymentIntent(t, u.ID, 1000)
	if _, err := testDB.Exec(ctx,
		`UPDATE payment_intents SET expires_at = NOW() - INTERVAL '1 day' WHERE id = $1`, intent.ID,
	); err != nil {
		t.Fatalf("backdate expires_at: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	n, err := purger.PurgeExpiredPaymentIntents(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged rows (within retention window), got %d", n)
	}
}

func TestPostgresPurger_PurgeExpiredPaymentIntents_PreservesCapturedIntents(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()
	u := seedUser(t)

	// Captured intent — must never be purged regardless of expires_at.
	intent := seedPaymentIntent(t, u.ID, 1000)
	if _, err := testDB.Exec(ctx,
		`UPDATE payment_intents
		    SET status = 'captured', capture_id = 'cap_test',
		        expires_at = NOW() - INTERVAL '30 days'
		  WHERE id = $1`, intent.ID,
	); err != nil {
		t.Fatalf("mark captured: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-7 * 24 * time.Hour)
	n, err := purger.PurgeExpiredPaymentIntents(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged rows (captured intents must not be purged), got %d", n)
	}
}

func TestPostgresPurger_PurgeOldOutboxEntries_PreservesPendingRows(t *testing.T) {
	cleanTables(t)
	ctx := context.Background()

	// pending status — must never be purged regardless of age
	if _, err := testDB.Exec(ctx,
		`INSERT INTO domain_outbox
		   (event_type, aggregate_id, aggregate_type, payload, status, created_at)
		 VALUES ('test.event', '1', 'test', '{}', 'pending', NOW() - INTERVAL '31 days')`,
	); err != nil {
		t.Fatalf("insert old pending outbox row: %v", err)
	}

	purger := repository.NewPostgresPurger(testDB)
	cutoff := time.Now().Add(-30 * 24 * time.Hour)
	n, err := purger.PurgeOldOutboxEntries(ctx, cutoff)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("expected 0 purged rows (pending rows must not be purged), got %d", n)
	}
}
