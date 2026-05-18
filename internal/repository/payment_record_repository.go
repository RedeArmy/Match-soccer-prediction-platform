package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresPaymentRecordRepository is the PostgreSQL-backed implementation of
// PaymentRecordRepository.
type PostgresPaymentRecordRepository struct {
	db *pgxpool.Pool
}

// NewPostgresPaymentRecordRepository constructs a PostgresPaymentRecordRepository.
func NewPostgresPaymentRecordRepository(db *pgxpool.Pool) *PostgresPaymentRecordRepository {
	return &PostgresPaymentRecordRepository{db: db}
}

const paymentColumns = "id, quiniela_id, user_id, amount, currency, status, reference, reviewed_by, notes, confirmed_at, created_at, updated_at"

const msgPaymentNotFound = "payment record not found"

func scanPaymentRecord(row pgx.Row) (*domain.PaymentRecord, error) {
	p := &domain.PaymentRecord{}
	err := row.Scan(
		&p.ID, &p.QuinielaID, &p.UserID, &p.Amount, &p.Currency, &p.Status,
		&p.Reference, &p.ReviewedBy, &p.Notes, &p.ConfirmedAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return p, nil
}

func collectPaymentRecords(rows pgx.Rows) ([]*domain.PaymentRecord, error) {
	var records []*domain.PaymentRecord
	for rows.Next() {
		p := &domain.PaymentRecord{}
		if err := rows.Scan(
			&p.ID, &p.QuinielaID, &p.UserID, &p.Amount, &p.Currency, &p.Status,
			&p.Reference, &p.ReviewedBy, &p.Notes, &p.ConfirmedAt,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		records = append(records, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return records, nil
}

func (r *PostgresPaymentRecordRepository) queryPaymentRecords(ctx context.Context, q string, args ...any) ([]*domain.PaymentRecord, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPaymentRecords(rows)
}

// Create inserts a new payment record in pending state. When reference is
// non-null and non-empty, the insert is idempotent: a duplicate reference
// returns the existing row unchanged rather than an error, making the
// operation safe to retry on webhook re-delivery or client retries.
// record is populated with the inserted or existing row on success.
func (r *PostgresPaymentRecordRepository) Create(ctx context.Context, record *domain.PaymentRecord) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx,
		`INSERT INTO payment_records (quiniela_id, user_id, amount, currency, reference)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (reference) WHERE reference IS NOT NULL AND reference <> ''
		 DO UPDATE SET updated_at = payment_records.updated_at
		 RETURNING `+paymentColumns,
		record.QuinielaID, record.UserID, record.Amount, record.Currency, record.Reference,
	)
	result, err := scanPaymentRecord(row)
	if err != nil {
		return err
	}
	*record = *result
	return nil
}

// GetByID returns a payment record by primary key. Returns nil, nil when not
// found.
func (r *PostgresPaymentRecordRepository) GetByID(ctx context.Context, id int) (*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE id = $1`, id,
	)
	return scanPaymentRecord(row)
}

// ListByQuiniela returns all payment records for a quiniela, optionally
// filtered by status. Results are ordered by created_at descending.
func (r *PostgresPaymentRecordRepository) ListByQuiniela(ctx context.Context, quinielaID int, f PaymentFilters) ([]*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	q := `SELECT ` + paymentColumns + ` FROM payment_records WHERE quiniela_id = $1`
	args := []any{quinielaID}
	if f.Status != nil {
		args = append(args, *f.Status)
		q += fmt.Sprintf(" AND status = $%d", len(args))
	}
	q += " ORDER BY created_at DESC"
	return r.queryPaymentRecords(ctx, q, args...)
}

// ListByUser returns all payment records for a user across all quinielas,
// ordered by created_at descending.
func (r *PostgresPaymentRecordRepository) ListByUser(ctx context.Context, userID int) ([]*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	return r.queryPaymentRecords(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
}

// ListPending returns all payment records in pending state, ordered oldest
// first to process by arrival order.
func (r *PostgresPaymentRecordRepository) ListPending(ctx context.Context) ([]*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	return r.queryPaymentRecords(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE status = 'pending' ORDER BY created_at ASC`,
	)
}

// Validate transitions a pending payment to confirmed, recording which admin
// approved it and any approval notes. Returns NotFound when the payment does
// not exist or is not in pending state.
func (r *PostgresPaymentRecordRepository) Validate(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return r.reviewPending(ctx, id, adminID, notes, "confirmed", true)
}

// Reject transitions a pending payment to rejected, recording the reviewing
// admin and reason. Returns NotFound when the payment does not exist or is
// not in pending state.
func (r *PostgresPaymentRecordRepository) Reject(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return r.reviewPending(ctx, id, adminID, notes, "rejected", false)
}

func (r *PostgresPaymentRecordRepository) reviewPending(ctx context.Context, id, adminID int, notes, newStatus string, setConfirmedAt bool) (*domain.PaymentRecord, error) {
	setConfirmed := ""
	if setConfirmedAt {
		setConfirmed = ", confirmed_at = NOW()"
	}

	row := r.db.QueryRow(ctx,
		`UPDATE payment_records
		    SET status      = $4`+setConfirmed+`,
		        reviewed_by = $2,
		        notes       = $3,
		        updated_at  = NOW()
		  WHERE id = $1 AND status = 'pending'
		  RETURNING `+paymentColumns,
		id, adminID, notes, newStatus,
	)
	result, err := scanPaymentRecord(row)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}

	// 0 rows updated: check whether the record exists and is already in the
	// target state (idempotent admin retry) or in a different terminal state
	// (genuine conflict).
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound(msgPaymentNotFound)
	}
	if string(existing.Status) == newStatus {
		return existing, nil // already in target state — idempotent
	}
	return nil, apperrors.Conflict(fmt.Sprintf("payment already %s", existing.Status))
}

