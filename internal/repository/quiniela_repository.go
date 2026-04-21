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

const quinielaColumns = "id, name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency, max_members, prize_threshold, status, created_at, updated_at, deleted_at"

const msgQuinielaNotFound = "quiniela not found"

func scanQuiniela(row pgx.Row) (*domain.Quiniela, error) {
	q := &domain.Quiniela{}
	err := row.Scan(
		&q.ID, &q.Name, &q.OwnerID, &q.InviteCode, &q.InviteCodeExpiresAt,
		&q.EntryFee, &q.Currency, &q.MaxMembers, &q.PrizeThreshold, &q.Status,
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
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	qRow := tx.QueryRow(ctx,
		`INSERT INTO quinielas (name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency, max_members, prize_threshold)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+quinielaColumns,
		q.Name, q.OwnerID, q.InviteCode, q.InviteCodeExpiresAt, q.EntryFee, q.Currency, q.MaxMembers, q.PrizeThreshold,
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
	mRow := tx.QueryRow(ctx,
		`INSERT INTO group_memberships (quiniela_id, user_id, status, paid, joined_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING `+membershipColumns,
		m.QuinielaID, m.UserID, m.Status, m.Paid, m.JoinedAt,
	)
	mResult, err := scanMembership(mRow)
	if err != nil {
		return err
	}
	*m = *mResult

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *PostgresQuinielaRepository) Create(ctx context.Context, q *domain.Quiniela) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO quinielas (name, owner_id, invite_code, invite_code_expires_at, entry_fee, currency, max_members, prize_threshold)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING `+quinielaColumns,
		q.Name, q.OwnerID, q.InviteCode, q.InviteCodeExpiresAt, q.EntryFee, q.Currency, q.MaxMembers, q.PrizeThreshold,
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
	// The expiry check is enforced here at the persistence layer — not only in
	// the service — so that any future caller of this method gets consistent
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
			&q.EntryFee, &q.Currency, &q.MaxMembers, &q.PrizeThreshold, &q.Status,
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
