package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresUserRepository is the PostgreSQL-backed implementation of UserRepository.
type PostgresUserRepository struct {
	db *pgxpool.Pool
}

// NewPostgresUserRepository constructs a PostgresUserRepository.
func NewPostgresUserRepository(db *pgxpool.Pool) *PostgresUserRepository {
	return &PostgresUserRepository{db: db}
}

// userColumns is the canonical SELECT/RETURNING column list for the users table.
// Keeping it in a single constant ensures that scanUser and every query that
// uses RETURNING stay in sync automatically when columns are added or removed.
// password_hash was removed in migration 000010: authentication is delegated
// to Clerk and no credential is stored in the application database.
const (
	userColumns     = "id, name, email, role, clerk_subject, created_at, updated_at, deleted_at, banned_at, banned_by, ban_reason"
	msgUserNotFound = "user not found"
)

// rowScanner is satisfied by both pgx.Row (single-row query) and pgx.Rows
// (multi-row iteration). Accepting this interface lets scanUserFields serve
// both callers without duplicating the Scan call and clerkSubject unwrap.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanUserFields populates a domain.User from any rowScanner. It returns the
// filled struct and a raw scan error (not wrapped). Callers are responsible for
// interpreting sentinel errors such as pgx.ErrNoRows.
func scanUserFields(s rowScanner) (*domain.User, error) {
	u := &domain.User{}
	var clerkSubject *string // nullable in DB; empty string in domain when NULL
	if err := s.Scan(
		&u.ID, &u.Name, &u.Email, &u.Role, &clerkSubject,
		&u.CreatedAt, &u.UpdatedAt, &u.DeletedAt,
		&u.BannedAt, &u.BannedBy, &u.BanReason,
	); err != nil {
		return nil, err
	}
	if clerkSubject != nil {
		u.ClerkSubject = *clerkSubject
	}
	return u, nil
}

// scanUser wraps scanUserFields for single-row queries. pgx.ErrNoRows is
// translated to (nil, nil) so callers can distinguish "not found" from a
// real database error without importing pgx in the service layer.
func scanUser(row pgx.Row) (*domain.User, error) {
	u, err := scanUserFields(row)
	if err != nil {
		return nil, singleScanErr(err)
	}
	return u, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, u *domain.User) error {
	// password_hash is no longer a column: only name, email, and role are
	// required at creation time. clerk_subject is set later via Update when
	// the Clerk webhook delivers the user.created event.
	row := r.db.QueryRow(ctx,
		`INSERT INTO users (name, email, role)
		 VALUES ($1, $2, $3)
		 RETURNING `+userColumns,
		u.Name, u.Email, u.Role,
	)
	result, err := scanUser(row)
	if err != nil {
		return err
	}
	*u = *result
	return nil
}

func (r *PostgresUserRepository) GetByID(ctx context.Context, id int) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE id=$1`+activeOnly, id,
	)
	return scanUser(row)
}

func (r *PostgresUserRepository) GetByClerkSubject(ctx context.Context, subject string) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE clerk_subject=$1`+activeOnly, subject,
	)
	return scanUser(row)
}

func (r *PostgresUserRepository) Update(ctx context.Context, u *domain.User) error {
	var clerkSubject *string
	if u.ClerkSubject != "" {
		clerkSubject = &u.ClerkSubject
	}
	// password_hash is no longer updated; only name, email, role, and
	// clerk_subject are mutable through the application layer.
	row := r.db.QueryRow(ctx,
		`UPDATE users SET name=$1, email=$2, role=$3, clerk_subject=$4, updated_at=NOW()
		 WHERE id=$5 RETURNING `+userColumns,
		u.Name, u.Email, u.Role, clerkSubject, u.ID,
	)
	result, err := scanUser(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound(msgUserNotFound)
	}
	*u = *result
	return nil
}

