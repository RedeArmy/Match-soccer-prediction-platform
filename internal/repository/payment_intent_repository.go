package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// intentColumns is the ordered column list shared by all SELECT/RETURNING
// queries; field order must match scanIntentRow / scanIntentRows exactly.
const intentColumns = `id, token, user_id, amount_cents, currency, provider, status,
       capture_id, expires_at, comprobante_key, comprobante_content_type,
       comprobante_file_size, comprobante_required, user_notes,
       reviewed_by, review_notes, rejected_at,
       created_at, updated_at`

// PostgresPaymentIntentRepository is the PostgreSQL-backed implementation of
// PaymentIntentRepository.
type PostgresPaymentIntentRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPaymentIntentRepository constructs a PostgresPaymentIntentRepository.
func NewPostgresPaymentIntentRepository(db *pgxpool.Pool) *PostgresPaymentIntentRepository {
	return &PostgresPaymentIntentRepository{db: db}
}

// scanIntentRow scans a single pgx.Row (from QueryRow / RETURNING) into a
// domain.PaymentIntent.  Field order must match intentColumns.
func scanIntentRow(row pgx.Row) (*domain.PaymentIntent, error) {
	return scanIntentFields(func(dest ...any) error { return row.Scan(dest...) })
}

// scanIntentRows scans the current pgx.Rows position into a domain.PaymentIntent.
// Called inside a rows.Next() loop.
func scanIntentRows(rows pgx.Rows) (*domain.PaymentIntent, error) {
	return scanIntentFields(func(dest ...any) error { return rows.Scan(dest...) })
}

// scanIntentFields is the shared scan implementation.
func scanIntentFields(scan func(...any) error) (*domain.PaymentIntent, error) {
	var i domain.PaymentIntent
	err := scan(
		&i.ID, &i.Token, &i.UserID, &i.AmountCents, &i.Currency, &i.Provider,
		&i.Status, &i.CaptureID, &i.ExpiresAt,
		&i.ComprobanteKey, &i.ComprobanteContentType, &i.ComprobanteFileSize,
		&i.ComprobanteRequired, &i.UserNotes,
		&i.ReviewedBy, &i.ReviewNotes, &i.RejectedAt,
		&i.CreatedAt, &i.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &i, nil
}

// Create inserts a new pending payment intent and populates intent.ID and
// intent.CreatedAt on success.
func (r *PostgresPaymentIntentRepository) Create(ctx context.Context, intent *domain.PaymentIntent) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	provider := intent.Provider
	if provider == "" {
		provider = "paypal"
	}

	return r.db.QueryRow(ctx, `
		INSERT INTO payment_intents (token, user_id, amount_cents, currency, provider, status, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at
	`, intent.Token, intent.UserID, intent.AmountCents, intent.Currency, provider,
		intent.Status, intent.ExpiresAt,
	).Scan(&intent.ID, &intent.CreatedAt, &intent.UpdatedAt)
}

