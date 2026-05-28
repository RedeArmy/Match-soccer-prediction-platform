package repository_test

import (
	"context"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── AdminNotificationLogRepository ───────────────────────────────────────────

func TestAdminNotificationLogRepository_Create_SetsIDAndCreatedAt(t *testing.T) {
	// admin_notification_log is append-only and not referenced by cleanTables;
	// rows accumulate across tests in this package run, which is acceptable
	// because each test asserts only on the entry it inserts.
	repo := repository.NewPostgresAdminNotificationLogRepository(testDB)

	entry := &domain.AdminNotificationLog{
		EventType:   "kyc_approved",
		Recipients:  []string{"ops@example.com"},
		Subject:     "KYC approved for user 42",
		Status:      domain.AdminNotifStatusSent,
		ResendMsgID: "resend-abc123",
	}

	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}
	if entry.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt after insert")
	}
}

func TestAdminNotificationLogRepository_Create_FailedStatus_NoResendID(t *testing.T) {
	repo := repository.NewPostgresAdminNotificationLogRepository(testDB)

	entry := &domain.AdminNotificationLog{
		EventType:   "payment_error",
		Recipients:  []string{"admin@example.com"},
		Subject:     "Payment processing error",
		Status:      domain.AdminNotifStatusFailed,
		ErrorDetail: "SMTP connection refused",
	}

	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create failed status: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}
}

func TestAdminNotificationLogRepository_Create_MultipleRecipients(t *testing.T) {
	repo := repository.NewPostgresAdminNotificationLogRepository(testDB)

	entry := &domain.AdminNotificationLog{
		EventType:  "sanctions_flag",
		Recipients: []string{"compliance@example.com", "legal@example.com", "ops@example.com"},
		Subject:    "Sanctions flag alert",
		Status:     domain.AdminNotifStatusSent,
	}

	if err := repo.Create(context.Background(), entry); err != nil {
		t.Fatalf("Create with multiple recipients: %v", err)
	}
	if entry.ID == 0 {
		t.Error("expected non-zero ID after insert")
	}

	// Confirm the persisted recipients match what was inserted.
	var recipients []string
	err := testDB.QueryRow(context.Background(),
		`SELECT recipients FROM admin_notification_log WHERE id = $1`, entry.ID,
	).Scan(&recipients)
	if err != nil {
		t.Fatalf("read recipients: %v", err)
	}
	if len(recipients) != 3 {
		t.Errorf("recipients: got %d, want 3", len(recipients))
	}
}
