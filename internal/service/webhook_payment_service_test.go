package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type webhookLedgerRepoStub struct {
	capturedKind      domain.BalanceLedgerKind
	capturedReference string
	creditErr         error
	// skipCredit simulates a duplicate reference: CreditIdempotent returns (false, nil).
	skipCredit bool
}

func (r *webhookLedgerRepoStub) Credit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (r *webhookLedgerRepoStub) CreditIdempotent(_ context.Context, _ int, _ int, kind domain.BalanceLedgerKind, reference string) (bool, error) {
	r.capturedKind = kind
	r.capturedReference = reference
	if r.creditErr != nil {
		return false, r.creditErr
	}
	return !r.skipCredit, nil
}
func (r *webhookLedgerRepoStub) Debit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (r *webhookLedgerRepoStub) Reserve(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *webhookLedgerRepoStub) ReleaseReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *webhookLedgerRepoStub) CommitReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *webhookLedgerRepoStub) ListByUser(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return nil, nil
}
func (r *webhookLedgerRepoStub) SumTransactionsByUserAndPeriod(_ context.Context, _ int, _ []domain.BalanceLedgerKind, _ time.Time) (int64, error) {
	return 0, nil
}

// webhookIntentRepoStub is the PaymentIntentRepository stub for service tests.
type webhookIntentRepoStub struct {
	intent     *domain.PaymentIntent
	captureErr error
}

func (r *webhookIntentRepoStub) Create(_ context.Context, intent *domain.PaymentIntent) error {
	intent.ID = 99
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = time.Now()
	return nil
}

func (r *webhookIntentRepoStub) CaptureAndCredit(_ context.Context, _, _ string) (*domain.PaymentIntent, error) {
	return r.intent, r.captureErr
}

func newWebhookPaymentSvc(ledger *webhookLedgerRepoStub, intent *webhookIntentRepoStub) WebhookPaymentService {
	if intent == nil {
		intent = &webhookIntentRepoStub{}
	}
	return NewWebhookPaymentService(ledger, intent, &noopAuditLogger{}, zap.NewNop())
}

// ── CreditFromRecurrente ──────────────────────────────────────────────────────

func TestWebhookPaymentService_CreditFromRecurrente_HappyPath(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-001"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 0, "GTQ", "REF"); err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_NegativeAmountReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 5, -1, "GTQ", "REF"); err == nil {
		t.Fatal("expected validation error for negative amount, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_EmptyReferenceReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 1000, "GTQ", ""); err == nil {
		t.Fatal("expected validation error for empty reference, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_RepoErrorPropagates(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{creditErr: errors.New("db error")}, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 1000, "GTQ", "REF-X"); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_UsesRecurrenteKind(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	svc := newWebhookPaymentSvc(ledger, nil)
	_ = svc.CreditFromRecurrente(context.Background(), 1, 1000, "GTQ", "REF")
	if ledger.capturedKind != domain.LedgerKindWebhookRecurrente {
		t.Errorf("kind: got %q, want %q", ledger.capturedKind, domain.LedgerKindWebhookRecurrente)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_PassesReferenceToRepo(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	svc := newWebhookPaymentSvc(ledger, nil)
	_ = svc.CreditFromRecurrente(context.Background(), 1, 1000, "GTQ", "TXN-XYZ")
	// Reference is scoped with the provider kind to prevent cross-provider collision.
	want := "webhook_recurrente:TXN-XYZ"
	if ledger.capturedReference != want {
		t.Errorf("reference: got %q, want %q", ledger.capturedReference, want)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_DuplicateReferenceIsNoop(t *testing.T) {
	ledger := &webhookLedgerRepoStub{skipCredit: true}
	svc := newWebhookPaymentSvc(ledger, nil)
	if err := svc.CreditFromRecurrente(context.Background(), 1, 1000, "GTQ", "TXN-DUP"); err != nil {
		t.Fatalf("duplicate reference must return nil, got %v", err)
	}
}

// ── ResolveAndCreditPayPalIntent ──────────────────────────────────────────────

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_HappyPath(t *testing.T) {
	intent := &domain.PaymentIntent{ID: 1, UserID: 7, AmountCents: 2500, Currency: "USD"}
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, &webhookIntentRepoStub{intent: intent})

	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-ABC", 2500); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_EmptyTokenReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "", "CAP", 0); err == nil {
		t.Fatal("expected validation error for empty token, got nil")
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_EmptyCaptureIDReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, nil)
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "", 0); err == nil {
		t.Fatal("expected validation error for empty captureID, got nil")
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_IdempotentReplayReturnsNil(t *testing.T) {
	intent := &domain.PaymentIntent{ID: 1, UserID: 7, AmountCents: 2500, Currency: "USD"}
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, &webhookIntentRepoStub{
		intent:     intent,
		captureErr: repository.ErrPaymentIntentAlreadyCaptured,
	})
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP", 0); err != nil {
		t.Fatalf("idempotent replay must return nil, got %v", err)
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_NotFoundPropagates(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, &webhookIntentRepoStub{
		captureErr: apperrors.NotFound("payment intent not found"),
	})
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP", 0); err == nil {
		t.Fatal("expected NotFound error to propagate, got nil")
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_ConflictPropagates(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, &webhookIntentRepoStub{
		captureErr: apperrors.Conflict("already captured by different transaction"),
	})
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP", 0); err == nil {
		t.Fatal("expected Conflict error to propagate, got nil")
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_AmountMismatchDoesNotError(t *testing.T) {
	// Webhook declares 2000 cents but intent has 2500 — mismatch is warned but
	// must not block the credit (server-authoritative amount wins).
	intent := &domain.PaymentIntent{ID: 1, UserID: 7, AmountCents: 2500, Currency: "USD"}
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{}, &webhookIntentRepoStub{intent: intent})
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-MISMATCH", 2000); err != nil {
		t.Fatalf("amount mismatch must not return an error, got %v", err)
	}
}
