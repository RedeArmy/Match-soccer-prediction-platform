package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresStadiumRepository is the PostgreSQL-backed implementation of StadiumRepository.
type PostgresStadiumRepository struct {
	db *pgxpool.Pool
}

// NewPostgresStadiumRepository constructs a PostgresStadiumRepository.
func NewPostgresStadiumRepository(db *pgxpool.Pool) *PostgresStadiumRepository {
	return &PostgresStadiumRepository{db: db}
}

const stadiumColumns = "id, name, city, country, capacity, created_at, updated_at"

func scanStadium(row pgx.Row) (*domain.Stadium, error) {
	s := &domain.Stadium{}
	err := row.Scan(&s.ID, &s.Name, &s.City, &s.Country, &s.Capacity, &s.CreatedAt, &s.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return s, nil
}

func (r *PostgresStadiumRepository) GetByID(ctx context.Context, id int) (*domain.Stadium, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+stadiumColumns+` FROM stadiums WHERE id = $1`, id,
	)
	return scanStadium(row)
}

func (r *PostgresStadiumRepository) List(ctx context.Context) ([]*domain.Stadium, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+stadiumColumns+` FROM stadiums ORDER BY country, city ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var stadiums []*domain.Stadium
	for rows.Next() {
		s := &domain.Stadium{}
		if err := rows.Scan(&s.ID, &s.Name, &s.City, &s.Country, &s.Capacity, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		stadiums = append(stadiums, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return stadiums, nil
}