// GetByToken returns the intent matching token regardless of status.
// Returns nil, nil when no intent exists with that token.
func (r *PostgresPaymentIntentRepository) GetByToken(ctx context.Context, token string) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	i, err := scanIntentRow(r.db.QueryRow(ctx,
		`SELECT `+intentColumns+` FROM payment_intents WHERE token = $1`, token,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// GetByID returns the intent matching id. Returns nil, nil when not found.
func (r *PostgresPaymentIntentRepository) GetByID(ctx context.Context, id int64) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	i, err := scanIntentRow(r.db.QueryRow(ctx,
		`SELECT `+intentColumns+` FROM payment_intents WHERE id = $1`, id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// CaptureAndCredit atomically transitions a pending, non-expired intent to
// captured and credits the user's balance in a single transaction.
// creditAmountCents is in the user's balance_currency. sourceCurrency and
// sourceAmountCents preserve the original payment denomination for display.
func (r *PostgresPaymentIntentRepository) CaptureAndCredit(ctx context.Context, token, captureID string, creditAmountCents int, sourceCurrency string, sourceAmountCents int) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var captured *domain.PaymentIntent

	err := withRetryTx(ctx, r.db, "PaymentIntentRepository.CaptureAndCredit", func(tx pgx.Tx) error {
		intent, err := captureIntentTx(ctx, tx, token, captureID)
		if err != nil {
			return err
		}
		if intent == nil {
			captured, err = resolveCaptureMissTx(ctx, tx, token, captureID)
			return err
		}
		if err := creditUserTx(ctx, tx, intent, creditAmountCents, sourceCurrency, sourceAmountCents); err != nil {
			return err
		}
		captured = intent
		return nil
	})

	if errors.Is(err, ErrPaymentIntentAlreadyCaptured) {
		return captured, ErrPaymentIntentAlreadyCaptured
	}
	if err != nil {
		return nil, err
	}
	return captured, nil
}

// captureIntentTx attempts the atomic UPDATE that transitions a pending intent
// to captured. Returns (intent, nil) on success, (nil, nil) when no row matched.
func captureIntentTx(ctx context.Context, tx pgx.Tx, token, captureID string) (*domain.PaymentIntent, error) {
	i, err := scanIntentRow(tx.QueryRow(ctx, `
		UPDATE payment_intents
		   SET status     = 'captured',
		       capture_id = $2,
		       updated_at = NOW()
		 WHERE token     = $1
		   AND status    = 'pending'
		   AND expires_at > NOW()
		 RETURNING `+intentColumns,
		token, captureID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// creditUserTx credits creditAmountCents to the user's balance and appends a
// ledger row inside tx. Kind is derived from intent.Provider.
// When sourceCurrency is non-empty the balance_currency column is set to that
// value (only takes effect when balance_currency is still at its default 'GTQ').
func creditUserTx(ctx context.Context, tx pgx.Tx, intent *domain.PaymentIntent, creditAmountCents int, sourceCurrency string, sourceAmountCents int) error {
	var balanceAfter int
	var err error
	if sourceCurrency != "" {
		err = tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents    = balance_cents + $2,
			       balance_currency = $3,
			       updated_at       = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, intent.UserID, creditAmountCents, sourceCurrency).Scan(&balanceAfter)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents + $2,
			       updated_at    = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, intent.UserID, creditAmountCents).Scan(&balanceAfter)
	}
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

	kind := domain.LedgerKindWebhookPayPal
	if intent.Provider == "recurrente" {
		kind = domain.LedgerKindWebhookRecurrente
	}

	return insertLedgerTx(ctx, tx, ledgerRow{
		UserID:            intent.UserID,
		DeltaCents:        creditAmountCents,
		Kind:              kind,
		BalanceAfter:      balanceAfter,
		RefID:             intent.ID,
		RefType:           "payment_intent",
		Reference:         captureID,
		SourceCurrency:    sourceCurrency,
		SourceAmountCents: sourceAmountCents,
	})
}

// resolveCaptureMissTx looks up the intent by token to determine why the
// capture UPDATE matched 0 rows.
func resolveCaptureMissTx(ctx context.Context, tx pgx.Tx, token, captureID string) (*domain.PaymentIntent, error) {
	existing, err := scanIntentRow(tx.QueryRow(ctx,
		`SELECT `+intentColumns+` FROM payment_intents WHERE token = $1`, token,
	))
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
		return existing, ErrPaymentIntentAlreadyCaptured
	}
	return nil, apperrors.Conflict("payment intent already captured by a different transaction")
}

// MarkCapturedByToken transitions a pending intent to captured without
// crediting the balance. Silently succeeds when not found or already captured.
func (r *PostgresPaymentIntentRepository) MarkCapturedByToken(ctx context.Context, token string) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	_, err := r.db.Exec(ctx, `
		UPDATE payment_intents
		   SET status     = 'captured',
		       updated_at = NOW()
		 WHERE token      = $1
		   AND status     = 'pending'
	`, token)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// SetComprobante stores the file proof metadata on the intent.
func (r *PostgresPaymentIntentRepository) SetComprobante(ctx context.Context, id int64, key, contentType string, fileSize int) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	tag, err := r.db.Exec(ctx, `
		UPDATE payment_intents
		   SET comprobante_key          = $2,
		       comprobante_content_type = $3,
		       comprobante_file_size    = $4,
		       updated_at               = NOW()
		 WHERE id = $1
		   AND status IN ('pending', 'expired')
	`, id, key, contentType, fileSize)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("payment intent not found or not editable")
	}
	return nil
}

// creditLedgerKind returns the ledger entry kind for the payment provider.
func creditLedgerKind(provider string) domain.BalanceLedgerKind {
	if provider == "recurrente" {
		return domain.LedgerKindWebhookRecurrente
	}
	return domain.LedgerKindWebhookPayPal
}

// AdminCreditExpired credits creditAmountCents to the user, transitions the
// intent to captured, and records the reviewer.
func (r *PostgresPaymentIntentRepository) AdminCreditExpired(ctx context.Context, id int64, adminID, creditAmountCents int, notes string) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	var intent *domain.PaymentIntent
	err := withRetryTx(ctx, r.db, "PaymentIntentRepository.AdminCreditExpired", func(tx pgx.Tx) error {
		existing, err := scanIntentRow(tx.QueryRow(ctx,
			`SELECT `+intentColumns+` FROM payment_intents WHERE id = $1 FOR UPDATE`, id,
		))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("payment intent not found")
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		if existing.Status == domain.PaymentIntentCaptured {
			return apperrors.Conflict("payment intent already captured")
		}

		var balanceAfter int
		err = tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents + $2,
			       updated_at    = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, existing.UserID, creditAmountCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("user not found")
		}
		if err != nil {
			return apperrors.Internal(err)
		}

		kind := creditLedgerKind(existing.Provider)
		captureRef := fmt.Sprintf("admin-credit-intent-%d", id)
		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID:       existing.UserID,
			DeltaCents:   creditAmountCents,
			Kind:         kind,
			BalanceAfter: balanceAfter,
			RefID:        existing.ID,
			RefType:      "payment_intent",
			Reference:    captureRef,
			CreatorID:    adminID,
		}); err != nil {
			return err
		}

		updated, err := scanIntentRow(tx.QueryRow(ctx, `
			UPDATE payment_intents
			   SET status       = 'captured',
			       capture_id   = $2,
			       reviewed_by  = $3,
			       review_notes = $4,
			       updated_at   = NOW()
			 WHERE id = $1
			 RETURNING `+intentColumns,
			id, captureRef, adminID, notes))
		if err != nil {
			return apperrors.Internal(err)
		}
		intent = updated
		return nil
	})
	if err != nil {
		return nil, err
	}
	return intent, nil
}

