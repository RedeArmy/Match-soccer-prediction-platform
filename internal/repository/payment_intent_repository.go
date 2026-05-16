package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresPaymentIntentRepository is the PostgreSQL-backed implementation of
// PaymentIntentRepository.
type PostgresPaymentIntentRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPaymentIntentRepository constructs a PostgresPaymentIntentRepository.
func NewPostgresPaymentIntentRepository(db *pgxpool.Pool) *PostgresPaymentIntentRepository {
	return &PostgresPaymentIntentRepository{db: db}
}

// Create inserts a new pending payment intent and populates intent.ID and
// intent.CreatedAt on success.
func (r *PostgresPaymentIntentRepository) Create(ctx context.Context, intent *domain.PaymentIntent) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO payment_intents (token, user_id, amount_cents, currency, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at, updated_at
	`, intent.Token, intent.UserID, intent.AmountCents, intent.Currency,
		intent.Status, intent.ExpiresAt,
	).Scan(&intent.ID, &intent.CreatedAt, &intent.UpdatedAt)
}

// CaptureAndCredit atomically transitions a pending, non-expired intent to
// captured and credits the user's balance in a single transaction.
//
// The implementation executes two SQL statements inside one transaction:
//  1. UPDATE payment_intents SET status='captured', capture_id=$2 WHERE
//     token=$1 AND status='pending' AND expires_at > NOW()
//  2. If 1 row updated: UPDATE users balance + INSERT balance_ledger row.
//  3. If 0 rows updated: check why and return the appropriate sentinel.
func (r *PostgresPaymentIntentRepository) CaptureAndCredit(ctx context.Context, token, captureID string) (*domain.PaymentIntent, error) {
	var captured *domain.PaymentIntent

	err := withTx(ctx, r.db, "PaymentIntentRepository.CaptureAndCredit", func(tx pgx.Tx) error {
		intent, err := captureIntentTx(ctx, tx, token, captureID)
		if err != nil {
			return err
		}
		if intent == nil {
			// UPDATE matched 0 rows — determine why.
			captured, err = resolveCaptureMissTx(ctx, tx, token, captureID)
			return err
		}
		if err := creditUserTx(ctx, tx, intent); err != nil {
			return err
		}
		captured = intent
		return nil
	})

	if errors.Is(err, ErrPaymentIntentAlreadyCaptured) {
		// Surface the captured intent alongside the sentinel so callers can log it.
		return captured, ErrPaymentIntentAlreadyCaptured
	}
	if err != nil {
		return nil, err
	}
	return captured, nil
}

// captureIntentTx attempts the atomic UPDATE that transitions a pending intent
// to captured. Returns (intent, nil) on success, (nil, nil) when no row matched
// (ErrNoRows), or (nil, error) on a real database error.
func captureIntentTx(ctx context.Context, tx pgx.Tx, token, captureID string) (*domain.PaymentIntent, error) {
	var intent domain.PaymentIntent
	err := tx.QueryRow(ctx, `
		UPDATE payment_intents
		   SET status     = 'captured',
		       capture_id = $2,
		       updated_at = NOW()
		 WHERE token     = $1
		   AND status    = 'pending'
		   AND expires_at > NOW()
		 RETURNING id, token, user_id, amount_cents, currency, status, capture_id,
		           expires_at, created_at, updated_at
	`, token, captureID).Scan(
		&intent.ID, &intent.Token, &intent.UserID, &intent.AmountCents,
		&intent.Currency, &intent.Status, &intent.CaptureID,
		&intent.ExpiresAt, &intent.CreatedAt, &intent.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return &intent, nil
}

// creditUserTx credits the user's balance and appends a ledger row inside tx.
func creditUserTx(ctx context.Context, tx pgx.Tx, intent *domain.PaymentIntent) error {
	var balanceAfter int
	err := tx.QueryRow(ctx, `
		UPDATE users
		   SET balance_cents = balance_cents + $2,
		       updated_at    = NOW()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING balance_cents
	`, intent.UserID, intent.AmountCents).Scan(&balanceAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound("user not found")
	}
	if err != nil {
		return apperrors.Internal(err)
	}
	captureID := ""
	if intent.CaptureID != nil {
		captureID = *intent.CaptureID
	}
	return insertLedgerTx(ctx, tx, ledgerRow{
		UserID:       intent.UserID,
		DeltaCents:   intent.AmountCents,
		Kind:         domain.LedgerKindWebhookPayPal,
		BalanceAfter: balanceAfter,
		RefID:        intent.ID,
		RefType:      "payment_intent",
		Reference:    captureID,
	})
}

// resolveCaptureMissTx looks up the intent by token to determine why the
// capture UPDATE matched 0 rows, then returns the appropriate sentinel error.
// Returns (intent, ErrPaymentIntentAlreadyCaptured) for idempotent re-delivery.
func resolveCaptureMissTx(ctx context.Context, tx pgx.Tx, token, captureID string) (*domain.PaymentIntent, error) {
	var existing domain.PaymentIntent
	err := tx.QueryRow(ctx, `
		SELECT id, token, user_id, amount_cents, currency, status, capture_id,
		       expires_at, created_at, updated_at
		  FROM payment_intents
		 WHERE token = $1
	`, token).Scan(
		&existing.ID, &existing.Token, &existing.UserID, &existing.AmountCents,
		&existing.Currency, &existing.Status, &existing.CaptureID,
		&existing.ExpiresAt, &existing.CreatedAt, &existing.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("payment intent not found")
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	if existing.Status != domain.PaymentIntentCaptured {
		return nil, apperrors.NotFound("payment intent expired or unavailable")
	}
	existingCaptureID := ""
	if existing.CaptureID != nil {
		existingCaptureID = *existing.CaptureID
	}
	if existingCaptureID == captureID {
		return &existing, ErrPaymentIntentAlreadyCaptured
	}
	return nil, apperrors.Conflict("payment intent already captured by a different transaction")
}

var _ PaymentIntentRepository = (*PostgresPaymentIntentRepository)(nil)
