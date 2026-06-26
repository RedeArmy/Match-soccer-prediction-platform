package repository

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresTeamRepository is the PostgreSQL-backed implementation of TeamRepository.
type PostgresTeamRepository struct {
	db *pgxpool.Pool
}

// NewPostgresTeamRepository constructs a PostgresTeamRepository.
func NewPostgresTeamRepository(db *pgxpool.Pool) *PostgresTeamRepository {
	return &PostgresTeamRepository{db: db}
}

// ListTeamNames returns team names from the teams table sorted A → Z.
// Returns an empty slice (never nil) so the JSON response is always [] not null.
func (r *PostgresTeamRepository) ListTeamNames(ctx context.Context) ([]string, error) {
	rows, err := r.db.Query(ctx, `SELECT name FROM teams ORDER BY name ASC`)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	names := make([]string, 0)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, apperrors.Internal(err)
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return names, nil
}

var _ TeamRepository = (*PostgresTeamRepository)(nil)
