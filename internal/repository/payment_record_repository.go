package repository

import (
	"context"
	"fmt"

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

// Create inserts a new payment record in pending state. record.ID is
// populated on success.
func (r *PostgresPaymentRecordRepository) Create(ctx context.Context, record *domain.PaymentRecord) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO payment_records (quiniela_id, user_id, amount, currency, reference)
		 VALUES ($1, $2, $3, $4, $5)
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
	row := r.db.QueryRow(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE id = $1`, id,
	)
	return scanPaymentRecord(row)
}

// ListByQuiniela returns all payment records for a quiniela, optionally
// filtered by status. Results are ordered by created_at descending.
func (r *PostgresPaymentRecordRepository) ListByQuiniela(ctx context.Context, quinielaID int, f PaymentFilters) ([]*domain.PaymentRecord, error) {
	q := `SELECT ` + paymentColumns + ` FROM payment_records WHERE quiniela_id = $1`
	args := []any{quinielaID}
	if f.Status != nil {
		args = append(args, *f.Status)
		q += fmt.Sprintf(" AND status = $%d", len(args))
	}
	q += " ORDER BY created_at DESC"
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPaymentRecords(rows)
}

// ListByUser returns all payment records for a user across all quinielas,
// ordered by created_at descending.
func (r *PostgresPaymentRecordRepository) ListByUser(ctx context.Context, userID int) ([]*domain.PaymentRecord, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPaymentRecords(rows)
}

// ListPending returns all payment records in pending state, ordered oldest
// first to process by arrival order.
func (r *PostgresPaymentRecordRepository) ListPending(ctx context.Context) ([]*domain.PaymentRecord, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+paymentColumns+` FROM payment_records WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectPaymentRecords(rows)
}

// Validate transitions a pending payment to confirmed, recording which admin
// approved it and any approval notes. Returns NotFound when the payment does
// not exist or is not in pending state.
func (r *PostgresPaymentRecordRepository) Validate(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error) {
	row := r.db.QueryRow(ctx,
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
	result, err := scanPaymentRecord(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(msgPaymentNotFound)
	}
	return result, nil
}

// Reject transitions a pending payment to rejected, recording the reviewing
// admin and reason. Returns NotFound when the payment does not exist or is
// not in pending state.
func (r *PostgresPaymentRecordRepository) Reject(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE payment_records
		    SET status      = 'rejected',
		        reviewed_by = $2,
		        notes       = $3,
		        updated_at  = NOW()
		  WHERE id = $1 AND status = 'pending'
		  RETURNING `+paymentColumns,
		id, adminID, notes,
	)
	result, err := scanPaymentRecord(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(msgPaymentNotFound)
	}
	return result, nil
}

var _ PaymentRecordRepository = (*PostgresPaymentRecordRepository)(nil)
