package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// seedPaymentIntent inserts a pending payment intent for the given user.
func seedPaymentIntent(t *testing.T, userID, amountCents int) *domain.PaymentIntent {
	t.Helper()
	repo := repository.NewPostgresPaymentIntentRepository(testDB)
	intent := &domain.PaymentIntent{
		Token:       "tok_" + nextCode(),
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("seedPaymentIntent: %v", err)
	}
	return intent
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestPaymentIntentRepository_Create_PopulatesID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 5000)

	if intent.ID == 0 {
		t.Error(msgNonZeroID)
	}
}

func TestPaymentIntentRepository_Create_PopulatesTimestamps(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2000)

	if intent.CreatedAt.IsZero() {
		t.Error("created_at must not be zero")
	}
	if intent.UpdatedAt.IsZero() {
		t.Error("updated_at must not be zero")
	}
}

func TestPaymentIntentRepository_Create_DuplicateTokenReturnsError(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	intent := &domain.PaymentIntent{
		Token:       "duplicate-token",
		UserID:      u.ID,
		AmountCents: 1000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("first create: %v", err)
	}

	intent2 := &domain.PaymentIntent{
		Token:       "duplicate-token",
		UserID:      u.ID,
		AmountCents: 2000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	if err := repo.Create(context.Background(), intent2); err == nil {
		t.Error("expected error for duplicate token, got nil")
	}
}

// ── CaptureAndCredit ──────────────────────────────────────────────────────────

func TestPaymentIntentRepository_CaptureAndCredit_HappyPath(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 3000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	captured, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-001")
	if err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if captured == nil {
		t.Fatal("expected captured intent, got nil")
	}
	if captured.Status != domain.PaymentIntentCaptured {
		t.Errorf("status: got %q, want captured", captured.Status)
	}
	if captured.CaptureID == nil || *captured.CaptureID != "CAP-001" {
		t.Errorf("capture_id: got %v, want CAP-001", captured.CaptureID)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_CreditsUserBalance(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 4000)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-002"); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	userRepo := repository.NewPostgresUserRepository(testDB)
	bal, _, err := userRepo.GetBalance(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if bal != 4000 {
		t.Errorf("balance: got %d, want 4000", bal)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_IdempotentReplaySameCaptureID(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2500)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-DUP"); err != nil {
		t.Fatalf("first capture: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-DUP")
	if !errors.Is(err, repository.ErrPaymentIntentAlreadyCaptured) {
		t.Errorf("expected ErrPaymentIntentAlreadyCaptured, got %v", err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_DifferentCaptureIDReturnsConflict(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	intent := seedPaymentIntent(t, u.ID, 2500)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	if _, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-FIRST"); err != nil {
		t.Fatalf("first capture: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-SECOND")
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (conflict), got %T: %v", err, err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_TokenNotFoundReturnsNotFound(t *testing.T) {
	cleanTables(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	_, err := repo.CaptureAndCredit(context.Background(), "nonexistent-token", "CAP-999")
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (not found), got %T: %v", err, err)
	}
}

func TestPaymentIntentRepository_CaptureAndCredit_ExpiredIntentReturnsNotFound(t *testing.T) {
	cleanTables(t)
	u := seedUser(t)
	repo := repository.NewPostgresPaymentIntentRepository(testDB)

	// Insert an already-expired intent directly.
	intent := &domain.PaymentIntent{
		Token:       "tok_expired_" + nextCode(),
		UserID:      u.ID,
		AmountCents: 1000,
		Currency:    "GTQ",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(-time.Hour), // expired 1 hour ago
	}
	if err := repo.Create(context.Background(), intent); err != nil {
		t.Fatalf("create expired intent: %v", err)
	}

	_, err := repo.CaptureAndCredit(context.Background(), intent.Token, "CAP-EXP")
	if !errors.As(err, new(*apperrors.AppError)) {
		t.Errorf("expected AppError (not found/expired), got %T: %v", err, err)
	}
}
