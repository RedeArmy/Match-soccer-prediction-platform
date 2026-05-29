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

const errKYCProfileNotFound = "kyc profile not found"

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
			submission_ip, submitted_at
		) VALUES (
			$1, $2, $3,
			$4, $5, $6,
			$7, $8,
			$9, $10, $11, $12,
			$13::inet, $14
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
			submission_ip    = COALESCE(kyc_profiles.submission_ip, EXCLUDED.submission_ip),
			submitted_at     = EXCLUDED.submitted_at,
			updated_at       = NOW()
		RETURNING id, created_at, updated_at
	`
	var ip *string
	if p.SubmissionIP != nil && *p.SubmissionIP != "" {
		ip = p.SubmissionIP
	}
	return r.db.QueryRow(ctx, q,
		p.UserID, string(p.Status), int(p.Tier),
		p.FullName, p.DateOfBirth, p.Nationality,
		p.DocumentType, p.DocumentNumber,
		p.AddressLine, p.City, p.Country, p.PostalCode,
		ip, p.SubmittedAt,
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
		return apperrors.NotFound(errKYCProfileNotFound)
	}
	return nil
}

// UpdateStatusWithEvent atomically transitions the profile's status and inserts
// a kyc_events audit row, ensuring no crash between the two writes leaves a
// compliance gap. oldStatus is the pre-transition value, already read by the caller.
func (r *PostgresKYCProfileRepository) UpdateStatusWithEvent(ctx context.Context, profileID, adminID int, ev KYCStatusEvent) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "KYCProfileRepository.UpdateStatusWithEvent", func(tx pgx.Tx) error {
		rejectionReason := ""
		if ev.NewStatus == domain.KYCStatusRejected {
			rejectionReason = ev.Reason
		}
		var reviewer *int
		if adminID != 0 {
			reviewer = &adminID
		}
		tag, err := tx.Exec(ctx, `
			UPDATE kyc_profiles
			   SET status           = $2,
			       reviewed_at      = NOW(),
			       reviewed_by      = $3,
			       rejection_reason = $4,
			       updated_at       = NOW()
			 WHERE id = $1
		`, profileID, string(ev.NewStatus), reviewer, rejectionReason)
		if err != nil {
			return apperrors.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.NotFound(errKYCProfileNotFound)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO kyc_events
			      (profile_id, profile_type, event_type, actor_id, old_status, new_status, reason, trace_id)
			VALUES ($1, 'user', $2, $3, $4, $5, $6, $7)
		`, profileID, string(ev.EventType), reviewer, string(ev.OldStatus), string(ev.NewStatus), ev.Reason, ev.TraceID)
		if err != nil {
			return apperrors.Internal(err)
		}
		return nil
	})
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
			return apperrors.NotFound(errKYCProfileNotFound)
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
		return apperrors.NotFound(errKYCProfileNotFound)
	}
	return nil
}