func (r *PostgresUserRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users SET deleted_at=NOW() WHERE id=$1`+activeOnly, id,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(msgUserNotFound)
	}
	return nil
}

func (r *PostgresUserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE `+activeFilter+` ORDER BY id ASC`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

// ListByIDs fetches all active users whose IDs are in the provided slice.
// The returned order is not guaranteed; callers must sort if needed.
// An empty ids slice returns an empty list without hitting the database.
func (r *PostgresUserRepository) ListByIDs(ctx context.Context, ids []int) ([]*domain.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE id=ANY($1) AND `+activeFilter,
		ids,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func collectUsers(rows pgx.Rows) ([]*domain.User, error) {
	return collectRows(rows, func(r pgx.Rows) (*domain.User, error) {
		// pgx.Rows satisfies rowScanner, so scanUserFields is reused directly
		// instead of duplicating the Scan call and clerkSubject unwrap here.
		return scanUserFields(r)
	})
}

// Ban sets banned_at, banned_by, and ban_reason for the given user, returning
// the updated record. If the user is already banned the ban is overwritten
// with the new details. Returns NotFound for unknown or soft-deleted users.
func (r *PostgresUserRepository) Ban(ctx context.Context, userID, adminID int, reason string) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE users
		    SET banned_at  = NOW(),
		        banned_by  = $2,
		        ban_reason = $3,
		        updated_at = NOW()
		  WHERE id = $1`+activeOnly+`
		  RETURNING `+userColumns,
		userID, adminID, reason,
	)
	result, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(msgUserNotFound)
	}
	return result, nil
}

// Unban clears the ban fields for the given user. It is idempotent: unbanning
// an already-active user succeeds silently. Returns NotFound for unknown or
// soft-deleted users.
func (r *PostgresUserRepository) Unban(ctx context.Context, userID int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users
		    SET banned_at  = NULL,
		        banned_by  = NULL,
		        ban_reason = '',
		        updated_at = NOW()
		  WHERE id = $1`+activeOnly,
		userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(msgUserNotFound)
	}
	return nil
}

// ListBanned returns all active users whose banned_at is not NULL, ordered by
// banned_at descending (most recently banned first).
func (r *PostgresUserRepository) ListBanned(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+userColumns+` FROM users WHERE banned_at IS NOT NULL AND `+activeFilter+` ORDER BY banned_at DESC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

// ListFiltered returns users matching the given filters with pagination.
// Filters are applied with AND semantics; nil fields are ignored.
func (r *PostgresUserRepository) ListFiltered(ctx context.Context, f UserFilters, p Pagination) ([]*domain.User, error) {
	q := `SELECT ` + userColumns + ` FROM users WHERE deleted_at IS NULL`
	args := []any{}
	n := 1

	if f.Banned != nil {
		if *f.Banned {
			q += ` AND banned_at IS NOT NULL`
		} else {
			q += ` AND banned_at IS NULL`
		}
	}
	if f.Role != nil {
		q += ` AND role = $` + itoa(n)
		args = append(args, string(*f.Role))
		n++
	}
	if f.Search != nil && *f.Search != "" {
		q += ` AND (name ILIKE $` + itoa(n) + ` OR email ILIKE $` + itoa(n) + `)`
		args = append(args, "%"+*f.Search+"%")
		n++
	}

	q += ` ORDER BY created_at DESC`
	q, args, _ = applyPagination(q, args, n, p)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

// GetStatusCounts returns a single-row summary of user counts grouped by
// lifecycle status. Uses conditional aggregation to avoid multiple round-trips.
func (r *PostgresUserRepository) GetStatusCounts(ctx context.Context) (UserStatusCounts, error) {
	var c UserStatusCounts
	err := r.db.QueryRow(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE deleted_at IS NULL),
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND banned_at IS NULL),
			COUNT(*) FILTER (WHERE deleted_at IS NULL AND banned_at IS NOT NULL)
		FROM users`).Scan(&c.Total, &c.Active, &c.Banned)
	if err != nil {
		return UserStatusCounts{}, apperrors.Internal(err)
	}
	return c, nil
}

var _ UserRepository = (*PostgresUserRepository)(nil)
