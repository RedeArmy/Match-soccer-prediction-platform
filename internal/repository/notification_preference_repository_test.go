package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── NotificationPreferenceRepository ─────────────────────────────────────────

func TestNotificationPreferenceRepository_Upsert_Insert(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	pref := &domain.NotificationPreference{
		UserID:       u.ID,
		EventType:    "payment_confirmed",
		ChannelEmail: true,
		ChannelPush:  true,
		ChannelInApp: true,
	}
	if err := repo.Upsert(context.Background(), pref); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestNotificationPreferenceRepository_Upsert_Update(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	pref := &domain.NotificationPreference{
		UserID: u.ID, EventType: "payment_confirmed",
		ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
	}
	if err := repo.Upsert(context.Background(), pref); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	pref.ChannelEmail = false
	pref.ChannelPush = false
	if err := repo.Upsert(context.Background(), pref); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.Get(context.Background(), u.ID, "payment_confirmed")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ChannelEmail {
		t.Error("expected ChannelEmail=false after update")
	}
	if got.ChannelPush {
		t.Error("expected ChannelPush=false after update")
	}
	if !got.ChannelInApp {
		t.Error("expected ChannelInApp=true after update")
	}
}

func TestNotificationPreferenceRepository_Get_ReturnsPreference(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	_ = repo.Upsert(context.Background(), &domain.NotificationPreference{
		UserID: u.ID, EventType: "withdrawal_completed",
		ChannelEmail: false, ChannelPush: true, ChannelInApp: true,
	})

	got, err := repo.Get(context.Background(), u.ID, "withdrawal_completed")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if got.UserID != u.ID {
		t.Errorf("UserID: got %d, want %d", got.UserID, u.ID)
	}
	if got.EventType != "withdrawal_completed" {
		t.Errorf("EventType: got %q, want withdrawal_completed", got.EventType)
	}
	if got.ChannelEmail {
		t.Error("expected ChannelEmail=false")
	}
	if !got.ChannelPush {
		t.Error("expected ChannelPush=true")
	}
}

func TestNotificationPreferenceRepository_Get_NotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	_, err := repo.Get(context.Background(), u.ID, "nonexistent_event")
	if !isNotFound(err) {
		t.Errorf(fmtNotFoundErr, err)
	}
}

func TestNotificationPreferenceRepository_ListByUser_ReturnsAll(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	events := []string{"payment_confirmed", "withdrawal_completed", "group_joined"}
	for _, ev := range events {
		_ = repo.Upsert(context.Background(), &domain.NotificationPreference{
			UserID: u.ID, EventType: ev,
			ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
		})
	}

	got, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 preferences, got %d", len(got))
	}
}

func TestNotificationPreferenceRepository_ListByUser_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	got, err := repo.ListByUser(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 preferences, got %d", len(got))
	}
}

// ── Cancelled-context error paths ─────────────────────────────────────────────

func TestNotificationPreferenceRepository_Get_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Get(ctx, u.ID, "payment_confirmed")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationPreferenceRepository_ListByUser_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.ListByUser(ctx, u.ID)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationPreferenceRepository_Upsert_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := repo.Upsert(ctx, &domain.NotificationPreference{
		UserID: u.ID, EventType: "payment_confirmed",
		ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
	})
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

// ── DisableAllEmail / GlobalEmailOptedOut ─────────────────────────────────────

func TestNotificationPreferenceRepository_DisableAllEmail_SetsGlobalOptOut(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	if err := repo.DisableAllEmail(context.Background(), u.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	optedOut, err := repo.GlobalEmailOptedOut(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GlobalEmailOptedOut: %v", err)
	}
	if !optedOut {
		t.Error("expected GlobalEmailOptedOut=true after DisableAllEmail")
	}
}

func TestNotificationPreferenceRepository_GlobalEmailOptedOut_False_WhenNoSentinel(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	optedOut, err := repo.GlobalEmailOptedOut(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if optedOut {
		t.Error("expected GlobalEmailOptedOut=false when no sentinel row exists")
	}
}

func TestNotificationPreferenceRepository_DisableAllEmail_Idempotent(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	for range 3 {
		if err := repo.DisableAllEmail(context.Background(), u.ID); err != nil {
			t.Fatalf("DisableAllEmail (idempotent): %v", err)
		}
	}

	optedOut, err := repo.GlobalEmailOptedOut(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GlobalEmailOptedOut: %v", err)
	}
	if !optedOut {
		t.Error("expected GlobalEmailOptedOut=true after repeated DisableAllEmail calls")
	}
}

func TestNotificationPreferenceRepository_DisableAllEmail_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.DisableAllEmail(ctx, u.ID); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationPreferenceRepository_GlobalEmailOptedOut_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.GlobalEmailOptedOut(ctx, u.ID)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationPreferenceRepository_ListByUser_IsolatedByUser(t *testing.T) {
	cleanTables(t)
	u1 := seedUser(t)
	u2 := seedUser(t)
	repo := repository.NewPostgresNotificationPreferenceRepository(testDB)

	_ = repo.Upsert(context.Background(), &domain.NotificationPreference{
		UserID: u1.ID, EventType: "payment_confirmed",
		ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
	})
	_ = repo.Upsert(context.Background(), &domain.NotificationPreference{
		UserID: u2.ID, EventType: "payment_confirmed",
		ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
	})
	_ = repo.Upsert(context.Background(), &domain.NotificationPreference{
		UserID: u2.ID, EventType: "withdrawal_completed",
		ChannelEmail: true, ChannelPush: true, ChannelInApp: true,
	})

	got, err := repo.ListByUser(context.Background(), u1.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 preference for u1, got %d", len(got))
	}
}
