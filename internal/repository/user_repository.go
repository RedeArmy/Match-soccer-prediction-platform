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

const userColumns = "id, name, email, password_hash, role, created_at, updated_at"

func scanUser(row pgx.Row) (*domain.User, error) {
	u := &domain.User{}
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return u, nil
}

func (r *PostgresUserRepository) Create(ctx context.Context, u *domain.User) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO users (name, email, password_hash, role)
		 VALUES ($1, $2, $3, $4)
		 RETURNING `+userColumns,
		u.Name, u.Email, u.PasswordHash, u.Role,
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
		`SELECT `+userColumns+` FROM users WHERE id=$1`, id,
	)
	return scanUser(row)
}

func (r *PostgresUserRepository) Update(ctx context.Context, u *domain.User) error {
	row := r.db.QueryRow(ctx,
		`UPDATE users SET name=$1, email=$2, password_hash=$3, role=$4, updated_at=NOW()
		 WHERE id=$5 RETURNING `+userColumns,
		u.Name, u.Email, u.PasswordHash, u.Role, u.ID,
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
	tag, err := r.db.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("user not found")
	}
	return nil
}

func (r *PostgresUserRepository) List(ctx context.Context) ([]*domain.User, error) {
	rows, err := r.db.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY id ASC`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectUsers(rows)
}

func collectUsers(rows pgx.Rows) ([]*domain.User, error) {
	var users []*domain.User
	for rows.Next() {
		u := &domain.User{}
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return users, nil
}
