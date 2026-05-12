package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresBankTransferProofRepository is the PostgreSQL-backed implementation
// of BankTransferProofRepository.
type PostgresBankTransferProofRepository struct {
	db *pgxpool.Pool
}

// NewPostgresBankTransferProofRepository constructs a PostgresBankTransferProofRepository.
func NewPostgresBankTransferProofRepository(db *pgxpool.Pool) *PostgresBankTransferProofRepository {
	return &PostgresBankTransferProofRepository{db: db}
}

const (
	bankTransferColumns     = "id, user_id, amount_cents, currency, storage_key, content_type, file_size, status, reviewed_by, notes, approved_at, created_at, updated_at"
	msgBankTransferNotFound = "bank transfer proof not found"
)

func scanBankTransferProof(row pgx.Row) (*domain.BankTransferProof, error) {
	p := &domain.BankTransferProof{}
	err := row.Scan(
		&p.ID, &p.UserID, &p.AmountCents, &p.Currency,
		&p.StorageKey, &p.ContentType, &p.FileSize,
		&p.Status, &p.ReviewedBy, &p.Notes, &p.ApprovedAt,
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

func scanBankTransferProofRows(rows pgx.Rows) (*domain.BankTransferProof, error) {
	p := &domain.BankTransferProof{}
	return p, rows.Scan(
		&p.ID, &p.UserID, &p.AmountCents, &p.Currency,
		&p.StorageKey, &p.ContentType, &p.FileSize,
		&p.Status, &p.ReviewedBy, &p.Notes, &p.ApprovedAt,
		&p.CreatedAt, &p.UpdatedAt,
	)
}

// Create inserts a new proof in pending status.
func (r *PostgresBankTransferProofRepository) Create(ctx context.Context, proof *domain.BankTransferProof) error {
	row := r.db.QueryRow(ctx, `
		INSERT INTO bank_transfer_proofs
		      (user_id, amount_cents, currency, storage_key, content_type, file_size)
		VALUES ($1,     $2,          $3,       $4,          $5,           $6)
		RETURNING `+bankTransferColumns,
		proof.UserID, proof.AmountCents, proof.Currency,
		proof.StorageKey, proof.ContentType, proof.FileSize,
	)
	result, err := scanBankTransferProof(row)
	if err != nil {
		return err
	}
	*proof = *result
	return nil
}

// GetByID returns the proof or nil, nil when not found.
func (r *PostgresBankTransferProofRepository) GetByID(ctx context.Context, id int) (*domain.BankTransferProof, error) {
	return scanBankTransferProof(r.db.QueryRow(ctx,
		`SELECT `+bankTransferColumns+` FROM bank_transfer_proofs WHERE id = $1`, id,
	))
}

// ListByUser returns all proofs for a user ordered by created_at DESC.
func (r *PostgresBankTransferProofRepository) ListByUser(ctx context.Context, userID int) ([]*domain.BankTransferProof, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bankTransferColumns+` FROM bank_transfer_proofs WHERE user_id = $1 ORDER BY created_at DESC`,
		userID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, scanBankTransferProofRows)
}

// ListPending returns all pending proofs ordered by created_at ASC.
func (r *PostgresBankTransferProofRepository) ListPending(ctx context.Context) ([]*domain.BankTransferProof, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+bankTransferColumns+` FROM bank_transfer_proofs WHERE status = 'pending' ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, scanBankTransferProofRows)
}

// ApproveAndCredit atomically approves the proof and credits the user's balance.
func (r *PostgresBankTransferProofRepository) ApproveAndCredit(ctx context.Context, id, reviewerID int, notes string) (*domain.BankTransferProof, error) {
	var proof *domain.BankTransferProof
	err := withTx(ctx, r.db, "BankTransferProofRepository.ApproveAndCredit", func(tx pgx.Tx) error {
		// Approve the proof and capture the user_id + amount for the balance update.
		var userID, amountCents int
		row := tx.QueryRow(ctx, `
			UPDATE bank_transfer_proofs
			   SET status      = 'approved',
			       reviewed_by = $2,
			       notes       = $3,
			       approved_at = NOW(),
			       updated_at  = NOW()
			 WHERE id = $1 AND status = 'pending'
			 RETURNING `+bankTransferColumns,
			id, reviewerID, notes,
		)
		p := &domain.BankTransferProof{}
		scanErr := row.Scan(
			&p.ID, &p.UserID, &p.AmountCents, &p.Currency,
			&p.StorageKey, &p.ContentType, &p.FileSize,
			&p.Status, &p.ReviewedBy, &p.Notes, &p.ApprovedAt,
			&p.CreatedAt, &p.UpdatedAt,
		)
		if scanErr == pgx.ErrNoRows {
			return nil // handled below via notFoundOrConflictBankTransfer
		}
		if scanErr != nil {
			return apperrors.Internal(scanErr)
		}
		proof = p
		userID = p.UserID
		amountCents = p.AmountCents

		// Credit the user's balance.
		var balanceAfter int
		if err := tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents + $2,
			       updated_at    = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, userID, amountCents).Scan(&balanceAfter); err != nil {
			return apperrors.Internal(err)
		}

		refID := p.ID
		refType := "bank_transfer_proof"
		return insertLedgerTx(ctx, tx, userID, amountCents, domain.LedgerKindBankTransfer,
			balanceAfter, refID, refType, reviewerID)
	})
	if err != nil {
		return nil, err
	}
	if proof == nil {
		return r.notFoundOrConflictBankTransfer(ctx, id, "approved")
	}
	return proof, nil
}

// Reject transitions a pending proof to rejected.
func (r *PostgresBankTransferProofRepository) Reject(ctx context.Context, id, reviewerID int, notes string) (*domain.BankTransferProof, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE bank_transfer_proofs
		   SET status      = 'rejected',
		       reviewed_by = $2,
		       notes       = $3,
		       updated_at  = NOW()
		 WHERE id = $1 AND status = 'pending'
		 RETURNING `+bankTransferColumns,
		id, reviewerID, notes,
	)
	result, err := scanBankTransferProof(row)
	if err != nil {
		return nil, err
	}
	if result != nil {
		return result, nil
	}
	return r.notFoundOrConflictBankTransfer(ctx, id, "rejected")
}

func (r *PostgresBankTransferProofRepository) notFoundOrConflictBankTransfer(ctx context.Context, id int, targetStatus string) (*domain.BankTransferProof, error) {
	existing, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, apperrors.NotFound(msgBankTransferNotFound)
	}
	if string(existing.Status) == targetStatus {
		return existing, nil
	}
	return nil, apperrors.Conflict(fmt.Sprintf("bank transfer proof already %s", existing.Status))
}

var _ BankTransferProofRepository = (*PostgresBankTransferProofRepository)(nil)
