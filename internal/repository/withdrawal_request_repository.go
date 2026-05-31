package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
)

// PostgresWithdrawalRequestRepository is the PostgreSQL-backed implementation
// of WithdrawalRequestRepository.
type PostgresWithdrawalRequestRepository struct {
	db  *pgxpool.Pool
	enc payoutenc.Encrypter // defaults to payoutenc.Noop when not set
}

// NewPostgresWithdrawalRequestRepository constructs a repository that stores
// payout_details as plaintext JSON.  Call WithEncrypter to enable at-rest
// encryption before serving real traffic.
func NewPostgresWithdrawalRequestRepository(db *pgxpool.Pool) *PostgresWithdrawalRequestRepository {
	return &PostgresWithdrawalRequestRepository{db: db, enc: payoutenc.Noop}
}

// WithEncrypter wires an Encrypter for payout_details.  Call this at
// composition time (before any requests are served) so the field is visible
// to all subsequent reads and writes without races.
func (r *PostgresWithdrawalRequestRepository) WithEncrypter(enc payoutenc.Encrypter) *PostgresWithdrawalRequestRepository {
	r.enc = enc
	return r
}

const (
	withdrawalColumns     = "id, user_id, amount_cents, currency, method, payout_details, status, reviewed_by, notes, processed_at, created_at, updated_at"
	msgWithdrawalNotFound = "withdrawal request not found"
)

// marshalPayout encrypts and JSON-encodes payout details for storage.
func (r *PostgresWithdrawalRequestRepository) marshalPayout(details map[string]string) ([]byte, error) {
	data, err := payoutenc.Marshal(r.enc, details)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("encode payout_details: %w", err))
	}
	return data, nil
}

// unmarshalPayout decrypts (when needed) and decodes a raw payout_details blob
// from the database.  Legacy plaintext rows are read transparently during the
// migration window.
func (r *PostgresWithdrawalRequestRepository) unmarshalPayout(data []byte) (map[string]string, error) {
	m, err := payoutenc.Unmarshal(r.enc, data)
	if err != nil {
		return nil, apperrors.Internal(fmt.Errorf("decode payout_details: %w", err))
	}
	return m, nil
}

// scanFields populates a WithdrawalRequest from any rowScanner.
func (r *PostgresWithdrawalRequestRepository) scanFields(s rowScanner) (*domain.WithdrawalRequest, error) {
	w := &domain.WithdrawalRequest{}
	var payoutJSON []byte
	if err := s.Scan(
		&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
		&payoutJSON, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
		&w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	details, err := r.unmarshalPayout(payoutJSON)
	if err != nil {
		return nil, err
	}
	w.PayoutDetails = details
	return w, nil
}

// scanOne wraps scanFields for single-row queries; pgx.ErrNoRows → (nil, nil).
func (r *PostgresWithdrawalRequestRepository) scanOne(row pgx.Row) (*domain.WithdrawalRequest, error) {
	w, err := r.scanFields(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return w, nil
}

// CreateAndReserve atomically inserts the request and reserves the balance.
func (r *PostgresWithdrawalRequestRepository) CreateAndReserve(ctx context.Context, req *domain.WithdrawalRequest) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	payoutJSON, err := r.marshalPayout(req.PayoutDetails)
	if err != nil {
		return err
	}

	return withTx(ctx, r.db, "WithdrawalRequestRepository.CreateAndReserve", func(tx pgx.Tx) error {
		var payoutRaw []byte
		w := &domain.WithdrawalRequest{}
		scanErr := tx.QueryRow(ctx, `
			INSERT INTO withdrawal_requests
			      (user_id, amount_cents, currency, method, payout_details)
			VALUES ($1,     $2,          $3,       $4,     $5)
			RETURNING `+withdrawalColumns,
			req.UserID, req.AmountCents, req.Currency, req.Method, payoutJSON,
		).Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&payoutRaw, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		details, err := r.unmarshalPayout(payoutRaw)
		if err != nil {
			return err
		}
		w.PayoutDetails = details

		var balanceAfter int
		reserveErr := tx.QueryRow(ctx, `
			UPDATE users
			   SET reserved_cents = reserved_cents + $2,
			       updated_at     = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND (balance_cents - reserved_cents) >= $2
			 RETURNING balance_cents
		`, w.UserID, w.AmountCents).Scan(&balanceAfter)
		if reserveErr == pgx.ErrNoRows {
			return insufficientOrNotFound(ctx, tx, w.UserID)
		}
		if reserveErr != nil {
			return apperrors.Internal(reserveErr)
		}

		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID: w.UserID, DeltaCents: -w.AmountCents,
			Kind: domain.LedgerKindWithdrawalReserve, BalanceAfter: balanceAfter,
			RefID: int64(w.ID), RefType: "withdrawal_request",
		}); err != nil {
			return err
		}

		*req = *w
		return nil
	})
}

// GetByID returns the request or nil, nil when not found.
func (r *PostgresWithdrawalRequestRepository) GetByID(ctx context.Context, id int) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	return r.scanOne(r.db.QueryRow(ctx,
		`SELECT `+withdrawalColumns+` FROM withdrawal_requests WHERE id = $1`, id,
	))
}

