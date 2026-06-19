package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
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
func (r *webhookLedgerRepoStub) CreditIdempotent(_ context.Context, _ int, _ int, kind domain.BalanceLedgerKind, reference string, _ string, _ int) (bool, error) {
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
func (r *webhookLedgerRepoStub) CountRows(_ context.Context) (int64, error) { return 0, nil }

// webhookIntentRepoStub is the PaymentIntentRepository stub for service tests.
type webhookIntentRepoStub struct {
	intent          *domain.PaymentIntent
	captureErr      error
	markCapturedErr error
}

func (r *webhookIntentRepoStub) Create(_ context.Context, intent *domain.PaymentIntent) error {
	intent.ID = 99
	intent.CreatedAt = time.Now()
	intent.UpdatedAt = time.Now()
	return nil
}

func (r *webhookIntentRepoStub) GetByToken(_ context.Context, _ string) (*domain.PaymentIntent, error) {
	return r.intent, nil
}
func (r *webhookIntentRepoStub) GetByID(_ context.Context, _ int64) (*domain.PaymentIntent, error) {
	return r.intent, nil
}
func (r *webhookIntentRepoStub) CaptureAndCredit(_ context.Context, _, _ string, _ int, _ string, _ int) (*domain.PaymentIntent, error) {
	return r.intent, r.captureErr
}
func (r *webhookIntentRepoStub) MarkCapturedByToken(_ context.Context, _ string) error {
	return r.markCapturedErr
}
func (r *webhookIntentRepoStub) SetComprobante(_ context.Context, _ int64, _, _ string, _ int) error {
	return nil
}
func (r *webhookIntentRepoStub) AdminCreditExpired(_ context.Context, _ int64, _, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) AdminReject(_ context.Context, _ int64, _ int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) ListForAdmin(_ context.Context, _ repository.PaymentIntentFilters, _ repository.Pagination) ([]*domain.PaymentIntent, int, error) {
	return nil, 0, nil
}
func (r *webhookIntentRepoStub) ListByUserPending(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) ListAllByUser(_ context.Context, _ int) ([]*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) RequestComprobante(_ context.Context, _ int64, _ int) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) SubmitForReview(_ context.Context, _ int64, _ int, _, _ *string, _ *int, _ string) (*domain.PaymentIntent, error) {
	return nil, nil
}
func (r *webhookIntentRepoStub) CancelByToken(_ context.Context, _ string, _ int) error {
	return nil
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

// ── KYC gate integration ──────────────────────────────────────────────────────

// webhookKYCGateStub is a configurable KYCGate stub for webhook payment tests.
type webhookKYCGateStub struct {
	depositErr         error
	velocityErr        error
	amlExceeded        bool
	cumulativeExceeded bool
}

func (g *webhookKYCGateStub) CheckWithdrawal(_ context.Context, _, _ int) error { return nil }
func (g *webhookKYCGateStub) CheckDeposit(_ context.Context, _, _ int) error    { return g.depositErr }
func (g *webhookKYCGateStub) CheckWinFreeze(_ context.Context, _, _ int) (bool, string, error) {
	return false, "", nil
}
func (g *webhookKYCGateStub) ExceedsAMLThreshold(_ context.Context, _ int) (bool, error) {
	return g.amlExceeded, nil
}
func (g *webhookKYCGateStub) ExceedsCumulativeAMLThreshold(_ context.Context, _, _ int) (bool, error) {
	return g.cumulativeExceeded, nil
}
func (g *webhookKYCGateStub) CheckDepositVelocity(_ context.Context, _, _ int) error {
	return g.velocityErr
}
func (g *webhookKYCGateStub) CheckWithdrawalVelocity(_ context.Context, _, _ int) error   { return nil }
func (g *webhookKYCGateStub) CheckIPSubmissionVelocity(_ context.Context, _ string) error { return nil }

// multiSpyAuditLogger accumulates every Log call so tests can assert on multiple events.
type multiSpyAuditLogger struct {
	actions []string
	meta    []map[string]any
}

func (s *multiSpyAuditLogger) Log(_ context.Context, _ *int, _ *domain.UserRole, action string, _ *string, _ *int, metadata map[string]any) {
	s.actions = append(s.actions, action)
	s.meta = append(s.meta, metadata)
}

func newWebhookPaymentSvcWithGate(ledger *webhookLedgerRepoStub, intent *webhookIntentRepoStub, gate *webhookKYCGateStub) WebhookPaymentService {
	if intent == nil {
		intent = &webhookIntentRepoStub{}
	}
	svc := NewWebhookPaymentService(ledger, intent, &noopAuditLogger{}, zap.NewNop())
	if gate != nil {
		svc.(*webhookPaymentService).SetKYCGate(gate)
	}
	return svc
}

func TestWebhookPaymentService_CreditFromRecurrente_KYCDepositBlocked_CreditNotApplied(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	gate := &webhookKYCGateStub{depositErr: errors.New("tier too low")}
	svc := newWebhookPaymentSvcWithGate(ledger, nil, gate)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-KYC"); err == nil {
		t.Fatal("expected KYC gate error, got nil")
	}
	if ledger.capturedKind != "" {
		t.Errorf("credit must NOT be applied when gate blocks, got kind=%q", ledger.capturedKind)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_KYCVelocityBlocked_CreditNotApplied(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	gate := &webhookKYCGateStub{velocityErr: errors.New("velocity exceeded")}
	svc := newWebhookPaymentSvcWithGate(ledger, nil, gate)
	if err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-VEL"); err == nil {
		t.Fatal("expected velocity gate error, got nil")
	}
	if ledger.capturedKind != "" {
		t.Errorf("credit must NOT be applied when velocity gate blocks, got kind=%q", ledger.capturedKind)
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_KYCDepositBlocked_CreditNotApplied(t *testing.T) {
	pending := &domain.PaymentIntent{
		ID: 1, UserID: 7, AmountCents: 5000,
		Status: domain.PaymentIntentPending,
	}
	intent := &webhookIntentRepoStub{intent: pending}
	gate := &webhookKYCGateStub{depositErr: errors.New("tier too low")}
	svc := newWebhookPaymentSvcWithGate(&webhookLedgerRepoStub{}, intent, gate)
	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-KYC", 5000); err == nil {
		t.Fatal("expected KYC gate error for PayPal, got nil")
	}
}

func newWebhookPaymentSvcWithGateAndAudit(ledger *webhookLedgerRepoStub, intent *webhookIntentRepoStub, gate *webhookKYCGateStub, audit *multiSpyAuditLogger) WebhookPaymentService {
	if intent == nil {
		intent = &webhookIntentRepoStub{}
	}
	svc := NewWebhookPaymentService(ledger, intent, audit, zap.NewNop())
	if gate != nil {
		svc.(*webhookPaymentService).SetKYCGate(gate)
	}
	return svc
}

// ── SetExchangeRateService / USD→GTQ conversion ───────────────────────────────

// fxSvcStub is a minimal ExchangeRateService stub for webhook payment tests.
type fxSvcStub struct {
	gtqCents int64
	convErr  error
}

func (f *fxSvcStub) RefreshRate(_ context.Context) (*domain.ExchangeRates, error) { return nil, nil }
func (f *fxSvcStub) GetCurrentRates(_ context.Context) (*domain.ExchangeRates, error) {
	return nil, nil
}
func (f *fxSvcStub) OverrideRate(_ context.Context, _ decimal.Decimal, _ string, _ int) (*domain.ExchangeRates, error) {
	return nil, nil
}
func (f *fxSvcStub) ConvertUSDToGTQ(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return f.gtqCents, decimal.Zero, f.convErr
}
func (f *fxSvcStub) ConvertGTQToUSD(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return 0, decimal.Zero, nil
}
func (f *fxSvcStub) WarmCache(_ context.Context) error { return nil }

func newWebhookPaymentSvcWithFX(ledger *webhookLedgerRepoStub, intent *webhookIntentRepoStub, fx ExchangeRateService) WebhookPaymentService {
	if intent == nil {
		intent = &webhookIntentRepoStub{}
	}
	svc := NewWebhookPaymentService(ledger, intent, &noopAuditLogger{}, zap.NewNop())
	if fx != nil {
		svc.(*webhookPaymentService).SetExchangeRateService(fx)
	}
	return svc
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_FxSvcNil_CreditsRawAmount(t *testing.T) {
	// When fxSvc is nil and the currency is USD, the raw intent amount is used.
	intent := &domain.PaymentIntent{
		ID: 1, UserID: 7, AmountCents: 2000,
		Currency: "USD", Status: domain.PaymentIntentPending,
	}
	intentRepo := &webhookIntentRepoStub{intent: intent}
	svc := newWebhookPaymentSvcWithFX(&webhookLedgerRepoStub{}, intentRepo, nil)

	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-FX-NIL", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_FxSvcSuccess_CreditsConvertedAmount(t *testing.T) {
	// When fxSvc converts successfully, the GTQ amount is credited.
	intent := &domain.PaymentIntent{
		ID: 2, UserID: 7, AmountCents: 1000,
		Currency: "USD", Status: domain.PaymentIntentPending,
	}
	intentRepo := &webhookIntentRepoStub{intent: intent}
	fx := &fxSvcStub{gtqCents: 7800}
	svc := newWebhookPaymentSvcWithFX(&webhookLedgerRepoStub{}, intentRepo, fx)

	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-FX-OK", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_FxSvcError_FallsBackToRawAmount(t *testing.T) {
	// When fxSvc returns an error, the service logs a warning and falls back to
	// the raw intent amount rather than blocking the credit.
	intent := &domain.PaymentIntent{
		ID: 3, UserID: 7, AmountCents: 500,
		Currency: "USD", Status: domain.PaymentIntentPending,
	}
	intentRepo := &webhookIntentRepoStub{intent: intent}
	fx := &fxSvcStub{convErr: errors.New("rate unavailable")}
	svc := newWebhookPaymentSvcWithFX(&webhookLedgerRepoStub{}, intentRepo, fx)

	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-FX-ERR", 0); err != nil {
		t.Fatalf("conversion failure must not block the credit, got: %v", err)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_CumulativeAML_AuditFlagged(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	gate := &webhookKYCGateStub{cumulativeExceeded: true}
	audit := &multiSpyAuditLogger{}
	svc := newWebhookPaymentSvcWithGateAndAudit(ledger, nil, gate, audit)

	if err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-CUMUL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	foundCumulative := false
	for i, action := range audit.actions {
		if action == domain.AuditActionAMLFlagged {
			if tt, ok := audit.meta[i]["threshold_type"]; ok && tt == "cumulative" {
				foundCumulative = true
				break
			}
		}
	}
	if !foundCumulative {
		t.Errorf("expected AuditActionAMLFlagged with threshold_type=cumulative; got actions=%v", audit.actions)
	}
}

// TestWebhookPaymentService_CreditFromRecurrente_MarkCapturedError_CreditStillSucceeds
// verifies that a failure in MarkCapturedByToken is non-fatal: the balance was
// already credited, so the function must return nil rather than propagating the
// mark error.
func TestWebhookPaymentService_CreditFromRecurrente_MarkCapturedError_CreditStillSucceeds(t *testing.T) {
	ledger := &webhookLedgerRepoStub{}
	intent := &webhookIntentRepoStub{markCapturedErr: errors.New("db write failed")}
	svc := newWebhookPaymentSvc(ledger, intent)

	err := svc.CreditFromRecurrente(context.Background(), 5, 5000, "GTQ", "REF-MARK-ERR")
	if err != nil {
		t.Errorf("MarkCapturedByToken error must be non-fatal, got: %v", err)
	}
	// Ledger must still have been credited.
	if ledger.capturedKind == "" {
		t.Error("expected credit to be applied despite mark error")
	}
}

// ── USD-balance user path (SetUserRepository) ─────────────────────────────────

// webhookUserRepoStub is a minimal UserRepository returning a configurable
// balance_currency. Wired via SetUserRepository to test the USD-balance
// PayPal credit path in ResolveAndCreditPayPalIntent.
type webhookUserRepoStub struct {
	balCurrency string
}

func (r *webhookUserRepoStub) Create(_ context.Context, _ *domain.User) error { return nil }
func (r *webhookUserRepoStub) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) GetByExternalSubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) Update(_ context.Context, _ *domain.User) error { return nil }
func (r *webhookUserRepoStub) Delete(_ context.Context, _ int) error          { return nil }
func (r *webhookUserRepoStub) List(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *webhookUserRepoStub) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) Unban(_ context.Context, _ int) error { return nil }
func (r *webhookUserRepoStub) ListBanned(_ context.Context) ([]*domain.User, error) {
	return nil, nil
}
func (r *webhookUserRepoStub) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", nil
}
func (r *webhookUserRepoStub) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, nil
}
func (r *webhookUserRepoStub) GetBalance(_ context.Context, _ int) (int, int, error) {
	return 0, 0, nil
}
func (r *webhookUserRepoStub) GetBalanceCurrency(_ context.Context, _ int) (string, error) {
	return r.balCurrency, nil
}
func (r *webhookUserRepoStub) UpdateLocale(_ context.Context, _ int, _ string) error { return nil }
func (r *webhookUserRepoStub) SetRole(_ context.Context, _ int, _ domain.UserRole) (*domain.User, error) {
	return nil, nil
}

