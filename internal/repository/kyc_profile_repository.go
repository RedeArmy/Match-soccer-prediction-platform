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
func (r *PostgresKYCProfileRepository) UpdateTier(ctx context.Context, userID int, tier domain.KYCTier, nextReviewAt *time.Time) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "KYCProfileRepository.UpdateTier", func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE kyc_profiles
			   SET tier           = $2,
			       next_review_at = COALESCE($3, next_review_at),
			       updated_at     = NOW()
			 WHERE user_id = $1
		`, userID, int(tier), nextReviewAt)
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

func (r *PostgresKYCProfileRepository) CountReviewQueue(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM kyc_profiles WHERE status = 'under_review'`).Scan(&n)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return n, nil
}

func (r *PostgresKYCProfileRepository) SumFrozenAmountCents(ctx context.Context) (int64, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var total int64
	err := r.db.QueryRow(ctx, `SELECT COALESCE(SUM(frozen_amount_cents), 0) FROM kyc_profiles WHERE balance_frozen = TRUE`).Scan(&total)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return total, nil
}

func (r *PostgresKYCProfileRepository) CountAccountsByDeviceFingerprint(ctx context.Context, fingerprint string, excludeUserID int) (int64, error) {
	if fingerprint == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var n int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM kyc_profiles
		  WHERE device_fingerprint = $1
		    AND user_id <> $2
		    AND status NOT IN ('rejected', 'unverified')`,
		fingerprint, excludeUserID,
	).Scan(&n)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return n, nil
}

func (r *PostgresKYCProfileRepository) UpdateRiskScore(ctx context.Context, profileID, score int) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	_, err := r.db.Exec(ctx,
		`UPDATE kyc_profiles SET risk_score = $1, updated_at = NOW() WHERE id = $2`,
		score, profileID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *PostgresKYCProfileRepository) ExistsByDocumentIdentity(ctx context.Context, documentType domain.KYCDocumentType, documentNumber string, dateOfBirth *time.Time, excludeUserID int) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	const q = `
	SELECT EXISTS (
	  SELECT 1 FROM kyc_profiles
	   WHERE document_type   = $1
	     AND document_number = $2
	     AND date_of_birth IS NOT DISTINCT FROM $3
	     AND user_id        <> $4
	     AND status NOT IN ('rejected', 'unverified')
	)`
	var exists bool
	err := r.db.QueryRow(ctx, q, string(documentType), documentNumber, dateOfBirth, excludeUserID).Scan(&exists)
	if err != nil {
		return false, apperrors.Internal(err)
	}
	return exists, nil
}

func (r *PostgresKYCProfileRepository) RiskDashboardStats(ctx context.Context) (*domain.KYCRiskDashboardStats, error) {
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()

	const q = `
	WITH
	  queue AS (
	    SELECT COUNT(*) AS n FROM kyc_profiles WHERE status = 'under_review'
	  ),
	  avg_review AS (
	    SELECT COALESCE(AVG(EXTRACT(EPOCH FROM (reviewed_at - submitted_at))), 0) AS secs
	      FROM kyc_profiles
	     WHERE reviewed_at IS NOT NULL AND submitted_at IS NOT NULL
	  ),
	  frozen_sum AS (
	    SELECT COALESCE(SUM(frozen_amount_cents), 0) AS total FROM kyc_profiles WHERE balance_frozen = TRUE
	  ),
	  fraud AS (
	    SELECT
	      COUNT(*) FILTER (WHERE pep_flag      = TRUE) AS pep,
	      COUNT(*) FILTER (WHERE sanctions_flag = TRUE) AS sanctions
	    FROM kyc_profiles
	  ),
	  tiers AS (
	    SELECT tier, COUNT(*) AS cnt FROM kyc_profiles GROUP BY tier
	  )
	SELECT q.n, a.secs, fs.total, f.pep, f.sanctions
	  FROM queue q, avg_review a, frozen_sum fs, fraud f`

	var s domain.KYCRiskDashboardStats
	var avgSecs float64
	err := r.db.QueryRow(ctx, q).Scan(&s.QueueDepth, &avgSecs, &s.FrozenBalanceTotalCents, &s.PEPFlagCount, &s.SanctionsFlagCount)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	s.AvgReviewTimeSecs = avgSecs

	// tier distribution — second query to avoid complex unnesting
	trows, terr := r.db.Query(ctx, `SELECT tier, COUNT(*) FROM kyc_profiles GROUP BY tier`)
	if terr != nil {
		return nil, apperrors.Internal(terr)
	}
	defer trows.Close()
	s.TierDistribution = make(map[domain.KYCTier]int64)
	for trows.Next() {
		var tier int
		var cnt int64
		if err := trows.Scan(&tier, &cnt); err != nil {
			return nil, apperrors.Internal(err)
		}
		s.TierDistribution[domain.KYCTier(tier)] = cnt
	}
	if err := trows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return &s, nil
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
