package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresSystemParamRepository is the PostgreSQL-backed implementation of
// SystemParamRepository.
type PostgresSystemParamRepository struct {
	db *pgxpool.Pool
}

// NewPostgresSystemParamRepository constructs a PostgresSystemParamRepository.
func NewPostgresSystemParamRepository(db *pgxpool.Pool) *PostgresSystemParamRepository {
	return &PostgresSystemParamRepository{db: db}
}

const systemParamColumns = "key, value, default_value, type, category, is_runtime, description, created_at, updated_at"

func scanSystemParam(row pgx.Row) (*domain.SystemParam, error) {
	p := &domain.SystemParam{}
	err := row.Scan(&p.Key, &p.Value, &p.DefaultValue, &p.Type, &p.Category, &p.IsRuntime, &p.Description, &p.CreatedAt, &p.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return p, nil
}

func collectSystemParams(rows pgx.Rows) ([]*domain.SystemParam, error) {
	var params []*domain.SystemParam
	for rows.Next() {
		p := &domain.SystemParam{}
		if err := rows.Scan(&p.Key, &p.Value, &p.DefaultValue, &p.Type, &p.Category, &p.IsRuntime, &p.Description, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		params = append(params, p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return params, nil
}

// Get returns the SystemParam for the given key. Returns nil, nil when the key
// does not exist so callers can distinguish "not configured" from a DB error.
func (r *PostgresSystemParamRepository) Get(ctx context.Context, key string) (*domain.SystemParam, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+systemParamColumns+` FROM system_params WHERE key = $1`, key,
	)
	return scanSystemParam(row)
}

// GetAll returns every row in system_params ordered by key for deterministic
// iteration.
func (r *PostgresSystemParamRepository) GetAll(ctx context.Context) ([]*domain.SystemParam, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+systemParamColumns+` FROM system_params ORDER BY key ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectSystemParams(rows)
}

// GetByCategory returns all params whose category equals cat, ordered by key.
func (r *PostgresSystemParamRepository) GetByCategory(ctx context.Context, category string) ([]*domain.SystemParam, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+systemParamColumns+` FROM system_params WHERE category = $1 ORDER BY key ASC`,
		category,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectSystemParams(rows)
}

// Set upserts a single key-value pair. When the key already exists only the
// value and updated_at are changed; type, category, and is_runtime are
// preserved from the original row. Returns the full param after the upsert.
// actorID identifies the caller for audit purposes and is handled by the
// service layer - it is accepted here to keep the interface consistent.
func (r *PostgresSystemParamRepository) Set(ctx context.Context, key, value string, _ int) (*domain.SystemParam, error) {
	row := r.db.QueryRow(ctx,
		`INSERT INTO system_params (key, value, default_value)
		 VALUES ($1, $2, $2)
		 ON CONFLICT (key) DO UPDATE
		     SET value = EXCLUDED.value, updated_at = NOW()
		 RETURNING `+systemParamColumns,
		key, value,
	)
	result, err := scanSystemParam(row)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// BulkSet upserts every entry in params atomically via a single UNNEST
// statement. Existing rows have only their value updated; type, category,
// and is_runtime are preserved. A nil or empty map is a no-op.
// actorID is forwarded to the service layer for audit logging.
func (r *PostgresSystemParamRepository) BulkSet(ctx context.Context, params map[string]string, _ int) error {
	if len(params) == 0 {
		return nil
	}
	keys := make([]string, 0, len(params))
	vals := make([]string, 0, len(params))
	for k, v := range params {
		keys = append(keys, k)
		vals = append(vals, v)
	}
	_, err := r.db.Exec(ctx,
		`INSERT INTO system_params (key, value, default_value)
		 SELECT k, v, v FROM UNNEST($1::text[], $2::text[]) AS t(k, v)
		 ON CONFLICT (key) DO UPDATE
		     SET value = EXCLUDED.value, updated_at = NOW()`,
		keys, vals,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// ResetToDefault sets value = default_value for key and returns the updated
// param. Returns nil, nil when the key does not exist.
func (r *PostgresSystemParamRepository) ResetToDefault(ctx context.Context, key string) (*domain.SystemParam, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE system_params
		    SET value = default_value, updated_at = NOW()
		  WHERE key = $1
		  RETURNING `+systemParamColumns,
		key,
	)
	return scanSystemParam(row)
}

var _ SystemParamRepository = (*PostgresSystemParamRepository)(nil)
