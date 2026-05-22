package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// cleanNotifTemplates truncates the notification template tables so tests start
// from a known empty state. These tables are not in the shared cleanTables call
// because they have no FK dependencies on the core entity tables.
func cleanNotifTemplates(t *testing.T) {
	t.Helper()
	_, err := testDB.Exec(context.Background(),
		`TRUNCATE notification_template_history, notification_templates RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("clean notification templates: %v", err)
	}
}

func seedNotifTemplate(t *testing.T, eventType, locale string) {
	t.Helper()
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)
	tmpl := &domain.NotificationTemplate{
		EventType: eventType,
		Locale:    locale,
		TitleTmpl: "Hello",
		BodyTmpl:  "World",
	}
	if err := repo.Upsert(context.Background(), tmpl); err != nil {
		t.Fatalf("seed notification template (%s/%s): %v", eventType, locale, err)
	}
}

// ── Upsert / Get ─────────────────────────────────────────────────────────────

func TestNotificationTemplateRepository_Upsert_Creates(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	tmpl := &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Payment confirmed",
		BodyTmpl:  "Your payment is confirmed.",
	}
	if err := repo.Upsert(context.Background(), tmpl); err != nil {
		t.Fatalf("Upsert: %v", err)
	}

	got, err := repo.Get(context.Background(), "payment.confirmed", "en")
	if err != nil {
		t.Fatalf("Get after Upsert: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil template after Upsert")
	}
	if got.TitleTmpl != "Payment confirmed" {
		t.Errorf("TitleTmpl: got %q, want %q", got.TitleTmpl, "Payment confirmed")
	}
}

func TestNotificationTemplateRepository_Upsert_Updates(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")

	updated := &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated title",
		BodyTmpl:  "Updated body",
	}
	if err := repo.Upsert(context.Background(), updated); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	got, err := repo.Get(context.Background(), "payment.confirmed", "en")
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got == nil || got.TitleTmpl != "Updated title" {
		t.Errorf("TitleTmpl after update: got %q, want %q", got.TitleTmpl, "Updated title")
	}
}

func TestNotificationTemplateRepository_Get_NotFound_ReturnsNil(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	got, err := repo.Get(context.Background(), "nonexistent.event", "en")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing template, got %+v", got)
	}
}

func TestNotificationTemplateRepository_Get_CancelledContext_ReturnsError(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.Get(ctx, "payment.confirmed", "en")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestNotificationTemplateRepository_List_ReturnsAll(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	seedNotifTemplate(t, "payment.confirmed", "es")

	templates, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 2 {
		t.Errorf("List: got %d templates, want 2", len(templates))
	}
}

func TestNotificationTemplateRepository_List_Empty_ReturnsNil(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	templates, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(templates) != 0 {
		t.Errorf("List: got %d templates, want 0", len(templates))
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestNotificationTemplateRepository_Delete_RemovesTemplate(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")

	if err := repo.Delete(context.Background(), "payment.confirmed", "en"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	got, err := repo.Get(context.Background(), "payment.confirmed", "en")
	if err != nil {
		t.Fatalf("Get after Delete: %v", err)
	}
	if got != nil {
		t.Error("expected nil after Delete")
	}
}

func TestNotificationTemplateRepository_Delete_NonExistent_NoError(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	if err := repo.Delete(context.Background(), "nonexistent.event", "en"); err != nil {
		t.Fatalf("Delete non-existent: unexpected error: %v", err)
	}
}

// ── ListHistory ───────────────────────────────────────────────────────────────

func TestNotificationTemplateRepository_ListHistory_NoHistoryOnFirstInsert(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")

	// BEFORE UPDATE trigger fires only on updates, not on inserts.
	entries, err := repo.ListHistory(context.Background(), "payment.confirmed", "en", 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ListHistory: got %d entries, want 0 after first insert", len(entries))
	}
}

func TestNotificationTemplateRepository_ListHistory_AfterUpdate_Archives(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")

	if err := repo.Upsert(context.Background(), &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated title",
		BodyTmpl:  "Updated body",
	}); err != nil {
		t.Fatalf("Upsert (update): %v", err)
	}

	entries, err := repo.ListHistory(context.Background(), "payment.confirmed", "en", 10)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("ListHistory: got %d entries, want 1", len(entries))
	}
	if entries[0].TitleTmpl != "Hello" {
		t.Errorf("archived TitleTmpl: got %q, want original 'Hello'", entries[0].TitleTmpl)
	}
}

func TestNotificationTemplateRepository_ListHistory_RespectsLimit(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	for i := range 3 {
		if err := repo.Upsert(context.Background(), &domain.NotificationTemplate{
			EventType: "payment.confirmed",
			Locale:    "en",
			TitleTmpl: fmt.Sprintf("Title v%d", i+2),
			BodyTmpl:  "body",
		}); err != nil {
			t.Fatalf("Upsert %d: %v", i, err)
		}
	}

	entries, err := repo.ListHistory(context.Background(), "payment.confirmed", "en", 2)
	if err != nil {
		t.Fatalf("ListHistory: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("ListHistory limit=2: got %d entries, want 2", len(entries))
	}
}

func TestNotificationTemplateRepository_ListHistory_InvalidLimit_UsesDefault(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	_ = repo.Upsert(context.Background(), &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated",
		BodyTmpl:  "body",
	})

	// limit=0 is invalid; implementation clamps it to 20.
	entries, err := repo.ListHistory(context.Background(), "payment.confirmed", "en", 0)
	if err != nil {
		t.Fatalf("ListHistory (limit=0): %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 archived entry, got %d", len(entries))
	}
}

func TestNotificationTemplateRepository_ListHistory_CancelledContext_ReturnsError(t *testing.T) {
	cleanNotifTemplates(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)
	_, err := repo.ListHistory(ctx, "payment.confirmed", "en", 10)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestNotificationTemplateRepository_ListHistory_OtherPairNotIncluded(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	seedNotifTemplate(t, "payment.confirmed", "es")

	// Update the "en" template to generate history.
	_ = repo.Upsert(context.Background(), &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated en",
		BodyTmpl:  "body",
	})

	// History for "es" must be empty.
	entries, err := repo.ListHistory(context.Background(), "payment.confirmed", "es", 10)
	if err != nil {
		t.Fatalf("ListHistory (es): %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("ListHistory (es): got %d; want 0 (different locale)", len(entries))
	}
}

// ── GetHistoryEntry ───────────────────────────────────────────────────────────

func TestNotificationTemplateRepository_GetHistoryEntry_ReturnsEntry(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	_ = repo.Upsert(context.Background(), &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated",
		BodyTmpl:  "body",
	})

	entries, _ := repo.ListHistory(context.Background(), "payment.confirmed", "en", 1)
	if len(entries) == 0 {
		t.Fatal("expected at least one history entry")
	}

	entry, err := repo.GetHistoryEntry(context.Background(), entries[0].ID, "payment.confirmed", "en")
	if err != nil {
		t.Fatalf("GetHistoryEntry: %v", err)
	}
	if entry == nil || entry.ID != entries[0].ID {
		t.Errorf("GetHistoryEntry: returned wrong or nil entry")
	}
}

func TestNotificationTemplateRepository_GetHistoryEntry_WrongEventType_ReturnsNotFound(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	seedNotifTemplate(t, "payment.confirmed", "en")
	_ = repo.Upsert(context.Background(), &domain.NotificationTemplate{
		EventType: "payment.confirmed",
		Locale:    "en",
		TitleTmpl: "Updated",
		BodyTmpl:  "body",
	})

	entries, _ := repo.ListHistory(context.Background(), "payment.confirmed", "en", 1)
	if len(entries) == 0 {
		t.Fatal("expected history entry")
	}

	_, err := repo.GetHistoryEntry(context.Background(), entries[0].ID, "payment.failed", "en")
	if !isNotFound(err) {
		t.Errorf("expected not-found for wrong event_type, got %v", err)
	}
}

func TestNotificationTemplateRepository_GetHistoryEntry_NotFound_ReturnsNotFound(t *testing.T) {
	cleanNotifTemplates(t)
	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)

	_, err := repo.GetHistoryEntry(context.Background(), 99999, "payment.confirmed", "en")
	if !isNotFound(err) {
		t.Errorf("expected not-found for missing entry, got %v", err)
	}
}

func TestNotificationTemplateRepository_GetHistoryEntry_CancelledContext_ReturnsError(t *testing.T) {
	cleanNotifTemplates(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := repository.NewPostgresNotificationTemplateRepository(testDB, time.Minute)
	_, err := repo.GetHistoryEntry(ctx, 1, "payment.confirmed", "en")
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
