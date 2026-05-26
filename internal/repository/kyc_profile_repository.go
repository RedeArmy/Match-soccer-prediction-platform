package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresKYCProfileRepository is the PostgreSQL-backed implementation of KYCProfileRepository.
type PostgresKYCProfileRepository struct {
	db *pgxpool.Pool
}

// NewPostgresKYCProfileRepository constructs a PostgresKYCProfileRepository.
func NewPostgresKYCProfileRepository(db *pgxpool.Pool) *PostgresKYCProfileRepository {
	return &PostgresKYCProfileRepository{db: db}
}

// Upsert creates or updates the KYC profile for a user.
// On conflict (user_id unique constraint) the identity fields are updated in
// place and status is reset to pending so the admin queue receives the resubmission.
func (r *PostgresKYCProfileRepository) Upsert(ctx context.Context, p *domain.KYCProfile) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	const q = `
		INSERT INTO kyc_profiles (
			user_id, status, tier,
			full_name, date_of_birth, nationality,
			document_type, document_number,
			address_line, city, country, postal_code,
			submitted_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8,
			$9, $10, $11, $12,
			$13
		)
		ON CONFLICT (user_id) DO UPDATE SET
			status           = EXCLUDED.status,
			full_name        = EXCLUDED.full_name,
			date_of_birth    = EXCLUDED.date_of_birth,
			nationality      = EXCLUDED.nationality,
			document_type    = EXCLUDED.document_type,
			document_number  = EXCLUDED.document_number,
			address_line     = EXCLUDED.address_line,
			city             = EXCLUDED.city,
			country          = EXCLUDED.country,
			postal_code      = EXCLUDED.postal_code,
			submitted_at     = EXCLUDED.submitted_at,
			updated_at       = NOW()
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(ctx, q,
		p.UserID, string(p.Status), int(p.Tier),
		p.FullName, p.DateOfBirth, p.Nationality,
		p.DocumentType, p.DocumentNumber,
		p.AddressLine, p.City, p.Country, p.PostalCode,
		p.SubmittedAt,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// GetByUserID returns the KYC profile for userID, or nil when none exists.
func (r *PostgresKYCProfileRepository) GetByUserID(ctx context.Context, userID int) (*domain.KYCProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx, kycProfileSelectByUserID, userID)
	p, err := scanKYCProfile(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return p, nil
}

// GetByID returns the KYC profile by primary key, or nil when not found.
func (r *PostgresKYCProfileRepository) GetByID(ctx context.Context, id int) (*domain.KYCProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx, kycProfileSelectByID, id)
	p, err := scanKYCProfile(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return p, nil
}

// UpdateStatus transitions the profile's status and captures the review metadata.
func (r *PostgresKYCProfileRepository) UpdateStatus(ctx context.Context, profileID int, newStatus domain.KYCStatus, reviewedBy int, rejectionReason string) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var reviewer *int
	if reviewedBy != 0 {
		reviewer = &reviewedBy
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE kyc_profiles
		   SET status           = $2,
		       reviewed_at      = NOW(),
		       reviewed_by      = $3,
		       rejection_reason = $4,
		       updated_at       = NOW()
		 WHERE id = $1
	`, profileID, string(newStatus), reviewer, rejectionReason)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("kyc profile not found")
	}
	return nil
}

// UpdateTier atomically updates tier on kyc_profiles and kyc_tier on users.
func (r *PostgresKYCProfileRepository) UpdateTier(ctx context.Context, userID int, tier domain.KYCTier) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "KYCProfileRepository.UpdateTier", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE kyc_profiles
			   SET tier       = $2,
			       updated_at = NOW()
			 WHERE user_id = $1
		`, userID, int(tier))
		if err != nil {
			return apperrors.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.NotFound("kyc profile not found")
		}
		if _, err := tx.Exec(ctx, `
			UPDATE users SET kyc_tier = $2, updated_at = NOW() WHERE id = $1
		`, userID, int(tier)); err != nil {
			return apperrors.Internal(err)
		}
		return nil
	})
}

// SetFrozen sets or clears the balance-freeze columns for a user's KYC profile.
func (r *PostgresKYCProfileRepository) SetFrozen(ctx context.Context, userID int, frozen bool, amountCents int, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var amt int
	var rsn string
	if frozen {
		amt = amountCents
		rsn = reason
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE kyc_profiles
		   SET balance_frozen       = $2,
		       frozen_amount_cents  = $3,
		       frozen_reason        = $4,
		       updated_at           = NOW()
		 WHERE user_id = $1
	`, userID, frozen, amt, rsn)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("kyc profile not found")
	}
	return nil
}

