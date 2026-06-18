package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
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

func (r *stubIntentRepo) GetByToken(_ context.Context, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) GetByID(_ context.Context, _ int64) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) CaptureAndCredit(_ context.Context, _, _ string, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) MarkCapturedByToken(_ context.Context, _ string) error { return nil }
func (r *stubIntentRepo) SetComprobante(_ context.Context, _ int64, _, _ string, _ int) error {
	return nil
}
func (r *stubIntentRepo) AdminCreditExpired(_ context.Context, _ int64, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) AdminReject(_ context.Context, _ int64, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) ListForAdmin(_ context.Context, _ repository.PaymentIntentFilters, _ repository.Pagination) ([]*domain.PaymentIntent, int, error) {
	return nil, 0, nil
}
func (r *stubIntentRepo) ListByUserPending(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) ListAllByUser(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) RequestComprobante(_ context.Context, _ int64, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepo) SubmitForReview(_ context.Context, _ int64, _ int, _, _ *string, _ *int, _ string) (*domain.PaymentIntent, error) {
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

func TestPaymentIntentService_Create_AmountAtMaxAllowed(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	_, err := svc.Create(context.Background(), 1, domain.DefaultPaymentIntentMaxCents, "GTQ")
	if err != nil {
		t.Fatalf("amount at max should be accepted, got %v", err)
	}
}

func TestPaymentIntentService_Create_AmountOverMaxReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})

	_, err := svc.Create(context.Background(), 1, domain.DefaultPaymentIntentMaxCents+1, "GTQ")
	if err == nil {
		t.Fatal("expected validation error for amount above max, got nil")
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

// ── CreateForRecurrente ───────────────────────────────────────────────────

func TestPaymentIntentService_CreateForRecurrente_HappyPath(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	intent, err := svc.CreateForRecurrente(context.Background(), 5, 3000, "GTQ", "ref-abc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Token != "ref-abc" {
		t.Errorf("token: got %q, want ref-abc", intent.Token)
	}
	if intent.Provider != "recurrente" {
		t.Errorf("provider: got %q, want recurrente", intent.Provider)
	}
	if intent.Status != domain.PaymentIntentPending {
		t.Errorf("status: got %q, want pending", intent.Status)
	}
}

func TestPaymentIntentService_CreateForRecurrente_ZeroAmountReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	_, err := svc.CreateForRecurrente(context.Background(), 1, 0, "GTQ", "ref")
	if err == nil {
		t.Fatal("expected validation error for zero amount, got nil")
	}
}

func TestPaymentIntentService_CreateForRecurrente_NegativeAmountReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	_, err := svc.CreateForRecurrente(context.Background(), 1, -1, "GTQ", "ref")
	if err == nil {
		t.Fatal("expected validation error for negative amount, got nil")
	}
}

func TestPaymentIntentService_CreateForRecurrente_EmptyReferenceReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	_, err := svc.CreateForRecurrente(context.Background(), 1, 1000, "GTQ", "")
	if err == nil {
		t.Fatal("expected validation error for empty reference, got nil")
	}
}

func TestPaymentIntentService_CreateForRecurrente_DefaultCurrency(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	intent, err := svc.CreateForRecurrente(context.Background(), 1, 1000, "", "ref-xyz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if intent.Currency != "GTQ" {
		t.Errorf("expected default currency GTQ, got %q", intent.Currency)
	}
}

func TestPaymentIntentService_CreateForRecurrente_RepoErrorPropagates(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{createErr: errors.New("db error")})
	_, err := svc.CreateForRecurrente(context.Background(), 1, 1000, "GTQ", "ref")
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── ListMyPending ─────────────────────────────────────────────────────────

func TestPaymentIntentService_ListMyPending_ReturnsEmpty(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	items, err := svc.ListMyPending(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil slice, got %v", items)
	}
}

// ── ListMyAll ─────────────────────────────────────────────────────────────

func TestPaymentIntentService_ListMyAll_ReturnsEmpty(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	items, err := svc.ListMyAll(context.Background(), 7)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil slice, got %v", items)
	}
}

// ── SetComprobanteByToken ─────────────────────────────────────────────────

