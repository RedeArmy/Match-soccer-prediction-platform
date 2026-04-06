package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresQuinielaRepository is the PostgreSQL-backed implementation of QuinielaRepository.
type PostgresQuinielaRepository struct {
	db *pgxpool.Pool
}

// NewPostgresQuinielaRepository constructs a PostgresQuinielaRepository.
func NewPostgresQuinielaRepository(db *pgxpool.Pool) *PostgresQuinielaRepository {
	return &PostgresQuinielaRepository{db: db}
}

const quinielaColumns = "id, name, owner_id, created_at, updated_at"

func scanQuiniela(row pgx.Row) (*domain.Quiniela, error) {
	q := &domain.Quiniela{}
	err := row.Scan(&q.ID, &q.Name, &q.OwnerID, &q.CreatedAt, &q.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return q, nil
}

func (r *PostgresQuinielaRepository) Create(ctx context.Context, q *domain.Quiniela) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO quinielas (name, owner_id) VALUES ($1, $2) RETURNING `+quinielaColumns,
		q.Name, q.OwnerID,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		return err
	}
	*q = *result
	return nil
}

func (r *PostgresQuinielaRepository) GetByID(ctx context.Context, id int) (*domain.Quiniela, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas WHERE id=$1`, id,
	)
	return scanQuiniela(row)
}

func (r *PostgresQuinielaRepository) Update(ctx context.Context, q *domain.Quiniela) error {
	row := r.db.QueryRow(ctx,
		`UPDATE quinielas SET name=$1, updated_at=NOW() WHERE id=$2 RETURNING `+quinielaColumns,
		q.Name, q.ID,
	)
	result, err := scanQuiniela(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound("quiniela not found")
	}
	*q = *result
	return nil
}

func (r *PostgresQuinielaRepository) Delete(ctx context.Context, id int) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM quinielas WHERE id=$1`, id)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("quiniela not found")
	}
	return nil
}

func (r *PostgresQuinielaRepository) ListByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+quinielaColumns+` FROM quinielas WHERE owner_id=$1 ORDER BY created_at DESC`, ownerID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectQuinielas(rows)
}

func collectQuinielas(rows pgx.Rows) ([]*domain.Quiniela, error) {
	var quinielas []*domain.Quiniela
	for rows.Next() {
		q := &domain.Quiniela{}
		if err := rows.Scan(&q.ID, &q.Name, &q.OwnerID, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		quinielas = append(quinielas, q)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return quinielas, nil
}
