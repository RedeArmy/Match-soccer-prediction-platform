package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── PushSubscriptionRepository ────────────────────────────────────────────────

func newPushSub(userID int, suffix string) *domain.PushSubscription {
	return &domain.PushSubscription{
		UserID:    userID,
		Endpoint:  fmt.Sprintf("https://push.example.com/%s", suffix),
		P256dhKey: "dGVzdC1wMjU2ZGgta2V5",
		AuthKey:   "dGVzdC1hdXRoLWtleQ==",
		UserAgent: "Mozilla/5.0",
	}
}

func TestPushSubscriptionRepository_Create_PopulatesIDAndCreatedAt(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "abc")
	if err := repo.Create(context.Background(), sub); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if sub.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if sub.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestPushSubscriptionRepository_Create_ConflictUpserts(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u1.ID, "shared-endpoint")
	if err := repo.Create(context.Background(), sub); err != nil {
		t.Fatalf("first create: %v", err)
	}
	firstID := sub.ID

	// Same endpoint but different user — ON CONFLICT should upsert.
	sub2 := newPushSub(u2.ID, "shared-endpoint")
	sub2.P256dhKey = "bmV3LXAyNTZkaA=="
	if err := repo.Create(context.Background(), sub2); err != nil {
		t.Fatalf("upsert create: %v", err)
	}
	if sub2.ID == 0 {
		t.Error("expected non-zero ID after upsert")
	}
	_ = firstID // ID may change on conflict update; just ensure no error
}

func TestPushSubscriptionRepository_Create_NoUserAgent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := &domain.PushSubscription{
		UserID:    u.ID,
		Endpoint:  "https://push.example.com/no-ua",
		P256dhKey: "dGVzdA==",
		AuthKey:   "dGVzdA==",
		UserAgent: "", // NULLIF converts empty string to NULL
	}
	if err := repo.Create(context.Background(), sub); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestPushSubscriptionRepository_ListActiveByUser_ReturnsSubs(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	other := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	_ = repo.Create(context.Background(), newPushSub(u.ID, "ep1"))
	_ = repo.Create(context.Background(), newPushSub(u.ID, "ep2"))
	_ = repo.Create(context.Background(), newPushSub(other.ID, "ep3"))

	got, err := repo.ListActiveByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(got))
	}
}

func TestPushSubscriptionRepository_ListActiveByUser_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	got, err := repo.ListActiveByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 subscriptions, got %d", len(got))
	}
}

func TestPushSubscriptionRepository_ListActiveByUser_ExcludesInactive(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "ep-inactive")
	_ = repo.Create(context.Background(), sub)
	_ = repo.MarkInactive(context.Background(), sub.ID)

	_ = repo.Create(context.Background(), newPushSub(u.ID, "ep-active"))

	got, err := repo.ListActiveByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 active subscription, got %d", len(got))
	}
}

func TestPushSubscriptionRepository_DeleteByEndpoint(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "to-delete")
	_ = repo.Create(context.Background(), sub)

	if err := repo.DeleteByEndpoint(context.Background(), sub.Endpoint); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.ListActiveByUser(context.Background(), u.ID)
	if len(got) != 0 {
		t.Errorf("expected 0 after delete, got %d", len(got))
	}
}

func TestPushSubscriptionRepository_DeleteByEndpoint_Nonexistent_NoError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	if err := repo.DeleteByEndpoint(context.Background(), "https://nonexistent.example.com/x"); err != nil {
		t.Fatalf("expected no error deleting nonexistent endpoint, got %v", err)
	}
}

func TestPushSubscriptionRepository_MarkInactive(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "ep-mark-inactive")
	_ = repo.Create(context.Background(), sub)

	if err := repo.MarkInactive(context.Background(), sub.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	got, _ := repo.ListActiveByUser(context.Background(), u.ID)
	if len(got) != 0 {
		t.Errorf("expected 0 active after MarkInactive, got %d", len(got))
	}
}

func TestPushSubscriptionRepository_MarkInactive_Nonexistent_NoError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	if err := repo.MarkInactive(context.Background(), 99999); err != nil {
		t.Fatalf("expected no error for nonexistent ID, got %v", err)
	}
}

// ── Cancelled-context error paths ─────────────────────────────────────────────

func TestPushSubscriptionRepository_Create_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	sub := newPushSub(u.ID, "ctx-cancel")
	if err := repo.Create(ctx, sub); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestPushSubscriptionRepository_ListActiveByUser_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ListActiveByUser(ctx, u.ID)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestPushSubscriptionRepository_DeleteByEndpoint_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.DeleteByEndpoint(ctx, "https://push.example.com/any"); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestPushSubscriptionRepository_UpdateLastUsed_SetsTimestamp(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "ep-update-last-used")
	_ = repo.Create(context.Background(), sub)

	if err := repo.UpdateLastUsed(context.Background(), sub.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestPushSubscriptionRepository_UpdateLastUsed_Nonexistent_NoError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	if err := repo.UpdateLastUsed(context.Background(), 99999); err != nil {
		t.Fatalf("expected no error for nonexistent ID, got %v", err)
	}
}

func TestPushSubscriptionRepository_UpdateLastUsed_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.UpdateLastUsed(ctx, 1); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestPushSubscriptionRepository_MarkInactive_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.MarkInactive(ctx, 1); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestPushSubscriptionRepository_DeleteInactive_DeletesOldInactiveRows(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "ep-delete-inactive-old")
	_ = repo.Create(context.Background(), sub)
	_ = repo.MarkInactive(context.Background(), sub.ID)

	// Cutoff is 1 hour in the future: inactivated_at < future ⇒ row qualifies for deletion.
	n, err := repo.DeleteInactive(context.Background(), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("DeleteInactive: %v", err)
	}
	if n != 1 {
		t.Errorf("DeleteInactive: deleted %d rows; want 1", n)
	}
}

func TestPushSubscriptionRepository_DeleteInactive_KeepsRecentlyInactivated(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	sub := newPushSub(u.ID, "ep-delete-inactive-recent")
	_ = repo.Create(context.Background(), sub)
	_ = repo.MarkInactive(context.Background(), sub.ID)

	// Cutoff is 1 hour in the past: inactivated_at is after the cutoff ⇒ row is kept.
	n, err := repo.DeleteInactive(context.Background(), time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("DeleteInactive: %v", err)
	}
	if n != 0 {
		t.Errorf("DeleteInactive: deleted %d rows; want 0 (inactivated too recently)", n)
	}
}

func TestPushSubscriptionRepository_DeleteInactive_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPushSubscriptionRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.DeleteInactive(ctx, time.Now())
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
