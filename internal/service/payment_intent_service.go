package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// PaymentIntentCreator manages server-generated payment intents used as
// opaque PayPal custom_id values or Recurrente wcq_reference values.
type PaymentIntentCreator interface {
	// Create generates a new pending PayPal payment intent for userID.
	Create(ctx context.Context, userID, amountCents int, currency string) (*domain.PaymentIntent, error)
	// CreateForRecurrente generates a new pending intent for a Recurrente checkout.
	CreateForRecurrente(ctx context.Context, userID, amountCents int, currency, reference string) (*domain.PaymentIntent, error)
	// ListMyPending returns actionable (non-captured, non-rejected) intents for
	// userID. Used to verify ownership during comprobante uploads.
	ListMyPending(ctx context.Context, userID int) ([]*domain.PaymentIntent, error)
	// ListMyAll returns all intents for userID sorted by created_at DESC.
	// Used on the balance page to show the full payment history.
	ListMyAll(ctx context.Context, userID int) ([]*domain.PaymentIntent, error)
	// SetComprobanteByToken stores the comprobante file metadata on the intent
	// identified by token. userID is validated against the intent so a caller
	// cannot attach a comprobante to another user's payment intent. Returns
	// apperrors.NotFound when the token is unknown or owned by a different user.
	SetComprobanteByToken(ctx context.Context, userID int, token, key, contentType string, fileSize int) error
	// ResubmitForReview transitions a rejected intent to under_review, records
	// user_notes, and optionally replaces the comprobante. The userID is validated
	// against the intent so a user cannot submit on behalf of another.
	ResubmitForReview(ctx context.Context, userID int, token string, comprobanteKey, contentType *string, fileSize *int, notes string) (*domain.PaymentIntent, error)
	// CancelIntent transitions a pending intent to cancelled when the user
	// abandons the provider checkout. userID is validated against the intent.
	CancelIntent(ctx context.Context, token string, userID int) error
}

type paymentIntentService struct {
	intentRepo repository.PaymentIntentRepository
	params     SystemParamService
	log        *zap.Logger
}

// NewPaymentIntentService constructs a PaymentIntentCreator.
func NewPaymentIntentService(intentRepo repository.PaymentIntentRepository, params SystemParamService, log *zap.Logger) PaymentIntentCreator {
	return &paymentIntentService{intentRepo: intentRepo, params: params, log: log}
}

