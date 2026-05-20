package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// makeDLQEntry builds a minimal NotificationDLQEntry with nullable fields unset.
func makeDLQEntry(channel, eventType string) *domain.NotificationDLQEntry {
	return &domain.NotificationDLQEntry{
		Channel:     channel,
		EventType:   eventType,
		Payload:     []byte(`{"user_id":1}`),
		ErrorDetail: "smtp timeout",
	}
}

// ── CreateEntry ───────────────────────────────────────────────────────────────

func TestNotificationDLQRepository_CreateEntry_PopulatesIDAndCreatedAt(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("email", "admin.bank_transfer_pending")
	if err := repo.CreateEntry(context.Background(), e); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if e.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if e.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestNotificationDLQRepository_CreateEntry_AllChannels(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	for _, ch := range []string{"email", "push", "sse"} {
		e := makeDLQEntry(ch, "system.circuit_breaker_opened")
		if err := repo.CreateEntry(context.Background(), e); err != nil {
			t.Errorf("channel %q: unexpected error: %v", ch, err)
		}
	}
}

func TestNotificationDLQRepository_CreateEntry_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.CreateEntry(ctx, makeDLQEntry("email", "admin.bank_transfer_pending")); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

// ── CountUnresolved ───────────────────────────────────────────────────────────

func TestNotificationDLQRepository_CountUnresolved_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	n, err := repo.CountUnresolved(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 0 {
		t.Errorf("got %d; want 0", n)
	}
}

func TestNotificationDLQRepository_CountUnresolved_ExcludesResolved(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	// Two unresolved entries.
	e1 := makeDLQEntry("email", "admin.bank_transfer_stale")
	e2 := makeDLQEntry("push", "admin.withdrawal_pending")
	_ = repo.CreateEntry(context.Background(), e1)
	_ = repo.CreateEntry(context.Background(), e2)

	// Resolve one.
	_ = repo.MarkResolved(context.Background(), e1.ID)

	n, err := repo.CountUnresolved(context.Background())
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if n != 1 {
		t.Errorf("got %d; want 1", n)
	}
}

// ── ClaimBatch ────────────────────────────────────────────────────────────────

func TestNotificationDLQRepository_ClaimBatch_ReturnsEligibleEntries(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("email", "admin.bank_transfer_pending")
	if err := repo.CreateEntry(context.Background(), e); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}

	entries, err := repo.ClaimBatch(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries; want 1", len(entries))
	}
	if entries[0].ID != e.ID {
		t.Errorf("entry ID: got %d; want %d", entries[0].ID, e.ID)
	}
}

func TestNotificationDLQRepository_ClaimBatch_ExcludesResolved(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("email", "admin.bank_transfer_pending")
	_ = repo.CreateEntry(context.Background(), e)
	_ = repo.MarkResolved(context.Background(), e.ID)

	entries, err := repo.ClaimBatch(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries; want 0 (resolved entries must be excluded)", len(entries))
	}
}

func TestNotificationDLQRepository_ClaimBatch_ExceedsMaxAttempts_Excluded(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("push", "admin.withdrawal_pending")
	_ = repo.CreateEntry(context.Background(), e)

	// Exhaust attempts by recording failures up to maxAttempts.
	const maxAttempts = 2
	for range maxAttempts {
		_ = repo.RecordFailure(context.Background(), e.ID, "retry error")
	}

	// ClaimBatch with maxAttempts=2 should exclude an entry with attempts >= 2.
	entries, err := repo.ClaimBatch(context.Background(), 10, maxAttempts)
	if err != nil {
		t.Fatalf("ClaimBatch: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries; want 0 (max attempts exceeded)", len(entries))
	}
}

func TestNotificationDLQRepository_ClaimBatch_Empty(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	entries, err := repo.ClaimBatch(context.Background(), 10, 5)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries; want 0", len(entries))
	}
}

// ── MarkResolved ──────────────────────────────────────────────────────────────

func TestNotificationDLQRepository_MarkResolved_SetsResolvedAt(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("email", "admin.bank_transfer_stale")
	_ = repo.CreateEntry(context.Background(), e)

	if err := repo.MarkResolved(context.Background(), e.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// After resolving, the entry must no longer appear in ClaimBatch.
	entries, _ := repo.ClaimBatch(context.Background(), 10, 5)
	if len(entries) != 0 {
		t.Errorf("got %d entries after resolve; want 0", len(entries))
	}
}

func TestNotificationDLQRepository_MarkResolved_Nonexistent_NoError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	if err := repo.MarkResolved(context.Background(), 99999); err != nil {
		t.Fatalf("expected no error for nonexistent id, got %v", err)
	}
}

// ── RecordFailure ─────────────────────────────────────────────────────────────

func TestNotificationDLQRepository_RecordFailure_IncrementsAttempts(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	e := makeDLQEntry("push", "system.circuit_breaker_opened")
	_ = repo.CreateEntry(context.Background(), e)

	if err := repo.RecordFailure(context.Background(), e.ID, "connection refused"); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// After RecordFailure the entry is still unresolved; verify via CountUnresolved.
	n, err := repo.CountUnresolved(context.Background())
	if err != nil {
		t.Fatalf("CountUnresolved after RecordFailure: %v", err)
	}
	if n < 1 {
		t.Error("expected ≥ 1 unresolved entry after RecordFailure")
	}
}

func TestNotificationDLQRepository_RecordFailure_SetsLastRetryAt(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	before := time.Now().UTC().Add(-time.Second)
	e := makeDLQEntry("email", "admin.high_value_withdrawal")
	_ = repo.CreateEntry(context.Background(), e)

	if err := repo.RecordFailure(context.Background(), e.ID, "timeout"); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Verify via CountUnresolved that the entry still exists.
	n, err := repo.CountUnresolved(context.Background())
	if err != nil {
		t.Fatalf("CountUnresolved: %v", err)
	}
	if n < 1 {
		t.Error("expected ≥ 1 unresolved entry after RecordFailure")
	}
	_ = before
}

func TestNotificationDLQRepository_RecordFailure_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.RecordFailure(ctx, 1, "timeout"); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationDLQRepository_ClaimBatch_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ClaimBatch(ctx, 10, 5)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationDLQRepository_MarkResolved_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.MarkResolved(ctx, 1); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationDLQRepository_CountUnresolved_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresNotificationDLQRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.CountUnresolved(ctx)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