func TestPaymentIntentService_SetComprobanteByToken_TokenNotFound_Returns404(t *testing.T) {
	// stubIntentRepo.GetByToken returns (nil, nil) by default.
	svc := newIntentSvc(&stubIntentRepo{})
	err := svc.SetComprobanteByToken(context.Background(), "missing-tok", "key", "image/jpeg", 1024)
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestPaymentIntentService_SetComprobanteByToken_HappyPath(t *testing.T) {
	repo := &stubIntentRepoWithGetByToken{
		intent: &domain.PaymentIntent{ID: 9, Token: "tok-ok", UserID: 3},
	}
	svc := NewPaymentIntentService(repo, &noopSystemParamService{}, zap.NewNop())
	if err := svc.SetComprobanteByToken(context.Background(), "tok-ok", "some/key.jpg", "image/jpeg", 2048); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// ── ResubmitForReview ─────────────────────────────────────────────────────

func TestPaymentIntentService_ResubmitForReview_EmptyNotes_ReturnsValidation(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	_, err := svc.ResubmitForReview(context.Background(), 1, "tok", nil, nil, nil, "")
	if err == nil {
		t.Fatal("expected validation error for empty notes, got nil")
	}
}

func TestPaymentIntentService_ResubmitForReview_TokenNotFound_Returns404(t *testing.T) {
	svc := newIntentSvc(&stubIntentRepo{})
	_, err := svc.ResubmitForReview(context.Background(), 1, "missing", nil, nil, nil, "dispute notes")
	if err == nil {
		t.Fatal("expected not-found error, got nil")
	}
}

func TestPaymentIntentService_ResubmitForReview_WrongOwner_Returns404(t *testing.T) {
	repo := &stubIntentRepoWithGetByToken{
		intent: &domain.PaymentIntent{ID: 5, Token: "tok-x", UserID: 99, Status: domain.PaymentIntentRejected},
	}
	svc := NewPaymentIntentService(repo, &noopSystemParamService{}, zap.NewNop())
	_, err := svc.ResubmitForReview(context.Background(), 1, "tok-x", nil, nil, nil, "dispute")
	if err == nil {
		t.Fatal("expected not-found for wrong owner, got nil")
	}
}

func TestPaymentIntentService_ResubmitForReview_WrongStatus_ReturnsValidation(t *testing.T) {
	repo := &stubIntentRepoWithGetByToken{
		intent: &domain.PaymentIntent{ID: 3, Token: "tok-p", UserID: 1, Status: domain.PaymentIntentPending},
	}
	svc := NewPaymentIntentService(repo, &noopSystemParamService{}, zap.NewNop())
	_, err := svc.ResubmitForReview(context.Background(), 1, "tok-p", nil, nil, nil, "note")
	if err == nil {
		t.Fatal("expected validation error for non-rejected intent, got nil")
	}
}

func TestPaymentIntentService_ResubmitForReview_HappyPath(t *testing.T) {
	updated := &domain.PaymentIntent{ID: 7, Token: "tok-r", UserID: 2, Status: domain.PaymentIntentUnderReview}
	repo := &stubIntentRepoWithGetByToken{
		intent:         &domain.PaymentIntent{ID: 7, Token: "tok-r", UserID: 2, Status: domain.PaymentIntentRejected},
		submitResponse: updated,
	}
	svc := NewPaymentIntentService(repo, &noopSystemParamService{}, zap.NewNop())
	result, err := svc.ResubmitForReview(context.Background(), 2, "tok-r", nil, nil, nil, "here's why")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Status != domain.PaymentIntentUnderReview {
		t.Errorf("status: got %q, want under_review", result.Status)
	}
}

// ── extended stub supporting GetByToken responses ─────────────────────────

type stubIntentRepoWithGetByToken struct {
	intent         *domain.PaymentIntent
	submitResponse *domain.PaymentIntent
}

func (r *stubIntentRepoWithGetByToken) Create(_ context.Context, intent *domain.PaymentIntent) error {
	intent.ID = 1
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = time.Now()
	return nil
}
func (r *stubIntentRepoWithGetByToken) GetByToken(_ context.Context, _ string) (*domain.PaymentIntent, error) {
	return r.intent, nil
}
func (r *stubIntentRepoWithGetByToken) GetByID(_ context.Context, _ int64) (*domain.PaymentIntent, error) {
	return r.intent, nil
}
func (r *stubIntentRepoWithGetByToken) CaptureAndCredit(_ context.Context, _, _ string, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) MarkCapturedByToken(_ context.Context, _ string) error {
	return nil
}
func (r *stubIntentRepoWithGetByToken) SetComprobante(_ context.Context, _ int64, _, _ string, _ int) error {
	return nil
}
func (r *stubIntentRepoWithGetByToken) AdminCreditExpired(_ context.Context, _ int64, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) AdminReject(_ context.Context, _ int64, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) ListForAdmin(_ context.Context, _ repository.PaymentIntentFilters, _ repository.Pagination) ([]*domain.PaymentIntent, int, error) {
	return nil, 0, nil
}
func (r *stubIntentRepoWithGetByToken) ListByUserPending(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) ListAllByUser(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) RequestComprobante(_ context.Context, _ int64, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *stubIntentRepoWithGetByToken) SubmitForReview(_ context.Context, _ int64, _ int, _, _ *string, _ *int, _ string) (*domain.PaymentIntent, error) {
	return r.submitResponse, nil
}
