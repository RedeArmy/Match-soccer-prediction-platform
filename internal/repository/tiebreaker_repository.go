package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresTiebreakerRepository is the PostgreSQL-backed implementation of TiebreakerRepository.
type PostgresTiebreakerRepository struct {
	db *pgxpool.Pool
}

// NewPostgresTiebreakerRepository constructs a PostgresTiebreakerRepository.
func NewPostgresTiebreakerRepository(db *pgxpool.Pool) *PostgresTiebreakerRepository {
	return &PostgresTiebreakerRepository{db: db}
}

const tiebreakerColumns = "id, user_id, quiniela_id, prediction, result, created_at, updated_at"

func scanTiebreaker(row pgx.Row) (*domain.Tiebreaker, error) {
	tb := &domain.Tiebreaker{}
	err := row.Scan(&tb.ID, &tb.UserID, &tb.QuinielaID, &tb.Prediction, &tb.Result, &tb.CreatedAt, &tb.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return tb, nil
}

func (r *PostgresTiebreakerRepository) Create(ctx context.Context, tb *domain.Tiebreaker) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tiebreakers (user_id, quiniela_id, prediction)
		 VALUES ($1, $2, $3) RETURNING `+tiebreakerColumns,
		tb.UserID, tb.QuinielaID, tb.Prediction,
	)
	result, err := scanTiebreaker(row)
	if err != nil {
		return err
	}
	*tb = *result
	return nil
}

func (r *PostgresTiebreakerRepository) GetByUserAndQuiniela(ctx context.Context, userID, quinielaID int) (*domain.Tiebreaker, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerColumns+` FROM tiebreakers WHERE user_id=$1 AND quiniela_id=$2`,
		userID, quinielaID,
	)
	return scanTiebreaker(row)
}

func (r *PostgresTiebreakerRepository) Update(ctx context.Context, tb *domain.Tiebreaker) error {
	row := r.db.QueryRow(ctx,
		`UPDATE tiebreakers SET prediction=$1, result=$2, updated_at=NOW()
		 WHERE id=$3 RETURNING `+tiebreakerColumns,
		tb.Prediction, tb.Result, tb.ID,
	)
	result, err := scanTiebreaker(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound("tiebreaker not found")
	}
	*tb = *result
	return nil
}

func (r *PostgresTiebreakerRepository) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.Tiebreaker, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+tiebreakerColumns+` FROM tiebreakers WHERE quiniela_id=$1 ORDER BY created_at ASC`, quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectTiebreakers(rows)
}

func collectTiebreakers(rows pgx.Rows) ([]*domain.Tiebreaker, error) {
	var tbs []*domain.Tiebreaker
	for rows.Next() {
		tb := &domain.Tiebreaker{}
		if err := rows.Scan(&tb.ID, &tb.UserID, &tb.QuinielaID, &tb.Prediction, &tb.Result, &tb.CreatedAt, &tb.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		tbs = append(tbs, tb)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return tbs, nil
}