func (s *paymentIntentService) Create(ctx context.Context, userID, amountCents int, currency string) (*domain.PaymentIntent, error) {
	if amountCents <= 0 {
		return nil, apperrors.Validation("amount_cents must be positive")
	}
	maxCents := s.params.GetInt(ctx, domain.ParamKeyPaymentIntentMaxCents, domain.DefaultPaymentIntentMaxCents)
	if amountCents > maxCents {
		return nil, apperrors.Validation(fmt.Sprintf("amount_cents exceeds maximum of %d", maxCents))
	}
	if currency == "" {
		currency = "GTQ"
	}

	token, err := generateIntentToken()
	if err != nil {
		return nil, apperrors.Internal(err)
	}

	ttl := time.Duration(s.params.GetInt(ctx, domain.ParamKeyPaymentIntentTTLMinutes, domain.DefaultPaymentIntentTTLMinutes)) * time.Minute
	intent := &domain.PaymentIntent{
		Token:       token,
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    currency,
		Provider:    "paypal",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(ttl),
	}

	if err := s.intentRepo.Create(ctx, intent); err != nil {
		s.log.Error("payment intent: failed to create",
			append([]zap.Field{
				zap.Int("user_id", userID),
				zap.Int("amount_cents", amountCents),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...)
		return nil, err
	}
	return intent, nil
}

// CreateForRecurrente creates a pending intent for a Recurrente checkout using
// the caller-supplied reference (wcq_reference) as the token. The token is
// already validated to be a non-empty hex string by the caller.
func (s *paymentIntentService) CreateForRecurrente(ctx context.Context, userID, amountCents int, currency, reference string) (*domain.PaymentIntent, error) {
	if amountCents <= 0 {
		return nil, apperrors.Validation("amount_cents must be positive")
	}
	if reference == "" {
		return nil, apperrors.Validation("reference is required")
	}
	maxCents := s.params.GetInt(ctx, domain.ParamKeyPaymentIntentMaxCents, domain.DefaultPaymentIntentMaxCents)
	if amountCents > maxCents {
		return nil, apperrors.Validation(fmt.Sprintf("amount_cents exceeds maximum of %d", maxCents))
	}
	if currency == "" {
		currency = "GTQ"
	}

	ttl := time.Duration(s.params.GetInt(ctx, domain.ParamKeyPaymentIntentTTLMinutes, domain.DefaultPaymentIntentTTLMinutes)) * time.Minute
	intent := &domain.PaymentIntent{
		Token:       reference,
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    currency,
		Provider:    "recurrente",
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(ttl),
	}

	if err := s.intentRepo.Create(ctx, intent); err != nil {
		s.log.Error("payment intent: failed to create for recurrente",
			append([]zap.Field{
				zap.Int("user_id", userID),
				zap.Int("amount_cents", amountCents),
				zap.String("reference", reference),
				zap.Error(err),
			}, tracing.LogFields(ctx)...)...)
		return nil, err
	}
	return intent, nil
}

// ListMyPending returns actionable intents for userID.
func (s *paymentIntentService) ListMyPending(ctx context.Context, userID int) ([]*domain.PaymentIntent, error) {
	return s.intentRepo.ListByUserPending(ctx, userID)
}

// ListMyAll returns all intents for userID.
func (s *paymentIntentService) ListMyAll(ctx context.Context, userID int) ([]*domain.PaymentIntent, error) {
	return s.intentRepo.ListAllByUser(ctx, userID)
}

// SetComprobanteByToken looks up the intent by token, validates ownership,
// and delegates to the repo. Returning NotFound (rather than Forbidden) for an
// owned-by-someone-else token avoids confirming to the caller that the token
// exists at all — the same convention ResubmitForReview and CancelIntent use.
func (s *paymentIntentService) SetComprobanteByToken(ctx context.Context, userID int, token, key, contentType string, fileSize int) error {
	intent, err := s.intentRepo.GetByToken(ctx, token)
	if err != nil {
		return err
	}
	if intent == nil || intent.UserID != userID {
		return apperrors.NotFound("payment intent not found")
	}
	return s.intentRepo.SetComprobante(ctx, intent.ID, key, contentType, fileSize)
}

// ResubmitForReview looks up the intent by token, validates ownership, and
// transitions a rejected intent to under_review with the provided evidence.
func (s *paymentIntentService) ResubmitForReview(ctx context.Context, userID int, token string, comprobanteKey, contentType *string, fileSize *int, notes string) (*domain.PaymentIntent, error) {
	if notes == "" {
		return nil, apperrors.Validation("notes are required when submitting for review")
	}
	intent, err := s.intentRepo.GetByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if intent == nil || intent.UserID != userID {
		return nil, apperrors.NotFound("payment intent not found")
	}
	if intent.Status != domain.PaymentIntentRejected {
		return nil, apperrors.Validation("only rejected intents can be submitted for review")
	}
	return s.intentRepo.SubmitForReview(ctx, intent.ID, userID, comprobanteKey, contentType, fileSize, notes)
}

// CancelIntent transitions a pending intent owned by userID to cancelled.
func (s *paymentIntentService) CancelIntent(ctx context.Context, token string, userID int) error {
	return s.intentRepo.CancelByToken(ctx, token, userID)
}

// generateIntentToken returns a 256-bit cryptographically random hex string.
func generateIntentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate intent token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

var _ PaymentIntentCreator = (*paymentIntentService)(nil)