// List returns payment records matching the given filters with pagination.
func (r *PostgresPaymentRecordRepository) List(ctx context.Context, f PaymentFilters, p Pagination) ([]*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	wb := newWhereBuilder()
	if f.Status != nil {
		wb.add("status = $%d", string(*f.Status))
	}
	if f.QuinielaID != nil {
		wb.add("quiniela_id = $%d", *f.QuinielaID)
	}
	if f.UserID != nil {
		wb.add("user_id = $%d", *f.UserID)
	}

	q := `SELECT ` + paymentColumns + ` FROM payment_records` + wb.clause() + ` ORDER BY created_at DESC`
	q, pagedArgs, _ := applyPagination(q, wb.args, wb.next(), p)
	return r.queryPaymentRecords(ctx, q, pagedArgs...)
}

// ListStale returns pending payment records older than olderThan.
func (r *PostgresPaymentRecordRepository) ListStale(ctx context.Context, olderThan time.Time) ([]*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	return r.queryPaymentRecords(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE status = 'pending' AND created_at < $1 ORDER BY created_at ASC`,
		olderThan,
	)
}

// GetStatusCounts returns a single-row summary of payment record counts and
// the total collected amount (sum of confirmed amounts) in one query.
func (r *PostgresPaymentRecordRepository) GetStatusCounts(ctx context.Context) (PaymentStatusCounts, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var c PaymentStatusCounts
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'pending'),
			COUNT(*) FILTER (WHERE status = 'confirmed'),
			COUNT(*) FILTER (WHERE status = 'rejected'),
			COALESCE(SUM(amount) FILTER (WHERE status = 'confirmed'), 0)
		FROM payment_records`).Scan(&c.Pending, &c.Confirmed, &c.Rejected, &c.TotalCollected)
	if err != nil {
		return PaymentStatusCounts{}, apperrors.Internal(err)
	}
	return c, nil
}

// ValidateAndMarkPaid atomically confirms a pending payment AND marks the
// corresponding group membership as paid in a single transaction.
func (r *PostgresPaymentRecordRepository) ValidateAndMarkPaid(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var record *domain.PaymentRecord
	err := withTx(ctx, r.db, "PaymentRecordRepository.ValidateAndMarkPaid", func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx,
			`UPDATE payment_records
			    SET status       = 'confirmed',
			        confirmed_at = NOW(),
			        reviewed_by  = $2,
			        notes        = $3,
			        updated_at   = NOW()
			  WHERE id = $1 AND status = 'pending'
			  RETURNING `+paymentColumns,
			id, adminID, notes,
		)
		p := &domain.PaymentRecord{}
		err := row.Scan(
			&p.ID, &p.QuinielaID, &p.UserID, &p.Amount, &p.Currency, &p.Status,
			&p.Reference, &p.ReviewedBy, &p.Notes, &p.ConfirmedAt,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if err == pgx.ErrNoRows {
			return nil // handled outside tx
		}
		if err != nil {
			return apperrors.Internal(err)
		}

		_, err = tx.Exec(ctx,
			`UPDATE group_memberships SET paid=TRUE, updated_at=NOW()
			 WHERE quiniela_id=$1 AND user_id=$2`,
			p.QuinielaID, p.UserID,
		)
		if err != nil {
			return apperrors.Internal(err)
		}

		record = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	if record == nil {
		existing, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if existing == nil {
			return nil, apperrors.NotFound(msgPaymentNotFound)
		}
		if existing.Status == domain.PaymentStatusConfirmed {
			return existing, nil
		}
		return nil, apperrors.Conflict(fmt.Sprintf("payment already %s", existing.Status))
	}
	return record, nil
}

var _ PaymentRecordRepository = (*PostgresPaymentRecordRepository)(nil)
