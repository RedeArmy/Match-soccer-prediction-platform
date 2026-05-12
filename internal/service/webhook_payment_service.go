package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// WebhookPaymentService processes confirmed payment notifications from
// external payment providers (Recurrente, PayPal) and credits the user's
// balance.
//
// Idempotency is the caller's responsibility: the HTTP handler layer must
// deduplicate webhook re-deliveries using the provider-specific message ID or
// event ID before calling these methods.
type WebhookPaymentService interface {
	// CreditFromRecurrente credits amountCents to userID's balance following a
	// confirmed Recurrente transaction.  reference is the provider's transaction
	// ID, stored in the audit log for traceability.
	CreditFromRecurrente(ctx context.Context, userID, amountCents int, currency, reference string) error
	// CreditFromPayPal credits amountCents following a confirmed PayPal payment.
	CreditFromPayPal(ctx context.Context, userID, amountCents int, currency, reference string) error
}

type webhookPaymentService struct {
	ledgerRepo repository.BalanceLedgerRepository
	audit      AuditLogger
	log        *zap.Logger
}

// NewWebhookPaymentService constructs a WebhookPaymentService.
func NewWebhookPaymentService(
	ledgerRepo repository.BalanceLedgerRepository,
	audit AuditLogger,
	log *zap.Logger,
) WebhookPaymentService {
	return &webhookPaymentService{ledgerRepo: ledgerRepo, audit: audit, log: log}
}

func (s *webhookPaymentService) CreditFromRecurrente(ctx context.Context, userID, amountCents int, currency, reference string) error {
	return s.credit(ctx, userID, amountCents, currency, reference, domain.LedgerKindWebhookRecurrente, domain.AuditActionWebhookPaymentCredit)
}

func (s *webhookPaymentService) CreditFromPayPal(ctx context.Context, userID, amountCents int, currency, reference string) error {
	return s.credit(ctx, userID, amountCents, currency, reference, domain.LedgerKindWebhookPayPal, domain.AuditActionWebhookPaymentCredit)
}

func (s *webhookPaymentService) credit(ctx context.Context, userID, amountCents int, currency, reference string, kind domain.BalanceLedgerKind, action string) error {
	if amountCents <= 0 {
		return apperrors.Validation("amount_cents must be positive")
	}
	if reference == "" {
		return apperrors.Validation("reference is required for webhook payments")
	}

	if err := s.ledgerRepo.Credit(ctx, userID, amountCents, kind, 0, reference, 0); err != nil {
		return err
	}

	resType := "user"
	s.audit.Log(ctx, nil, nil, action, &resType, &userID, map[string]any{
		"amount_cents": amountCents,
		"currency":     currency,
		"reference":    reference,
		"kind":         string(kind),
	})
	return nil
}

var _ WebhookPaymentService = (*webhookPaymentService)(nil)
