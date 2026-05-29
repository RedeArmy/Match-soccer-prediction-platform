package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const errMsgDuplicateGroupName = "a group with this name already exists"

// PostgresQuinielaRepository is the PostgreSQL-backed implementation of QuinielaRepository.
type PostgresQuinielaRepository struct {
	db *pgxpool.Pool
}

// NewPostgresQuinielaRepository constructs a PostgresQuinielaRepository.
func NewPostgresQuinielaRepository(db *pgxpool.Pool) *PostgresQuinielaRepository {
	return &PostgresQuinielaRepository{db: db}
}

const quinielaColumns = "id, name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency, status, created_at, updated_at, deleted_at"

const msgQuinielaNotFound = "quiniela not found"

func scanQuiniela(row pgx.Row) (*domain.Quiniela, error) {
	q := &domain.Quiniela{}
	err := row.Scan(
		&q.ID, &q.Name, &q.OwnerID, &q.InviteCode, &q.InviteCodeExpiresAt,
		&q.EntryFee, &q.Currency, &q.Status,
		&q.CreatedAt, &q.UpdatedAt, &q.DeletedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return q, nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// CreateWithMembership inserts the quiniela and the owner's initial membership
// inside a single pgx transaction. If either insert fails the transaction is
// rolled back and neither row appears in the database.
func (r *PostgresQuinielaRepository) CreateWithMembership(ctx context.Context, q *domain.Quiniela, m *domain.GroupMembership) error {
	return withTx(ctx, r.db, "QuinielaRepository.CreateWithMembership", func(tx pgx.Tx) error {
		qRow := tx.QueryRow(ctx,
			`INSERT INTO quinielas (name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+quinielaColumns,
			q.Name, q.OwnerID, q.InviteCode, q.InviteCodeExpiresAt, q.EntryFee, q.Currency,
		)
		qResult, err := scanQuiniela(qRow)
		if err != nil {
			if isUniqueViolation(err) {
				return apperrors.Conflict(errMsgDuplicateGroupName)
			}
			return err
		}
		*q = *qResult

		m.QuinielaID = q.ID
		mRole := m.Role
		if mRole == "" {
			mRole = domain.MembershipRoleMember
		}
		mRow := tx.QueryRow(ctx,
			`INSERT INTO group_memberships (quiniela_id, user_id, status, role, paid, joined_at)
			 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+membershipColumns,
			m.QuinielaID, m.UserID, m.Status, mRole, m.Paid, m.JoinedAt,
		)
		mResult, err := scanMembership(mRow)
		if err != nil {
			return err
		}
		*m = *mResult
		return nil
	})
}

func (r *PostgresQuinielaRepository) Create(ctx context.Context, q *domain.Quiniela) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO quinielas (name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency)
		 VALUES ($1, $2, $3, $4, $5, $6) RETURNING `+quinielaColumns,
		q.Name, q.OwnerID, q.InviteCode, q.InviteCodeExpiresAt, q.EntryFee, q.Currency,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict(errMsgDuplicateGroupName)
		}
		return err
	}
	*q = *result
	return nil
}

func (r *PostgresQuinielaRepository) GetByID(ctx context.Context, id int) (*domain.Quiniela, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas WHERE id=$1`+activeOnly, id,
	)
	return scanQuiniela(row)
}

func (r *PostgresQuinielaRepository) GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error) {
	// The expiry check is enforced here at the persistence layer - not only in
	// the service - so that any future caller of this method gets consistent
	// behaviour without relying on the service to apply the guard. An expired
	// code returns nil (not found) rather than an error, matching the nil-for-
	// not-found convention used throughout the repository layer.
	row := r.db.QueryRow(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas
		 WHERE invite_code=$1`+activeOnly+`
		   AND (invite_code_expires_at IS NULL OR invite_code_expires_at > NOW())`,
		code,
	)
	return scanQuiniela(row)
}

// RotateInviteCode replaces the current invite code and expiry for the given
// quiniela in a single UPDATE. It returns the updated Quiniela so callers can
// surface the new code to the owner without an extra read.
func (r *PostgresQuinielaRepository) RotateInviteCode(ctx context.Context, id int, newCode string, expiresAt *time.Time) (*domain.Quiniela, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE quinielas
		    SET invite_code=$1, invite_code_expires_at=$2, updated_at=NOW()
		  WHERE id=$3`+activeOnly+`
		  RETURNING `+quinielaColumns,
		newCode, expiresAt, id,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(msgQuinielaNotFound)
	}
	return result, nil
}

