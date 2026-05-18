package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresWithdrawalRequestRepository is the PostgreSQL-backed implementation
// of WithdrawalRequestRepository.
type PostgresWithdrawalRequestRepository struct {
	db *pgxpool.Pool
}

// NewPostgresWithdrawalRequestRepository constructs a PostgresWithdrawalRequestRepository.
func NewPostgresWithdrawalRequestRepository(db *pgxpool.Pool) *PostgresWithdrawalRequestRepository {
	return &PostgresWithdrawalRequestRepository{db: db}
}

const (
	withdrawalColumns     = "id, user_id, amount_cents, currency, method, payout_details, status, reviewed_by, notes, processed_at, created_at, updated_at"
	msgWithdrawalNotFound = "withdrawal request not found"
)

// scanWithdrawalFields populates a WithdrawalRequest from any rowScanner,
// including the JSON unmarshal of payout_details. Returns a raw scan/decode
// error; callers interpret sentinel values such as pgx.ErrNoRows.
func scanWithdrawalFields(s rowScanner) (*domain.WithdrawalRequest, error) {
	w := &domain.WithdrawalRequest{}
	var payoutJSON []byte
	if err := s.Scan(
		&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
		&payoutJSON, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
		&w.CreatedAt, &w.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(payoutJSON) > 0 {
		if err := json.Unmarshal(payoutJSON, &w.PayoutDetails); err != nil {
			return nil, err
		}
	}
	return w, nil
}

// scanWithdrawal wraps scanWithdrawalFields for single-row queries.
// pgx.ErrNoRows is translated to (nil, nil) so callers can distinguish
// "not found" from a real database error.
func scanWithdrawal(row pgx.Row) (*domain.WithdrawalRequest, error) {
	w, err := scanWithdrawalFields(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return w, nil
}

// CreateAndReserve atomically inserts the request and reserves the balance.
func (r *PostgresWithdrawalRequestRepository) CreateAndReserve(ctx context.Context, req *domain.WithdrawalRequest) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	payoutJSON, err := json.Marshal(req.PayoutDetails)
	if err != nil {
		return apperrors.Internal(fmt.Errorf("marshal payout_details: %w", err))
	}
	return withTx(ctx, r.db, "WithdrawalRequestRepository.CreateAndReserve", func(tx pgx.Tx) error {
		// Insert the request first.
		row := tx.QueryRow(ctx, `
			INSERT INTO withdrawal_requests
			      (user_id, amount_cents, currency, method, payout_details)
			VALUES ($1,     $2,          $3,       $4,     $5)
			RETURNING `+withdrawalColumns,
			req.UserID, req.AmountCents, req.Currency, req.Method, payoutJSON,
		)
		w := &domain.WithdrawalRequest{}
		var pd []byte
		scanErr := row.Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&pd, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		if len(pd) > 0 {
			_ = json.Unmarshal(pd, &w.PayoutDetails)
		}

		// Reserve the balance (available must cover the amount).
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

		if err := insertLedgerTx(ctx, tx, ledgerRow{UserID: w.UserID, DeltaCents: -w.AmountCents, Kind: domain.LedgerKindWithdrawalReserve, BalanceAfter: balanceAfter, RefID: int64(w.ID), RefType: "withdrawal_request"}); err != nil {
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
	return scanWithdrawal(r.db.QueryRow(ctx,
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
	return collectRows(rows, func(r pgx.Rows) (*domain.WithdrawalRequest, error) {
		return scanWithdrawalFields(r)
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
	return collectRows(rows, func(r pgx.Rows) (*domain.WithdrawalRequest, error) {
		return scanWithdrawalFields(r)
	})
}

// Approve transitions a pending request to approved (status change only).
func (r *PostgresWithdrawalRequestRepository) Approve(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx, `
		UPDATE withdrawal_requests
		   SET status      = 'approved',
		       reviewed_by = $2,
		       notes       = $3,
		       updated_at  = NOW()
		 WHERE id = $1 AND status = 'pending'
		 RETURNING `+withdrawalColumns,
		id, reviewerID, notes,
	)
	result, err := scanWithdrawal(row)
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
		row := tx.QueryRow(ctx, `
			UPDATE withdrawal_requests
			   SET status      = 'rejected',
			       reviewed_by = $2,
			       notes       = $3,
			       updated_at  = NOW()
			 WHERE id = $1 AND status = 'pending'
			 RETURNING `+withdrawalColumns,
			id, reviewerID, notes,
		)
		w := &domain.WithdrawalRequest{}
		var pd []byte
		scanErr := row.Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&pd, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil // handled outside tx
		}
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		if len(pd) > 0 {
			_ = json.Unmarshal(pd, &w.PayoutDetails)
		}

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

		if err := insertLedgerTx(ctx, tx, ledgerRow{UserID: w.UserID, DeltaCents: w.AmountCents, Kind: domain.LedgerKindWithdrawalRelease, BalanceAfter: balanceAfter, RefID: int64(w.ID), RefType: "withdrawal_request", CreatorID: reviewerID}); err != nil {
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
		row := tx.QueryRow(ctx, `
			UPDATE withdrawal_requests
			   SET status       = 'processed',
			       processed_at = NOW(),
			       updated_at   = NOW()
			 WHERE id = $1 AND status = 'approved'
			 RETURNING `+withdrawalColumns,
			id,
		)
		w := &domain.WithdrawalRequest{}
		var pd []byte
		scanErr := row.Scan(
			&w.ID, &w.UserID, &w.AmountCents, &w.Currency, &w.Method,
			&pd, &w.Status, &w.ReviewedBy, &w.Notes, &w.ProcessedAt,
			&w.CreatedAt, &w.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil // handled outside tx
		}
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		if len(pd) > 0 {
			_ = json.Unmarshal(pd, &w.PayoutDetails)
		}

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

		if err := insertLedgerTx(ctx, tx, ledgerRow{UserID: w.UserID, DeltaCents: -w.AmountCents, Kind: domain.LedgerKindWithdrawalDeduct, BalanceAfter: balanceAfter, RefID: int64(w.ID), RefType: "withdrawal_request"}); err != nil {
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