func TestWebhookPaymentService_ResolveAndCreditPayPalIntent_USDBalanceUser_CreditsUSDDirectly(t *testing.T) {
	// When the user has balance_currency="USD", the PayPal USD amount is credited
	// directly without GTQ conversion, even when fxSvc is wired.
	intent := &domain.PaymentIntent{
		ID: 10, UserID: 7, AmountCents: 1000,
		Currency: "USD", Status: domain.PaymentIntentPending,
	}
	intentRepo := &webhookIntentRepoStub{intent: intent}
	fx := &fxSvcStub{gtqCents: 7800}
	ur := &webhookUserRepoStub{balCurrency: "USD"}

	svc := newWebhookPaymentSvcWithFX(&webhookLedgerRepoStub{}, intentRepo, fx)
	svc.(*webhookPaymentService).SetUserRepository(ur)

	if err := svc.ResolveAndCreditPayPalIntent(context.Background(), "tok", "CAP-USD-USER", 0); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestWebhookPaymentService_CreditFromRecurrente_USDCurrency_SetsSourceFields(t *testing.T) {
	// Recurrente USD payment: sourceCurrency must be set, no conversion applied.
	ledger := &webhookLedgerRepoStub{}
	svc := newWebhookPaymentSvc(ledger, nil)

	if err := svc.CreditFromRecurrente(context.Background(), 5, 500, "USD", "REF-USD"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ledger.capturedKind != domain.LedgerKindWebhookRecurrente {
		t.Errorf("expected kind %q, got %q", domain.LedgerKindWebhookRecurrente, ledger.capturedKind)
	}
}