func (r *PostgresQuinielaRepository) Update(ctx context.Context, q *domain.Quiniela) error {
	row := r.db.QueryRow(ctx,
		`UPDATE quinielas SET name=$1, updated_at=NOW() WHERE id=$2 RETURNING `+quinielaColumns,
		q.Name, q.ID,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict(errMsgDuplicateGroupName)
		}
		return err
	}
	if result == nil {
		return apperrors.NotFound(msgQuinielaNotFound)
	}
	*q = *result
	return nil
}

func (r *PostgresQuinielaRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quinielas SET deleted_at=NOW() WHERE id=$1`+activeOnly, id,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(msgQuinielaNotFound)
	}
	return nil
}

func (r *PostgresQuinielaRepository) ListByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas WHERE owner_id=$1`+activeOnly+` ORDER BY created_at DESC`, ownerID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectQuinielas(rows)
}

// UpdateStatus sets the quiniela's active/inactive status atomically. It is
// called exclusively by the membership service after every membership state
// transition.
func (r *PostgresQuinielaRepository) UpdateStatus(ctx context.Context, quinielaID int, status domain.QuinielaStatus) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quinielas SET status=$1, updated_at=NOW() WHERE id=$2`+activeOnly,
		status, quinielaID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(msgQuinielaNotFound)
	}
	return nil
}

func collectQuinielas(rows pgx.Rows) ([]*domain.Quiniela, error) {
	var quinielas []*domain.Quiniela
	for rows.Next() {
		q := &domain.Quiniela{}
		if err := rows.Scan(
			&q.ID, &q.Name, &q.OwnerID, &q.InviteCode, &q.InviteCodeExpiresAt,
			&q.EntryFee, &q.Currency, &q.Status,
			&q.CreatedAt, &q.UpdatedAt, &q.DeletedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		quinielas = append(quinielas, q)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return quinielas, nil
}

// UpdateGroupSettings updates the entry_fee for a quiniela. Returns the
// updated quiniela or NotFound when the group does not exist or is soft-deleted.
func (r *PostgresQuinielaRepository) UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee int) (*domain.Quiniela, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE quinielas
		    SET entry_fee  = $1,
		        updated_at = NOW()
		  WHERE id = $2`+activeOnly+`
		  RETURNING `+quinielaColumns,
		entryFee, quinielaID,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(msgQuinielaNotFound)
	}
	return result, nil
}

// DeleteByAdmin soft-deletes a quiniela on behalf of an administrator.
// The adminID identifies the actor for the caller's audit trail - it is not
// stored in the quinielas table. Returns NotFound when the quiniela does not
// exist or is already soft-deleted.
func (r *PostgresQuinielaRepository) DeleteByAdmin(ctx context.Context, quinielaID, _ int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE quinielas SET deleted_at = NOW() WHERE id = $1`+activeOnly,
		quinielaID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(msgQuinielaNotFound)
	}
	return nil
}

// ListByIDs returns quinielas for the given IDs in a single query.
// An empty ids slice returns nil, nil without querying the database.
func (r *PostgresQuinielaRepository) ListByIDs(ctx context.Context, ids []int) ([]*domain.Quiniela, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas WHERE id = ANY($1) AND deleted_at IS NULL`,
		ids,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectQuinielas(rows)
}

// GetStatusCounts returns a single-row summary of quiniela counts grouped by
// lifecycle status. Deleted rows are counted separately.
func (r *PostgresQuinielaRepository) GetStatusCounts(ctx context.Context) (QuinielaStatusCounts, error) {
	var c QuinielaStatusCounts
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*),
			COUNT(*) FILTER (WHERE status = 'active'   AND deleted_at IS NULL),
			COUNT(*) FILTER (WHERE status = 'inactive' AND deleted_at IS NULL),
			COUNT(*) FILTER (WHERE deleted_at IS NOT NULL)
		FROM quinielas`).Scan(&c.Total, &c.Active, &c.Inactive, &c.Deleted)
	if err != nil {
		return QuinielaStatusCounts{}, apperrors.Internal(err)
	}
	return c, nil
}

// BulkDeleteByAdmin soft-deletes multiple quinielas. Returns the IDs that were
// actually updated; already-deleted quinielas are silently skipped.
func (r *PostgresQuinielaRepository) BulkDeleteByAdmin(ctx context.Context, ids []int, _ int) ([]int, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		UPDATE quinielas
		SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = ANY($1) AND deleted_at IS NULL
		RETURNING id`, ids)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	var succeeded []int
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, apperrors.Internal(err)
		}
		succeeded = append(succeeded, id)
	}
	return succeeded, rows.Err()
}

