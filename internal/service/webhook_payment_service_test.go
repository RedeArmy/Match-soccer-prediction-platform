package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type webhookLedgerRepoStub struct {
	creditErr error
}

func (r *webhookLedgerRepoStub) Credit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return r.creditErr
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

func newWebhookPaymentSvc(ledger *webhookLedgerRepoStub) WebhookPaymentService {
	return NewWebhookPaymentService(ledger, &noopAuditLogger{}, zap.NewNop())
}

// ── CreditFromRecurrente ──────────────────────────────────────────────────────

func TestWebhookPaymentService_CreditFromRecurrente_HappyPath(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromRecurrente(context.Background(), 5, 0, "GTQ", "REF-001")
	if err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_NegativeAmountReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromRecurrente(context.Background(), 5, -100, "GTQ", "REF-001")
	if err == nil {
		t.Fatal("expected validation error for negative amount, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_EmptyReferenceReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromRecurrente(context.Background(), 5, 1000, "GTQ", "")
	if err == nil {
		t.Fatal("expected validation error for empty reference, got nil")
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_RepoErrorPropagates(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{creditErr: errors.New("db error")})

	err := svc.CreditFromRecurrente(context.Background(), 5, 1000, "GTQ", "REF-X")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── CreditFromPayPal ──────────────────────────────────────────────────────────

func TestWebhookPaymentService_CreditFromPayPal_HappyPath(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromPayPal(context.Background(), 7, 2500, "USD", "CAPTURE-ABC")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_CreditFromPayPal_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromPayPal(context.Background(), 7, 0, "USD", "CAPTURE-ABC")
	if err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestWebhookPaymentService_CreditFromPayPal_EmptyReferenceReturnsValidation(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{})

	err := svc.CreditFromPayPal(context.Background(), 7, 1000, "USD", "")
	if err == nil {
		t.Fatal("expected validation error for empty reference, got nil")
	}
}

func TestWebhookPaymentService_CreditFromPayPal_RepoErrorPropagates(t *testing.T) {
	svc := newWebhookPaymentSvc(&webhookLedgerRepoStub{creditErr: errors.New("conflict")})

	err := svc.CreditFromPayPal(context.Background(), 7, 5000, "USD", "CAPTURE-Y")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── ledger kind routing ────────────────────────────────────────────────────────

// capturedKindLedgerRepo records the kind passed to Credit to verify routing.
type capturedKindLedgerRepo struct {
	capturedKind domain.BalanceLedgerKind
}

func (r *capturedKindLedgerRepo) Credit(_ context.Context, _ int, _ int, kind domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	r.capturedKind = kind
	return nil
}
func (r *capturedKindLedgerRepo) Debit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (r *capturedKindLedgerRepo) Reserve(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *capturedKindLedgerRepo) ReleaseReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *capturedKindLedgerRepo) CommitReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (r *capturedKindLedgerRepo) ListByUser(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return nil, nil
}

func TestWebhookPaymentService_CreditFromRecurrente_UsesRecurrenteKind(t *testing.T) {
	repo := &capturedKindLedgerRepo{}
	svc := NewWebhookPaymentService(repo, &noopAuditLogger{}, zap.NewNop())

	_ = svc.CreditFromRecurrente(context.Background(), 1, 1000, "GTQ", "REF")
	if repo.capturedKind != domain.LedgerKindWebhookRecurrente {
		t.Errorf("kind: got %q, want %q", repo.capturedKind, domain.LedgerKindWebhookRecurrente)
	}
}

func TestWebhookPaymentService_CreditFromPayPal_UsesPayPalKind(t *testing.T) {
	repo := &capturedKindLedgerRepo{}
	svc := NewWebhookPaymentService(repo, &noopAuditLogger{}, zap.NewNop())

	_ = svc.CreditFromPayPal(context.Background(), 1, 1000, "USD", "CAP")
	if repo.capturedKind != domain.LedgerKindWebhookPayPal {
		t.Errorf("kind: got %q, want %q", repo.capturedKind, domain.LedgerKindWebhookPayPal)
	}
}
