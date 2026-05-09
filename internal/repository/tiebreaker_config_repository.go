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

const tiebreakerConfigColumns = "id, question, phase, quiniela_id, result, created_at, updated_at"

func scanTiebreakerConfig(row pgx.Row) (*domain.TiebreakerConfig, error) {
	cfg := &domain.TiebreakerConfig{}
	var phase *string
	err := row.Scan(&cfg.ID, &cfg.Question, &phase, &cfg.QuinielaID, &cfg.Result, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	if phase != nil {
		p := domain.MatchPhase(*phase)
		cfg.Phase = &p
	}
	return cfg, nil
}

// Get returns the global tiebreaker configuration (phase IS NULL, quiniela_id IS NULL).
// Returns nil, nil when no global question has been configured yet.
func (r *PostgresTiebreakerConfigRepository) Get(ctx context.Context) (*domain.TiebreakerConfig, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerConfigColumns+`
		   FROM tiebreaker_config
		  WHERE id = 1`,
	)
	return scanTiebreakerConfig(row)
}

// GetByPhase returns the platform-wide config scoped to phase (quiniela_id IS NULL).
// Returns nil, nil when no question has been set for that phase.
func (r *PostgresTiebreakerConfigRepository) GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.TiebreakerConfig, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerConfigColumns+`
		   FROM tiebreaker_config
		  WHERE phase = $1
		    AND quiniela_id IS NULL`,
		string(phase),
	)
	return scanTiebreakerConfig(row)
}

// GetByQuiniela returns the group-specific config for quinielaID (phase IS NULL).
// Returns nil, nil when no group-specific question has been configured.
func (r *PostgresTiebreakerConfigRepository) GetByQuiniela(ctx context.Context, quinielaID int) (*domain.TiebreakerConfig, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+tiebreakerConfigColumns+`
		   FROM tiebreaker_config
		  WHERE quiniela_id = $1
		    AND phase IS NULL`,
		quinielaID,
	)
	return scanTiebreakerConfig(row)
}

// Upsert sets or replaces the global tiebreaker question. The row is always
// written at id=1 (the global singleton). Passing the same question again is a
// no-op in terms of business logic but still touches updated_at.
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

// UpsertForPhase sets or replaces the phase-scoped question (quiniela_id IS NULL).
func (r *PostgresTiebreakerConfigRepository) UpsertForPhase(ctx context.Context, phase domain.MatchPhase, question string) (*domain.TiebreakerConfig, error) {
	// ON CONFLICT targets the partial unique index uq_tiebreaker_config_phase.
	row := r.db.QueryRow(ctx,
		`INSERT INTO tiebreaker_config (question, phase)
		 VALUES ($1, $2)
		 ON CONFLICT (phase) WHERE phase IS NOT NULL AND quiniela_id IS NULL
		     DO UPDATE SET question=$1, updated_at=NOW()
		 RETURNING `+tiebreakerConfigColumns,
		question, string(phase),
	)
	cfg, err := scanTiebreakerConfig(row)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// UpsertForQuiniela sets or replaces the group-specific question (phase IS NULL).
func (r *PostgresTiebreakerConfigRepository) UpsertForQuiniela(ctx context.Context, quinielaID int, question string) (*domain.TiebreakerConfig, error) {
	// ON CONFLICT targets the partial unique index uq_tiebreaker_config_quiniela.
	row := r.db.QueryRow(ctx,
		`INSERT INTO tiebreaker_config (question, quiniela_id)
		 VALUES ($1, $2)
		 ON CONFLICT (quiniela_id) WHERE quiniela_id IS NOT NULL AND phase IS NULL
		     DO UPDATE SET question=$1, updated_at=NOW()
		 RETURNING `+tiebreakerConfigColumns,
		question, quinielaID,
	)
	cfg, err := scanTiebreakerConfig(row)
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

// SetResult records the confirmed numeric outcome for the global config.
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

// SetResultByID records the confirmed numeric outcome for any config by ID.
// Returns NotFound when configID does not exist.
func (r *PostgresTiebreakerConfigRepository) SetResultByID(ctx context.Context, configID, result int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE tiebreaker_config SET result=$1, updated_at=NOW() WHERE id=$2`,
		result, configID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound("tiebreaker config not found")
	}
	return nil
}

var _ TiebreakerConfigRepository = (*PostgresTiebreakerConfigRepository)(nil)