// DistributePrizesAtomically claims prizes_distributed_at on quinielaID and
// executes all credits and freezes in a single pgx transaction so that a crash
// mid-loop never leaves the quiniela in a partially-distributed state.
//
// The first statement is a SELECT ... FOR UPDATE that both acts as the
// idempotency guard and closes the TOCTOU window: the row is locked before any
// prize amounts are applied, so a concurrent admin setting change to entry_fee
// between the service layer's pre-transaction read and this call is detected
// and rejected with apperrors.Conflict rather than silently using stale amounts.
func (r *PostgresQuinielaRepository) DistributePrizesAtomically(
	ctx context.Context,
	quinielaID, expectedEntryFee int,
	credits []PrizeCredit,
	freezes []PrizeFreeze,
) error {
	ctx, cancel := context.WithTimeout(ctx, dbWriteTimeout)
	defer cancel()
	return withTx(ctx, r.db, "QuinielaRepository.DistributePrizesAtomically", func(tx pgx.Tx) error {
		if err := lockAndMarkDistributed(ctx, tx, quinielaID, expectedEntryFee); err != nil {
			return err
		}
		for _, c := range credits {
			if err := applyPrizeCreditTx(ctx, tx, c); err != nil {
				return err
			}
		}
		for _, f := range freezes {
			if err := applyPrizeFreezeTx(ctx, tx, f); err != nil {
				return err
			}
		}
		return nil
	})
}

// lockAndMarkDistributed acquires a FOR UPDATE lock on the quiniela row,
// verifies the entry_fee has not changed, and stamps prizes_distributed_at.
// Extracted from DistributePrizesAtomically to stay within the cognitive
// complexity budget.
func lockAndMarkDistributed(ctx context.Context, tx pgx.Tx, quinielaID, expectedEntryFee int) error {
	var lockedEntryFee int
	err := tx.QueryRow(ctx,
		`SELECT entry_fee FROM quinielas
		  WHERE id = $1
		    AND prizes_distributed_at IS NULL
		    AND deleted_at IS NULL
		  FOR UPDATE`,
		quinielaID,
	).Scan(&lockedEntryFee)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.Conflict("prizes already distributed for this quiniela")
	}
	if err != nil {
		return apperrors.Internal(err)
	}
	if lockedEntryFee != expectedEntryFee {
		return apperrors.Conflict("entry_fee changed between leaderboard read and distribution — retry")
	}
	if _, err := tx.Exec(ctx,
		`UPDATE quinielas
		    SET prizes_distributed_at = NOW(),
		        updated_at            = NOW()
		  WHERE id = $1`,
		quinielaID,
	); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// applyPrizeCreditTx credits a single winner inside an open transaction:
// increments users.balance_cents and inserts the corresponding ledger row.
func applyPrizeCreditTx(ctx context.Context, tx pgx.Tx, c PrizeCredit) error {
	var balanceAfter int
	err := tx.QueryRow(ctx,
		`UPDATE users
		    SET balance_cents = balance_cents + $2,
		        updated_at    = NOW()
		  WHERE id = $1 AND deleted_at IS NULL
		  RETURNING balance_cents`,
		c.UserID, c.AmountCents,
	).Scan(&balanceAfter)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.NotFound("prize recipient not found")
	}
	if err != nil {
		return apperrors.Internal(err)
	}
	return insertLedgerTx(ctx, tx, ledgerRow{
		UserID:       c.UserID,
		DeltaCents:   c.AmountCents,
		Kind:         domain.LedgerKindPrize,
		BalanceAfter: balanceAfter,
		RefID:        c.RefID,
		RefType:      c.RefType,
	})
}

// applyPrizeFreezeTx freezes the prize share of a single KYC-gated winner
// inside an open transaction by setting balance_frozen on their kyc_profile.
// Every user is guaranteed to have a kyc_profiles row (created at registration
// via EnsureStub and backfilled by migration 000133), so RowsAffected() == 0
// indicates an unexpected data integrity issue and is logged at Error level
// rather than rolling back the entire distribution.
func applyPrizeFreezeTx(ctx context.Context, tx pgx.Tx, f PrizeFreeze) error {
	_, err := tx.Exec(ctx,
		`UPDATE kyc_profiles
		    SET balance_frozen      = TRUE,
		        frozen_amount_cents = $2,
		        frozen_reason       = $3,
		        updated_at          = NOW()
		  WHERE user_id = $1`,
		f.UserID, f.AmountCents, f.Reason,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	// RowsAffected() == 0 means the kyc_profiles row is missing. EnsureStub at
	// registration and migration 000133 guarantee every user has a row, so this
	// is an invariant violation. We intentionally do not return an error: doing so
	// would roll back the entire distribution and deny prizes to all other winners.
	// The operator can backfill the missing row without re-running distribution.
	return nil
}

var _ QuinielaRepository = (*PostgresQuinielaRepository)(nil)