// ListPending returns profiles in active review states with optional filtering.
func (r *PostgresKYCProfileRepository) ListPending(ctx context.Context, f KYCProfileFilters, p Pagination) ([]*domain.KYCProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	wb := newWhereBuilder()
	if f.Status != nil {
		wb.add("status = $%d", string(*f.Status))
	} else {
		wb.addCond("status IN ('pending','under_review','escalated')")
	}
	if f.Tier != nil {
		wb.add("tier = $%d", int(*f.Tier))
	}
	q := kycProfileSelectAll + wb.clause() + " ORDER BY submitted_at ASC NULLS LAST"
	q, args, _, err := applyPagination(q, wb.args, wb.next(), p)
	if err != nil {
		return nil, apperrors.BadRequest(err.Error())
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.KYCProfile, error) {
		return scanKYCProfile(r)
	})
}

// ListFrozen returns a summary of all accounts with balance_frozen = true.
func (r *PostgresKYCProfileRepository) ListFrozen(ctx context.Context) ([]*domain.FrozenBalanceSummary, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = `
		SELECT u.id, u.name, u.email,
		       kp.status, kp.tier,
		       kp.frozen_amount_cents, kp.frozen_reason, kp.updated_at
		  FROM kyc_profiles kp
		  JOIN users u ON u.id = kp.user_id
		 WHERE kp.balance_frozen = TRUE
		   AND u.deleted_at IS NULL
		 ORDER BY kp.updated_at ASC
	`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.FrozenBalanceSummary, error) {
		s := &domain.FrozenBalanceSummary{}
		var status string
		var tier int
		err := r.Scan(
			&s.UserID, &s.UserName, &s.UserEmail,
			&status, &tier,
			&s.FrozenAmountCents, &s.FrozenReason, &s.FrozenSince,
		)
		s.KYCStatus = domain.KYCStatus(status)
		s.KYCTier = domain.KYCTier(tier)
		return s, err
	})
}

// ListDueForReview returns approved profiles whose next_review_at is before threshold.
func (r *PostgresKYCProfileRepository) ListDueForReview(ctx context.Context, threshold time.Time) ([]*domain.KYCProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = kycProfileSelectAll + ` WHERE status = 'approved' AND next_review_at <= $1 ORDER BY next_review_at ASC`
	rows, err := r.db.Query(ctx, q, threshold)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.KYCProfile, error) {
		return scanKYCProfile(r)
	})
}

// ── helpers ───────────────────────────────────────────────────────────────────

const kycProfileCols = `
	id, user_id, status, tier,
	full_name, date_of_birth, nationality,
	document_type, document_number,
	address_line, city, country, postal_code,
	submitted_at, reviewed_at, reviewed_by, rejection_reason,
	risk_score, pep_flag, sanctions_flag,
	balance_frozen, frozen_amount_cents, frozen_reason,
	next_review_at, created_at, updated_at`

const kycProfileSelectAll = `SELECT` + kycProfileCols + ` FROM kyc_profiles`
const kycProfileSelectByUserID = kycProfileSelectAll + ` WHERE user_id = $1`
const kycProfileSelectByID = kycProfileSelectAll + ` WHERE id = $1`

func scanKYCProfile(s rowScanner) (*domain.KYCProfile, error) {
	p := &domain.KYCProfile{}
	var status string
	var tier int
	var docType *string
	err := s.Scan(
		&p.ID, &p.UserID, &status, &tier,
		&p.FullName, &p.DateOfBirth, &p.Nationality,
		&docType, &p.DocumentNumber,
		&p.AddressLine, &p.City, &p.Country, &p.PostalCode,
		&p.SubmittedAt, &p.ReviewedAt, &p.ReviewedBy, &p.RejectionReason,
		&p.RiskScore, &p.PEPFlag, &p.SanctionsFlag,
		&p.BalanceFrozen, &p.FrozenAmountCents, &p.FrozenReason,
		&p.NextReviewAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	p.Status = domain.KYCStatus(status)
	p.Tier = domain.KYCTier(tier)
	if docType != nil {
		dt := domain.KYCDocumentType(*docType)
		p.DocumentType = &dt
	}
	return p, nil
}

var _ KYCProfileRepository = (*PostgresKYCProfileRepository)(nil)