// ListByUser returns all requests for a user ordered by created_at DESC.
func (r *PostgresWithdrawalRequestRepository) ListByUser(ctx context.Context, userID int) ([]*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	rows, err := r.db.Query(ctx,
		`SELECT `+withdrawalColumns+` FROM withdrawal_requests WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(row pgx.Rows) (*domain.WithdrawalRequest, error) {
		return r.scanFields(row)
	})
}

// ListPending returns all pending requests ordered by created_at ASC.
func (r *PostgresWithdrawalRequestRepository) ListPending(ctx context.Context) ([]*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	rows, err := r.db.Query(ctx,
		`SELECT `+withdrawalColumns+` FROM withdrawal_requests WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(row pgx.Rows) (*domain.WithdrawalRequest, error) {
		return r.scanFields(row)
	})
}

// Approve transitions a pending request to approved (status change only).
func (r *PostgresWithdrawalRequestRepository) Approve(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	result, err := r.scanOne(r.db.QueryRow(ctx, `
		UPDATE withdrawal_requests
		   SET status      = 'approved',
		       reviewed_by = $2,
		       notes       = $3,
		       updated_at  = NOW()
		 WHERE id = $1 AND status = 'pending'
		 RETURNING `+withdrawalColumns,
		id, reviewerID, notes,
	))
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	return r.notFoundOrConflict(ctx, id, "approved")
}

// RejectAndRelease atomically rejects the request and releases the balance reservation.
func (r *PostgresWithdrawalRequestRepository) RejectAndRelease(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var result *domain.WithdrawalRequest
	err := withTx(ctx, r.db, "WithdrawalRequestRepository.RejectAndRelease", func(tx pgx.Tx) error {
		var payoutRaw []byte
		w := &domain.WithdrawalRequest{}
		scanErr := tx.QueryRow(ctx, `
			UPDATE withdrawal_requests
			   SET status      = 'rejected',
			       reviewed_by = $2,
			       notes       = $3,
			       updated_at  = NOW()
			 WHERE id = $1 AND status = 'pending'
			 RETURNING `+withdrawalColumns,
			id, reviewerID, notes,
		).Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&payoutRaw, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil // handled outside tx
		}
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		details, err := r.unmarshalPayout(payoutRaw)
		if err != nil {
			return err
		}
		w.PayoutDetails = details

		var balanceAfter int
		releaseErr := tx.QueryRow(ctx, `
			UPDATE users
			   SET reserved_cents = reserved_cents - $2,
			       updated_at     = NOW()
			 WHERE id = $1 AND deleted_at IS NULL AND reserved_cents >= $2
			 RETURNING balance_cents
		`, w.UserID, w.AmountCents).Scan(&balanceAfter)
		if releaseErr != nil && releaseErr != pgx.ErrNoRows {
			return apperrors.Internal(releaseErr)
		}

		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID: w.UserID, DeltaCents: w.AmountCents,
			Kind: domain.LedgerKindWithdrawalRelease, BalanceAfter: balanceAfter,
			RefID: int64(w.ID), RefType: "withdrawal_request", CreatorID: reviewerID,
		}); err != nil {
			return err
		}

		result = w
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return r.notFoundOrConflict(ctx, id, "rejected")
	}
	return result, nil
}

// MarkProcessedAndCommit atomically marks the request as processed and commits the reservation.
func (r *PostgresWithdrawalRequestRepository) MarkProcessedAndCommit(ctx context.Context, id int) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var result *domain.WithdrawalRequest
	err := withTx(ctx, r.db, "WithdrawalRequestRepository.MarkProcessedAndCommit", func(tx pgx.Tx) error {
		var payoutRaw []byte
		w := &domain.WithdrawalRequest{}
		scanErr := tx.QueryRow(ctx, `
			UPDATE withdrawal_requests
			   SET status       = 'processed',
			       processed_at = NOW(),
			       updated_at   = NOW()
			 WHERE id = $1 AND status = 'approved'
			 RETURNING `+withdrawalColumns,
			id,
		).Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&payoutRaw, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil // handled outside tx
		}
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		details, err := r.unmarshalPayout(payoutRaw)
		if err != nil {
			return err
		}
		w.PayoutDetails = details

		var balanceAfter int
		commitErr := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents  = balance_cents  - $2,
			       reserved_cents = reserved_cents - $2,
			       updated_at     = NOW()
			 WHERE id = $1 AND deleted_at IS NULL AND reserved_cents >= $2
			 RETURNING balance_cents
		`, w.UserID, w.AmountCents).Scan(&balanceAfter)
		if commitErr == pgx.ErrNoRows {
			return apperrors.Conflict("insufficient reserved balance to commit withdrawal")
		}
		if commitErr != nil {
			return apperrors.Internal(commitErr)
		}

		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID: w.UserID, DeltaCents: -w.AmountCents,
			Kind: domain.LedgerKindWithdrawalDeduct, BalanceAfter: balanceAfter,
			RefID: int64(w.ID), RefType: "withdrawal_request",
		}); err != nil {
			return err
		}

		result = w
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return r.notFoundOrConflict(ctx, id, "processed")
	}
	return result, nil
}

func (r *PostgresWithdrawalRequestRepository) notFoundOrConflict(ctx context.Context, id int, targetStatus string) (*domain.WithdrawalRequest, error) {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound(msgWithdrawalNotFound)
	}
	if string(existing.Status) == targetStatus {
		return existing, nil
	}
	return nil, apperrors.Conflict(fmt.Sprintf("withdrawal already %s", existing.Status))
}

var _ WithdrawalRequestRepository = (*PostgresWithdrawalRequestRepository)(nil)