// ReleaseAndCreditFrozen atomically:
//  1. Locks the profile row and reads frozen_amount_cents.
//  2. Credits users.balance_cents by that amount.
//  3. Inserts a balance_ledger row (kind=prize).
//  4. Clears the freeze columns on kyc_profiles.
//
// Returns 0, nil when balance_frozen is already false (idempotent).
func (r *PostgresKYCProfileRepository) ReleaseAndCreditFrozen(ctx context.Context, userID int, refID int64, refType string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	var credited int
	err := withTx(ctx, r.db, "KYCProfileRepository.ReleaseAndCreditFrozen", func(tx pgx.Tx) error {
		var profileID, frozenCents int
		err := tx.QueryRow(ctx, `
			SELECT id, frozen_amount_cents
			  FROM kyc_profiles
			 WHERE user_id = $1 AND balance_frozen = TRUE
			 FOR UPDATE
		`, userID).Scan(&profileID, &frozenCents)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already unfrozen — idempotent no-op
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		if frozenCents == 0 {
			// Mark unfrozen but nothing to credit.
			_, err = tx.Exec(ctx, `
				UPDATE kyc_profiles
				   SET balance_frozen = FALSE, frozen_amount_cents = 0,
				       frozen_reason = '', updated_at = NOW()
				 WHERE user_id = $1
			`, userID)
			return err
		}
		var balanceAfter int
		err = tx.QueryRow(ctx, `
			UPDATE users
			   SET balance_cents = balance_cents + $2, updated_at = NOW()
			 WHERE id = $1 AND deleted_at IS NULL
			 RETURNING balance_cents
		`, userID, frozenCents).Scan(&balanceAfter)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound("user not found")
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		if err := insertLedgerTx(ctx, tx, ledgerRow{
			UserID:       userID,
			DeltaCents:   frozenCents,
			Kind:         domain.LedgerKindPrize,
			BalanceAfter: balanceAfter,
			RefID:        refID,
			RefType:      refType,
		}); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `
			UPDATE kyc_profiles
			   SET balance_frozen = FALSE, frozen_amount_cents = 0,
			       frozen_reason = '', updated_at = NOW()
			 WHERE user_id = $1
		`, userID)
		if err != nil {
			return apperrors.Internal(err)
		}
		credited = frozenCents
		return nil
	})
	return credited, err
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

