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

// PostgresKYBRepository is the PostgreSQL-backed implementation of KYBRepository.
type PostgresKYBRepository struct {
	db *pgxpool.Pool
}

// NewPostgresKYBRepository constructs a PostgresKYBRepository.
func NewPostgresKYBRepository(db *pgxpool.Pool) *PostgresKYBRepository {
	return &PostgresKYBRepository{db: db}
}

// Create inserts a new KYB profile.
func (r *PostgresKYBRepository) Create(ctx context.Context, p *domain.KYBProfile) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	const q = `
		INSERT INTO kyb_profiles (
			user_id, status, tier,
			legal_name, tax_id, registration_number, jurisdiction,
			incorporation_date, ubo_name, ubo_document_number,
			submitted_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6, $7,
			$8, $9, $10,
			$11
		)
		RETURNING id, created_at, updated_at
	`
	return r.db.QueryRow(ctx, q,
		p.UserID, string(p.Status), int(p.Tier),
		p.LegalName, p.TaxID, p.RegistrationNumber, p.Jurisdiction,
		p.IncorporationDate, p.UBOName, p.UBODocumentNumber,
		p.SubmittedAt,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
}

// GetByUserID returns the KYB profile for userID, or nil when none exists.
func (r *PostgresKYBRepository) GetByUserID(ctx context.Context, userID int) (*domain.KYBProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx, kybSelectAll+` WHERE user_id = $1`, userID)
	p, err := scanKYBProfile(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return p, nil
}

// GetByID returns the KYB profile by primary key, or nil when not found.
func (r *PostgresKYBRepository) GetByID(ctx context.Context, id int) (*domain.KYBProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	row := r.db.QueryRow(ctx, kybSelectAll+` WHERE id = $1`, id)
	p, err := scanKYBProfile(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return p, nil
}

// UpdateStatus transitions the KYB profile's status.
func (r *PostgresKYBRepository) UpdateStatus(ctx context.Context, id int, status domain.KYCStatus, reviewerID int, reason string) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var reviewer *int
	if reviewerID != 0 {
		reviewer = &reviewerID
	}
	tag, err := r.db.Exec(ctx, `
		UPDATE kyb_profiles
		   SET status           = $2,
		       reviewed_at      = NOW(),
		       reviewed_by      = $3,
		       rejection_reason = $4,
		       updated_at       = NOW()
		 WHERE id = $1
	`, id, string(status), reviewer, reason)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("kyb profile not found")
	}
	return nil
}

// ListPending returns profiles in pending or under_review state, ordered by
// submitted_at ASC.
func (r *PostgresKYBRepository) ListPending(ctx context.Context, limit, offset int) ([]*domain.KYBProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = kybSelectAll + `
		WHERE status IN ('pending','under_review','escalated')
		ORDER BY submitted_at ASC NULLS LAST
		LIMIT $1 OFFSET $2
	`
	rows, err := r.db.Query(ctx, q, limit, offset)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return collectRows(rows, func(r pgx.Rows) (*domain.KYBProfile, error) {
		return scanKYBProfile(r)
	})
}

// CountByStatus returns the count of profiles for each KYCStatus.
func (r *PostgresKYBRepository) CountByStatus(ctx context.Context) (map[domain.KYCStatus]int64, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	rows, err := r.db.Query(ctx, `SELECT status, COUNT(*) FROM kyb_profiles GROUP BY status`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	result := make(map[domain.KYCStatus]int64)
	for rows.Next() {
		var status string
		var count int64
		if err := rows.Scan(&status, &count); err != nil {
			return nil, apperrors.Internal(err)
		}
		result[domain.KYCStatus(status)] = count
	}
	return result, rows.Err()
}

// ExistsByTaxIDAndJurisdiction checks whether a non-rejected profile with the
// given tax_id + jurisdiction already exists for a different user.
func (r *PostgresKYBRepository) ExistsByTaxIDAndJurisdiction(ctx context.Context, taxID, jurisdiction string, excludeUserID int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM kyb_profiles
			 WHERE tax_id = $1
			   AND jurisdiction = $2
			   AND user_id != $3
			   AND status NOT IN ('rejected','unverified')
		)
	`, taxID, jurisdiction, excludeUserID).Scan(&exists)
	if err != nil {
		return false, apperrors.Internal(err)
	}
	return exists, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

const kybCols = `
	id, user_id, status, tier,
	legal_name, tax_id, registration_number, jurisdiction,
	incorporation_date, ubo_name, ubo_document_number,
	submitted_at, reviewed_at, reviewed_by, rejection_reason,
	created_at, updated_at`

const kybSelectAll = `SELECT` + kybCols + ` FROM kyb_profiles`

func scanKYBProfile(s rowScanner) (*domain.KYBProfile, error) {
	p := &domain.KYBProfile{}
	var status string
	var tier int
	var reviewedBy *int
	var reviewedAt *time.Time
	err := s.Scan(
		&p.ID, &p.UserID, &status, &tier,
		&p.LegalName, &p.TaxID, &p.RegistrationNumber, &p.Jurisdiction,
		&p.IncorporationDate, &p.UBOName, &p.UBODocumentNumber,
		&p.SubmittedAt, &reviewedAt, &reviewedBy, &p.RejectionReason,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, pgx.ErrNoRows
		}
		return nil, err
	}
	p.Status = domain.KYCStatus(status)
	p.Tier = domain.KYCTier(tier)
	p.ReviewedAt = reviewedAt
	p.ReviewedBy = reviewedBy
	return p, nil
}

var _ KYBRepository = (*PostgresKYBRepository)(nil)
