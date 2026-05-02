package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresTiebreakerConfigRepository is the PostgreSQL-backed implementation
// of TiebreakerConfigRepository.
type PostgresTiebreakerConfigRepository struct {
	db *pgxpool.Pool
}

// NewPostgresTiebreakerConfigRepository constructs a PostgresTiebreakerConfigRepository.
func NewPostgresTiebreakerConfigRepository(db *pgxpool.Pool) *PostgresTiebreakerConfigRepository {
	return &PostgresTiebreakerConfigRepository{db: db}
}

const tiebreakerConfigColumns = "id, question, result, created_at, updated_at"

func scanTiebreakerConfig(row pgx.Row) (*domain.TiebreakerConfig, error) {
	cfg := &domain.TiebreakerConfig{}
	err := row.Scan(&cfg.ID, &cfg.Question, &cfg.Result, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return cfg, nil
}

// Get returns the current global tiebreaker configuration.
// Returns nil, nil when no question has been configured yet.
func (r *PostgresTiebreakerConfigRepository) Get(ctx context.Context) (*domain.TiebreakerConfig, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerConfigColumns+` FROM tiebreaker_config WHERE id=1`,
	)
	return scanTiebreakerConfig(row)
}

// Upsert sets or replaces the global tiebreaker question. The row is always
// written at id=1 (the singleton). Passing the same question again is a no-op
// in terms of business logic but still performs an UPDATE with updated_at=NOW().
func (r *PostgresTiebreakerConfigRepository) Upsert(ctx context.Context, question string) (*domain.TiebreakerConfig, error) {
	row := r.db.QueryRow(ctx,
		`INSERT INTO tiebreaker_config (id, question)
		 VALUES (1, $1)
		 ON CONFLICT (id) DO UPDATE
		     SET question=$1, updated_at=NOW()
		 RETURNING `+tiebreakerConfigColumns,
		question,
	)
	cfg, err := scanTiebreakerConfig(row)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// SetResult records the confirmed numeric outcome for the global tiebreaker.
// Returns NotFound when no configuration row exists (i.e. no question has
// been set yet).
func (r *PostgresTiebreakerConfigRepository) SetResult(ctx context.Context, result int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE tiebreaker_config SET result=$1, updated_at=NOW() WHERE id=1`,
		result,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("tiebreaker config not found - set the question first")
	}
	return nil
}

var _ TiebreakerConfigRepository = (*PostgresTiebreakerConfigRepository)(nil)
