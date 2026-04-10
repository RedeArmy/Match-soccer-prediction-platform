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
const userColumns = "id, name, email, role, clerk_subject, created_at, updated_at, deleted_at"

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
	if err := s.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &clerkSubject, &u.CreatedAt, &u.UpdatedAt, &u.DeletedAt); err != nil {
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
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
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
		`SELECT `+userColumns+` FROM users WHERE id=$1 AND deleted_at IS NULL`, id,
	)
	return scanUser(row)
}

func (r *PostgresUserRepository) GetByClerkSubject(ctx context.Context, subject string) (*domain.User, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE clerk_subject = $1 AND deleted_at IS NULL`, subject,
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
		return apperrors.NotFound("user not found")
	}
	*u = *result
	return nil
}

func (r *PostgresUserRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users SET deleted_at=NOW() WHERE id=$1 AND deleted_at IS NULL`, id,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("user not found")
	}
	return nil
}

func (r *PostgresUserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE deleted_at IS NULL ORDER BY id ASC`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func collectUsers(rows pgx.Rows) ([]*domain.User, error) {
	var users []*domain.User
	for rows.Next() {
		// pgx.Rows satisfies rowScanner, so scanUserFields is reused directly
		// instead of duplicating the Scan call and clerkSubject unwrap here.
		u, err := scanUserFields(rows)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return users, nil
}
