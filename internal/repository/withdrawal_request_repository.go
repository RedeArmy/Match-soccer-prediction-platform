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
	withdrawalColumns     = "id, user_id, amount_cents, currency, method, payout_details, status, reviewed_by, notes, processed_at, created_at, updated_at, gtq_reserved_cents"
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
		&w.CreatedAt, &w.UpdatedAt, &w.GTQReservedCents,
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

// scanWithdrawalMutationTx executes a mutation query inside tx, scans the
// RETURNING row into a WithdrawalRequest, and decodes payout_details. It is
// the shared scan path for ApproveAndDebit and other transactional mutations.
//
// Returns (nil, nil) when the query matches no rows (ErrNoRows), signalling
// that the caller should fall through to notFoundOrConflict after the
// transaction closes.
func (r *PostgresWithdrawalRequestRepository) scanWithdrawalMutationTx(
	ctx context.Context, tx pgx.Tx, query string, args ...any,
) (*domain.WithdrawalRequest, error) {
	var payoutRaw []byte
	w := &domain.WithdrawalRequest{}
	scanErr := tx.QueryRow(ctx, query, args...).Scan(
		&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
		&payoutRaw, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
		&w.CreatedAt, &w.UpdatedAt, &w.GTQReservedCents,
	)
	if scanErr == pgx.ErrNoRows {
		return nil, nil
	}
	if scanErr != nil {
		return nil, apperrors.Internal(scanErr)
	}
	details, err := r.unmarshalPayout(payoutRaw)
	if err != nil {
		return nil, err
	}
	w.PayoutDetails = details
	return w, nil
}

// Create inserts the withdrawal request after verifying sufficient available
// balance. The balance is NOT touched here — deduction happens in ApproveAndDebit.
func (r *PostgresWithdrawalRequestRepository) Create(ctx context.Context, req *domain.WithdrawalRequest) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()

	payoutJSON, err := r.marshalPayout(req.PayoutDetails)
	if err != nil {
		return err
	}

	return withTx(ctx, r.db, "WithdrawalRequestRepository.Create", func(tx pgx.Tx) error {
		// Verify sufficient available balance before inserting.
		var available int
		balErr := tx.QueryRow(ctx, `
			SELECT (balance_cents - reserved_cents)
			  FROM users
			 WHERE id = $1 AND deleted_at IS NULL
		`, req.UserID).Scan(&available)
		if balErr == pgx.ErrNoRows {
			return apperrors.NotFound("user not found")
		}
		if balErr != nil {
			return apperrors.Internal(balErr)
		}
		if available < req.GTQReservedCents {
			return apperrors.Conflict("insufficient balance for withdrawal")
		}

		w, err := r.scanWithdrawalMutationTx(ctx, tx, `
			INSERT INTO withdrawal_requests
			      (user_id, amount_cents, currency, method, payout_details, gtq_reserved_cents)
			VALUES ($1,     $2,          $3,       $4,     $5,             $6)
			RETURNING `+withdrawalColumns,
			req.UserID, req.AmountCents, req.Currency, req.Method, payoutJSON, req.GTQReservedCents,
		)
		if isUniqueViolation(err) {
			return apperrors.Conflict("a withdrawal request is already pending for this user")
		}
		if err != nil {
			return err
		}
		if w == nil {
			return apperrors.Internal(fmt.Errorf("insert withdrawal_requests returned no rows"))
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

// ListAll returns requests optionally filtered by status, ordered by
// created_at DESC. Results are bounded by p.Limit and offset by p.Offset.
func (r *PostgresWithdrawalRequestRepository) ListAll(ctx context.Context, status string, p Pagination) ([]*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	if p.Limit <= 0 && !p.IsUnbounded() {
		return nil, apperrors.Validation("pagination limit must be positive")
	}
	var rows pgx.Rows
	var err error
	if status == "" {
		rows, err = r.db.Query(ctx,
			`SELECT `+withdrawalColumns+` FROM withdrawal_requests ORDER BY created_at DESC LIMIT $1 OFFSET $2`,
			p.Limit, p.Offset,
		)
	} else {
		rows, err = r.db.Query(ctx,
			`SELECT `+withdrawalColumns+` FROM withdrawal_requests WHERE status = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
			status, p.Limit, p.Offset,
		)
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(row pgx.Rows) (*domain.WithdrawalRequest, error) {
		return r.scanFields(row)
	})
}

// ApproveAndDebit atomically approves the request and deducts the balance.
func (r *PostgresWithdrawalRequestRepository) ApproveAndDebit(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var result *domain.WithdrawalRequest
	// withRetryTx: this debits real money on approval, so a transient
	// serialization failure/deadlock should be retried rather than surfaced to
	// the admin as a spurious 500 — same resilience policy already applied to
	// BalanceLedgerRepository.Credit for deposits (ATD-002).
	err := withRetryTx(ctx, r.db, "WithdrawalRequestRepository.ApproveAndDebit", func(tx pgx.Tx) error {
		w, err := r.scanWithdrawalMutationTx(ctx, tx, `
			UPDATE withdrawal_requests
			   SET status      = 'approved',
			       reviewed_by = $2,
			       notes       = $3,
			       updated_at  = NOW()
			 WHERE id = $1 AND status = 'pending'
			 RETURNING `+withdrawalColumns,
			id, reviewerID, notes,
		)
		if err != nil {
			return err
		}
		if w == nil {
			return nil // handled outside tx
		}

		var balanceAfter int
		debitErr := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents - $2,
			       updated_at    = NOW()
			 WHERE id = $1
			   AND deleted_at IS NULL
			   AND (balance_cents - reserved_cents) >= $2
			 RETURNING balance_cents
		`, w.UserID, w.GTQReservedCents).Scan(&balanceAfter)
		if debitErr == pgx.ErrNoRows {
			return apperrors.Conflict("insufficient available balance to approve withdrawal")
		}
		if debitErr != nil {
			return apperrors.Internal(debitErr)
		}

		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID: w.UserID, DeltaCents: -w.GTQReservedCents,
			Kind: domain.LedgerKindWithdrawalDeduct, BalanceAfter: balanceAfter,
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
		return r.notFoundOrConflict(ctx, id, "approved")
	}
	return result, nil
}

// Reject transitions a pending request to rejected (status change only, no balance ops).
func (r *PostgresWithdrawalRequestRepository) Reject(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	result, err := r.scanOne(r.db.QueryRow(ctx, `
		UPDATE withdrawal_requests
		   SET status      = 'rejected',
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
	return r.notFoundOrConflict(ctx, id, "rejected")
}

// MarkProcessed transitions an approved request to processed (status change only).
func (r *PostgresWithdrawalRequestRepository) MarkProcessed(ctx context.Context, id int) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	result, err := r.scanOne(r.db.QueryRow(ctx, `
		UPDATE withdrawal_requests
		   SET status       = 'processed',
		       processed_at = NOW(),
		       updated_at   = NOW()
		 WHERE id = $1 AND status = 'approved'
		 RETURNING `+withdrawalColumns,
		id,
	))
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	return r.notFoundOrConflict(ctx, id, "processed")
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
