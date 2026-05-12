package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ── stub ──────────────────────────────────────────────────────────────────────

type stubIntentRepo struct {
	createErr error
}

func (r *stubIntentRepo) Create(_ context.Context, intent *domain.PaymentIntent) error {
	if r.createErr != nil {
		return r.createErr
	}
	intent.ID = 1
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = time.Now()
	return nil
}

func (r *stubIntentRepo) CaptureAndCredit(_ context.Context, _, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}

func newIntentSvc(repo *stubIntentRepo) PaymentIntentCreator {
	return NewPaymentIntentService(repo, &noopSystemParamService{}, zap.NewNop())
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestPaymentIntentService_Create_HappyPath(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	intent, err := svc.Create(context.Background(), 10, 5000, "GTQ")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent == nil {
		t.Fatal("expected intent, got nil")
	}
	if len(intent.Token) != 64 {
		t.Errorf("token length: got %d, want 64", len(intent.Token))
	}
	if intent.UserID != 10 {
		t.Errorf("user_id: got %d, want 10", intent.UserID)
	}
	if intent.AmountCents != 5000 {
		t.Errorf("amount_cents: got %d, want 5000", intent.AmountCents)
	}
	if intent.Currency != "GTQ" {
		t.Errorf("currency: got %q, want GTQ", intent.Currency)
	}
	if intent.Status != domain.PaymentIntentPending {
		t.Errorf("status: got %q, want %q", intent.Status, domain.PaymentIntentPending)
	}
	if intent.ExpiresAt.IsZero() {
		t.Error("expires_at must not be zero")
	}
}

func TestPaymentIntentService_Create_DefaultCurrency(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	intent, err := svc.Create(context.Background(), 1, 1000, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Currency != "GTQ" {
		t.Errorf("expected default currency GTQ, got %q", intent.Currency)
	}
}

func TestPaymentIntentService_Create_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	_, err := svc.Create(context.Background(), 1, 0, "GTQ")
	if err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestPaymentIntentService_Create_NegativeAmountReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	_, err := svc.Create(context.Background(), 1, -100, "GTQ")
	if err == nil {
		t.Fatal("expected validation error for negative amount, got nil")
	}
}

func TestPaymentIntentService_Create_RepoErrorPropagates(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{createErr: errors.New("db error")})

	_, err := svc.Create(context.Background(), 1, 1000, "GTQ")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestPaymentIntentService_Create_TokensAreUnique(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	a, _ := svc.Create(context.Background(), 1, 1000, "GTQ")
	b, _ := svc.Create(context.Background(), 1, 1000, "GTQ")
	if a.Token == b.Token {
		t.Error("two consecutive tokens must not be equal")
	}
}

func TestPaymentIntentService_Create_ExpiresInFuture(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	intent, _ := svc.Create(context.Background(), 1, 1000, "GTQ")
	if !intent.ExpiresAt.After(time.Now()) {
		t.Error("expires_at must be in the future")
	}
}
