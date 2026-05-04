package repository

import (
	"context"
	"strconv"

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

const tiebreakerColumns = "id, user_id, prediction, created_at, updated_at"

func scanTiebreaker(row pgx.Row) (*domain.Tiebreaker, error) {
	tb := &domain.Tiebreaker{}
	err := row.Scan(&tb.ID, &tb.UserID, &tb.Prediction, &tb.CreatedAt, &tb.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return tb, nil
}

func collectTiebreakers(rows pgx.Rows) ([]*domain.Tiebreaker, error) {
	var tbs []*domain.Tiebreaker
	for rows.Next() {
		tb := &domain.Tiebreaker{}
		if err := rows.Scan(&tb.ID, &tb.UserID, &tb.Prediction, &tb.CreatedAt, &tb.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		tbs = append(tbs, tb)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return tbs, nil
}

func (r *PostgresTiebreakerRepository) Create(ctx context.Context, tb *domain.Tiebreaker) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tiebreakers (user_id, prediction)
		 VALUES ($1, $2) RETURNING `+tiebreakerColumns,
		tb.UserID, tb.Prediction,
	)
	result, err := scanTiebreaker(row)
	if err != nil {
		return err
	}
	*tb = *result
	return nil
}

func (r *PostgresTiebreakerRepository) GetByUser(ctx context.Context, userID int) (*domain.Tiebreaker, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerColumns+` FROM tiebreakers WHERE user_id=$1`,
		userID,
	)
	return scanTiebreaker(row)
}

func (r *PostgresTiebreakerRepository) Update(ctx context.Context, tb *domain.Tiebreaker) error {
	row := r.db.QueryRow(ctx,
		`UPDATE tiebreakers SET prediction=$1, updated_at=NOW()
		 WHERE id=$2 RETURNING `+tiebreakerColumns,
		tb.Prediction, tb.ID,
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

// ListByUserIDs returns global tiebreaker predictions for the given user IDs.
// Used by the ranking service to load all relevant entries for a group in a
// single round-trip. An empty ids slice returns nil, nil without hitting the
// database.
func (r *PostgresTiebreakerRepository) ListByUserIDs(ctx context.Context, userIDs []int) ([]*domain.Tiebreaker, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx,
		`SELECT `+tiebreakerColumns+` FROM tiebreakers WHERE user_id = ANY($1)`,
		userIDs,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	return collectTiebreakers(rows)
}

// ListAll returns all tiebreaker submissions with pagination.
// Pagination.Limit must be positive or unbounded (via Unbounded()).
func (r *PostgresTiebreakerRepository) ListAll(ctx context.Context, p Pagination) ([]*domain.Tiebreaker, error) {
	if p.Limit == 0 {
		return nil, apperrors.Validation("pagination limit must be positive or use Unbounded()")
	}

	q := `SELECT ` + tiebreakerColumns + ` FROM tiebreakers ORDER BY created_at DESC`
	args := []any{}
	n := 1

	if p.Limit > 0 {
		q += ` LIMIT $` + strconv.Itoa(n)
		args = append(args, p.Limit)
		n++
	}
	// p.IsUnbounded() case: no LIMIT clause
	if p.Offset > 0 {
		q += ` OFFSET $` + strconv.Itoa(n)
		args = append(args, p.Offset)
	}

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	return collectTiebreakers(rows)
}

var _ TiebreakerRepository = (*PostgresTiebreakerRepository)(nil)
