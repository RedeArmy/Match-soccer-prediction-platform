package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── UserNotificationRepository ────────────────────────────────────────────────

func TestUserNotificationRepository_Create_PopulatesIDAndCreatedAt(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{
		UserID:    u.ID,
		EventType: "payment_confirmed",
		Title:     "Payment received",
		Body:      "Your payment of $100 has been received.",
	}
	inserted, err := repo.Create(context.Background(), n)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !inserted {
		t.Error("expected inserted=true for first create")
	}
	if n.ID == 0 {
		t.Error(msgNonZeroID)
	}
	if n.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt")
	}
}

func TestUserNotificationRepository_Create_WithMetadata(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{
		UserID:    u.ID,
		EventType: "payment_confirmed",
		Title:     "With meta",
		Body:      "body",
		Metadata:  map[string]any{"amount": 500, "currency": "MXN"},
	}
	inserted, err := repo.Create(context.Background(), n)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !inserted {
		t.Error("expected inserted=true")
	}
}

func TestUserNotificationRepository_Create_WithActionURL(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{
		UserID:    u.ID,
		EventType: "payment_confirmed",
		Title:     "With URL",
		Body:      "body",
		ActionURL: "/payments/42",
	}
	inserted, err := repo.Create(context.Background(), n)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if !inserted {
		t.Error("expected inserted=true")
	}
}

func TestUserNotificationRepository_Create_IdempotencyKey_DuplicateReturnsNotInserted(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{
		UserID:         u.ID,
		EventType:      "payment_confirmed",
		Title:          "Idempotent",
		Body:           "body",
		IdempotencyKey: "unique-key-001",
	}
	if _, err := repo.Create(context.Background(), n); err != nil {
		t.Fatalf("first create: %v", err)
	}

	n2 := &domain.UserNotification{
		UserID:         u.ID,
		EventType:      "payment_confirmed",
		Title:          "Idempotent duplicate",
		Body:           "body",
		IdempotencyKey: "unique-key-001",
	}
	inserted, err := repo.Create(context.Background(), n2)
	if err != nil {
		t.Fatalf("duplicate create: %v", err)
	}
	if inserted {
		t.Error("expected inserted=false for duplicate idempotency key")
	}
}

func TestUserNotificationRepository_List_ReturnsUserNotifications(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	other := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	for range 3 {
		_, _ = repo.Create(context.Background(), &domain.UserNotification{
			UserID: u.ID, EventType: "ev", Title: "t", Body: "b",
		})
	}
	_, _ = repo.Create(context.Background(), &domain.UserNotification{
		UserID: other.ID, EventType: "ev", Title: "t", Body: "b",
	})

	got, err := repo.List(context.Background(), u.ID, 100, 0, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 3 {
		t.Errorf("expected 3 notifications, got %d", len(got))
	}
}

func TestUserNotificationRepository_List_UnreadOnly(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n1 := &domain.UserNotification{UserID: u.ID, EventType: "ev", Title: "t1", Body: "b"}
	n2 := &domain.UserNotification{UserID: u.ID, EventType: "ev", Title: "t2", Body: "b"}
	_, _ = repo.Create(context.Background(), n1)
	_, _ = repo.Create(context.Background(), n2)

	// Mark n1 as read.
	if err := repo.MarkRead(context.Background(), n1.ID, u.ID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}

	got, err := repo.List(context.Background(), u.ID, 100, 0, true)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 unread notification, got %d", len(got))
	}
	if got[0].ID != n2.ID {
		t.Errorf("expected unread notification ID %d, got %d", n2.ID, got[0].ID)
	}
}

func TestUserNotificationRepository_List_Pagination(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	for range 5 {
		_, _ = repo.Create(context.Background(), &domain.UserNotification{
			UserID: u.ID, EventType: "ev", Title: "t", Body: "b",
		})
	}

	page1, err := repo.List(context.Background(), u.ID, 3, 0, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page1) != 3 {
		t.Errorf("expected 3 on page 1, got %d", len(page1))
	}

	page2, err := repo.List(context.Background(), u.ID, 3, 3, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(page2) != 2 {
		t.Errorf("expected 2 on page 2, got %d", len(page2))
	}
}

func TestUserNotificationRepository_List_Empty(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	got, err := repo.List(context.Background(), u.ID, 100, 0, false)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(got))
	}
}

func TestUserNotificationRepository_CountUnread(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	for range 4 {
		_, _ = repo.Create(context.Background(), &domain.UserNotification{
			UserID: u.ID, EventType: "ev", Title: "t", Body: "b",
		})
	}

	n := &domain.UserNotification{UserID: u.ID, EventType: "ev", Title: "read-me", Body: "b"}
	_, _ = repo.Create(context.Background(), n)
	_ = repo.MarkRead(context.Background(), n.ID, u.ID)

	count, err := repo.CountUnread(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 4 {
		t.Errorf("expected 4 unread, got %d", count)
	}
}

func TestUserNotificationRepository_CountUnread_Zero(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	count, err := repo.CountUnread(context.Background(), u.ID)
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestUserNotificationRepository_MarkRead_SetsReadAt(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{UserID: u.ID, EventType: "ev", Title: "t", Body: "b"}
	_, _ = repo.Create(context.Background(), n)

	if err := repo.MarkRead(context.Background(), n.ID, u.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	rows, _ := repo.List(context.Background(), u.ID, 100, 0, false)
	if len(rows) != 1 || rows[0].ReadAt == nil {
		t.Error("expected notification to have ReadAt set")
	}
}

func TestUserNotificationRepository_MarkRead_NotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	err := repo.MarkRead(context.Background(), 99999, u.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestUserNotificationRepository_MarkRead_AlreadyRead_ReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	n := &domain.UserNotification{UserID: u.ID, EventType: "ev", Title: "t", Body: "b"}
	_, _ = repo.Create(context.Background(), n)
	_ = repo.MarkRead(context.Background(), n.ID, u.ID)

	err := repo.MarkRead(context.Background(), n.ID, u.ID)
	if !isNotFound(err) {
		t.Errorf("expected not-found for already-read notification, got %v", err)
	}
}

func TestUserNotificationRepository_MarkAllRead(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	for range 3 {
		_, _ = repo.Create(context.Background(), &domain.UserNotification{
			UserID: u.ID, EventType: "ev", Title: "t", Body: "b",
		})
	}

	if err := repo.MarkAllRead(context.Background(), u.ID); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	count, _ := repo.CountUnread(context.Background(), u.ID)
	if count != 0 {
		t.Errorf("expected 0 unread after MarkAllRead, got %d", count)
	}
}

func TestUserNotificationRepository_MarkAllRead_NoRows_NoError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	if err := repo.MarkAllRead(context.Background(), u.ID); err != nil {
		t.Fatalf("expected no error for empty MarkAllRead, got %v", err)
	}
}

// ── Cancelled-context error paths ─────────────────────────────────────────────

func TestUserNotificationRepository_List_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.List(ctx, u.ID, 100, 0, false)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestUserNotificationRepository_CountUnread_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := repo.CountUnread(ctx, u.ID)
	if err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}

func TestUserNotificationRepository_MarkAllRead_CancelledContext_ReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresUserNotificationRepository(testDB)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := repo.MarkAllRead(ctx, u.ID); err == nil {
		t.Error("expected error for cancelled context, got nil")
	}
}