func (r *PostgresKYCProfileRepository) CountRecentSubmissionsByIP(ctx context.Context, ip string, since time.Time) (int64, error) {
	if ip == "" {
		return 0, nil
	}
	ctx, cancel := context.WithTimeout(ctx, dbReadTimeout)
	defer cancel()
	var n int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM kyc_profiles
		  WHERE submission_ip = $1::inet
		    AND submitted_at >= $2`,
		ip, since,
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
	  ip_velocity AS (
	    SELECT COUNT(*) AS n FROM kyc_events WHERE event_type = 'ip_velocity_flag'
	  ),
	  tiers AS (
	    SELECT tier, COUNT(*) AS cnt FROM kyc_profiles GROUP BY tier
	  )
	SELECT q.n, a.secs, fs.total, f.pep, f.sanctions, iv.n
	  FROM queue q, avg_review a, frozen_sum fs, fraud f, ip_velocity iv`

	var s domain.KYCRiskDashboardStats
	var avgSecs float64
	err := r.db.QueryRow(ctx, q).Scan(&s.QueueDepth, &avgSecs, &s.FrozenBalanceTotalCents, &s.PEPFlagCount, &s.SanctionsFlagCount, &s.IPVelocityFlagCount)
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
	next_review_at, submission_ip, created_at, updated_at`

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
		&p.NextReviewAt, &p.SubmissionIP, &p.CreatedAt, &p.UpdatedAt,
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

// ApproveAndSetTier atomically approves the KYC profile, updates both tier
// columns (kyc_profiles.tier and users.kyc_tier), and inserts the audit event —
// all in a single transaction. A process kill between the three writes leaves
// no split state: either all three succeed or none is committed.
func (r *PostgresKYCProfileRepository) ApproveAndSetTier(ctx context.Context, profileID, adminID int, p KYCApprovalParams) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "KYCProfileRepository.ApproveAndSetTier", func(tx pgx.Tx) error {
		// 1. Update status, tier, and review metadata on kyc_profiles.
		var userID int
		err := tx.QueryRow(ctx, `
			UPDATE kyc_profiles
			   SET status           = 'approved',
			       tier             = $2,
			       next_review_at   = $3,
			       reviewed_by      = $4,
			       reviewed_at      = NOW(),
			       rejection_reason = '',
			       updated_at       = NOW()
			 WHERE id = $1
			 RETURNING user_id
		`, profileID, int(p.Tier), p.NextReview, adminID).Scan(&userID)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound(errKYCProfileNotFound)
		}
		if err != nil {
			return apperrors.Internal(err)
		}

		// 2. Propagate tier to the denormalised users.kyc_tier column.
		tag, err := tx.Exec(ctx,
			`UPDATE users SET kyc_tier = $2, updated_at = NOW() WHERE id = $1`,
			userID, int(p.Tier),
		)
		if err != nil {
			return apperrors.Internal(err)
		}
		if tag.RowsAffected() == 0 {
			return apperrors.NotFound("user not found for KYC tier update")
		}

		// 3. Append the approval event to the immutable audit trail.
		_, err = tx.Exec(ctx, `
			INSERT INTO kyc_events
			      (profile_id, profile_type, event_type, actor_id, old_status, new_status, reason, metadata, trace_id)
			VALUES ($1, 'user', 'approved', $2, $3, 'approved', $4, $5::jsonb, $6)
		`, profileID, adminID, string(p.OldStatus), p.Reason,
			`{"source":"ApproveAndSetTier"}`, p.TraceID,
		)
		if err != nil {
			return apperrors.Internal(err)
		}
		return nil
	})
}

// FreezeAtomic atomically sets balance_frozen=TRUE on kyc_profiles and inserts
// a 'frozen' kyc_events audit row within a single transaction, closing the
// window where a crash between the two writes left the freeze recorded but
// the compliance trail missing.
func (r *PostgresKYCProfileRepository) FreezeAtomic(ctx context.Context, userID, amountCents int, reason, traceID string) error {
	return r.FreezeAtomicWithTxHook(ctx, userID, amountCents, reason, traceID, func(_ context.Context, _ pgx.Tx) error { return nil })
}

// FreezeAtomicWithTxHook is like FreezeAtomic but calls hook(ctx, tx) within
// the same transaction after the freeze and audit-event writes, before commit.
// This closes the crash window between the freeze commit and an outbox insert
// made by the caller: either both succeed or neither does.
func (r *PostgresKYCProfileRepository) FreezeAtomicWithTxHook(ctx context.Context, userID, amountCents int, reason, traceID string, hook func(context.Context, pgx.Tx) error) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "KYCProfileRepository.FreezeAtomicWithTxHook", func(tx pgx.Tx) error {
		var profileID int
		var status string
		err := tx.QueryRow(ctx, `
			UPDATE kyc_profiles
			   SET balance_frozen      = TRUE,
			       frozen_amount_cents = $2,
			       frozen_reason       = $3,
			       updated_at          = NOW()
			 WHERE user_id = $1
			RETURNING id, status
		`, userID, amountCents, reason).Scan(&profileID, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.NotFound(errKYCProfileNotFound)
		}
		if err != nil {
			return apperrors.Internal(err)
		}
		metadata := fmt.Sprintf(`{"frozen_amount_cents":%d}`, amountCents)
		_, err = tx.Exec(ctx, `
			INSERT INTO kyc_events
			      (profile_id, profile_type, event_type, old_status, new_status, reason, metadata, trace_id)
			VALUES ($1, 'user', 'frozen', $2, $2, $3, $4::jsonb, $5)
		`, profileID, status, reason, metadata, traceID)
		if err != nil {
			return apperrors.Internal(err)
		}
		return hook(ctx, tx)
	})
}

// EnsureStub inserts a minimal kyc_profiles row for userID if one does not
// already exist. All columns carry their schema defaults. The call is
// idempotent: ON CONFLICT (user_id) DO NOTHING means it is safe to call
// during both user registration and backfill migrations without overwriting
// any existing profile data.
func (r *PostgresKYCProfileRepository) EnsureStub(ctx context.Context, userID int) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	_, err := r.db.Exec(ctx, `
		INSERT INTO kyc_profiles (user_id)
		VALUES ($1)
		ON CONFLICT (user_id) DO NOTHING
	`, userID)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

var _ KYCProfileRepository = (*PostgresKYCProfileRepository)(nil)