// AdminReject transitions the intent to rejected and records the review.
// Accepts pending, expired, and under_review statuses.
func (r *PostgresPaymentIntentRepository) AdminReject(ctx context.Context, id int64, adminID int, notes string) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	i, err := scanIntentRow(r.db.QueryRow(ctx, `
		UPDATE payment_intents
		   SET status       = 'rejected',
		       reviewed_by  = $2,
		       review_notes = $3,
		       rejected_at  = NOW(),
		       updated_at   = NOW()
		 WHERE id = $1
		   AND status IN ('pending', 'expired', 'under_review')
		 RETURNING `+intentColumns,
		id, adminID, notes))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("payment intent not found or not rejectable")
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// RequestComprobante sets comprobante_required=true on a pending intent so
// the user is prompted to upload a receipt on the balance page.
func (r *PostgresPaymentIntentRepository) RequestComprobante(ctx context.Context, id int64, adminID int) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	i, err := scanIntentRow(r.db.QueryRow(ctx, `
		UPDATE payment_intents
		   SET comprobante_required = TRUE,
		       reviewed_by          = $2,
		       updated_at           = NOW()
		 WHERE id = $1
		   AND status = 'pending'
		 RETURNING `+intentColumns,
		id, adminID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("payment intent not found or not pending")
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// ListAllByUser returns all intents for userID sorted by created_at DESC.
func (r *PostgresPaymentIntentRepository) ListAllByUser(ctx context.Context, userID int) ([]*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	rows, err := r.db.Query(ctx, `
		SELECT `+intentColumns+`
		  FROM payment_intents
		 WHERE user_id = $1
		 ORDER BY created_at DESC
		 LIMIT 100
	`, userID)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.PaymentIntent
	for rows.Next() {
		i, err := scanIntentRows(rows)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// SubmitForReview transitions a rejected intent to under_review, sets user_notes,
// and optionally updates the comprobante fields. userID is validated against
// intent.user_id so users cannot submit on behalf of others.
func (r *PostgresPaymentIntentRepository) SubmitForReview(ctx context.Context, id int64, userID int, comprobanteKey, contentType *string, fileSize *int, notes string) (*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	// Build dynamic SET for optional comprobante fields.
	var setClauses string
	args := []any{id, userID, notes}
	n := 4
	if comprobanteKey != nil {
		setClauses = fmt.Sprintf(", comprobante_key = $%d, comprobante_content_type = $%d, comprobante_file_size = $%d", n, n+1, n+2)
		args = append(args, *comprobanteKey, *contentType, *fileSize)
		n += 3
	}
	_ = n // suppress unused warning

	i, err := scanIntentRow(r.db.QueryRow(ctx, `
		UPDATE payment_intents
		   SET status     = 'under_review',
		       user_notes = $3,
		       updated_at = NOW()`+setClauses+`
		 WHERE id      = $1
		   AND user_id = $2
		   AND status  = 'rejected'
		 RETURNING `+intentColumns,
		args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, apperrors.NotFound("payment intent not found, not yours, or not rejectable")
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return i, nil
}

// ListForAdmin returns intents matching filters ordered by created_at DESC.
// Also returns the total count before pagination.
func (r *PostgresPaymentIntentRepository) ListForAdmin(ctx context.Context, f PaymentIntentFilters, p Pagination) ([]*domain.PaymentIntent, int, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	wb := newWhereBuilder()
	if f.Provider != nil {
		wb.add("provider = $%d", *f.Provider)
	}
	if f.Status != nil {
		wb.add("status = $%d", string(*f.Status))
	}

	var total int
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM payment_intents`+wb.clause(), wb.args...,
	).Scan(&total); err != nil {
		return nil, 0, apperrors.Internal(err)
	}

	q := `SELECT ` + intentColumns + ` FROM payment_intents` + wb.clause() + ` ORDER BY created_at DESC`
	q, pagedArgs, _, err := applyPagination(q, wb.args, wb.next(), p)
	if err != nil {
		return nil, 0, apperrors.BadRequest(err.Error())
	}
	rows, err := r.db.Query(ctx, q, pagedArgs...)
	if err != nil {
		return nil, 0, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.PaymentIntent
	for rows.Next() {
		i, err := scanIntentRows(rows)
		if err != nil {
			return nil, 0, apperrors.Internal(err)
		}
		out = append(out, i)
	}
	return out, total, rows.Err()
}

// ListByUserPending returns actionable (non-captured, non-rejected) intents for userID
// ordered by created_at DESC. Used to verify ownership during comprobante uploads.
func (r *PostgresPaymentIntentRepository) ListByUserPending(ctx context.Context, userID int) ([]*domain.PaymentIntent, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	rows, err := r.db.Query(ctx, `
		SELECT `+intentColumns+`
		  FROM payment_intents
		 WHERE user_id = $1
		   AND status IN ('pending', 'expired', 'under_review')
		 ORDER BY created_at DESC
		 LIMIT 20
	`, userID)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var out []*domain.PaymentIntent
	for rows.Next() {
		i, err := scanIntentRows(rows)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

// CancelByToken transitions a pending intent owned by userID to cancelled.
// Returns apperrors.NotFound when no matching pending intent exists.
func (r *PostgresPaymentIntentRepository) CancelByToken(ctx context.Context, token string, userID int) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	res, err := r.db.Exec(ctx,
		`UPDATE payment_intents
		    SET status = 'cancelled', updated_at = NOW()
		  WHERE token = $1 AND user_id = $2 AND status = 'pending'`,
		token, userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if res.RowsAffected() == 0 {
		return apperrors.NotFound("payment intent not found or not cancellable")
	}
	return nil
}

// CancelStalePendingCardIntents bulk-cancels Recurrente and PayPal intents
// that are still pending and were created before the given cutoff. Both
// providers expire checkout sessions after ~1 hour, so any intent that old
// will never be captured. Bank transfers live in a separate table and are
// never touched by this query. The caller passes time.Now().Add(-time.Hour).
func (r *PostgresPaymentIntentRepository) CancelStalePendingCardIntents(ctx context.Context, before time.Time) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	tag, err := r.db.Exec(ctx,
		`UPDATE payment_intents
		    SET status = 'cancelled', updated_at = NOW()
		  WHERE status   = 'pending'
		    AND provider IN ('recurrente', 'paypal')
		    AND created_at < $1`,
		before,
	)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

// _ asserts interface compliance at compile time.
var _ PaymentIntentRepository = (*PostgresPaymentIntentRepository)(nil)
