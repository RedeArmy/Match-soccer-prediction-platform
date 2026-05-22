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
)

// PaymentIntentCreator manages server-generated payment intents used as
// opaque PayPal custom_id values. Creating an intent before the PayPal order
// is created ensures the webhook can only credit the user who initiated the
// session — the token is unguessable and single-use.
type PaymentIntentCreator interface {
	// Create generates a new pending payment intent for userID. The returned
	// intent's Token must be passed as custom_id when the frontend creates the
	// PayPal order. The intent expires after DefaultPaymentIntentTTL.
	Create(ctx context.Context, userID, amountCents int, currency string) (*domain.PaymentIntent, error)
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
		Status:      domain.PaymentIntentPending,
		ExpiresAt:   time.Now().Add(ttl),
	}

	if err := s.intentRepo.Create(ctx, intent); err != nil {
		s.log.Error("payment intent: failed to create",
			zap.Int("user_id", userID),
			zap.Int("amount_cents", amountCents),
			zap.Error(err),
		)
		return nil, err
	}
	return intent, nil
}

// generateIntentToken returns a 256-bit cryptographically random hex string
// (64 lowercase hex characters). The token is used as PayPal custom_id; its
// entropy makes it computationally infeasible to enumerate or guess.
func generateIntentToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate intent token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

var _ PaymentIntentCreator = (*paymentIntentService)(nil)
