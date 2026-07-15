package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresExtraRuleRepository is the PostgreSQL-backed implementation of
// ExtraRuleRepository.
type PostgresExtraRuleRepository struct {
	db *pgxpool.Pool
}

// NewPostgresExtraRuleRepository constructs a PostgresExtraRuleRepository.
func NewPostgresExtraRuleRepository(db *pgxpool.Pool) *PostgresExtraRuleRepository {
	return &PostgresExtraRuleRepository{db: db}
}

const extraRuleColumns = "id, extra_type, points, is_active, created_at, updated_at"

func scanExtraRule(row pgx.Row) (*domain.ExtraRule, error) {
	r := &domain.ExtraRule{}
	err := row.Scan(&r.ID, &r.ExtraType, &r.Points, &r.IsActive, &r.CreatedAt, &r.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return r, nil
}

// List returns all extra rules ordered by extra_type for stable admin display.
func (r *PostgresExtraRuleRepository) List(ctx context.Context) ([]*domain.ExtraRule, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+extraRuleColumns+` FROM extra_rules ORDER BY extra_type`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectRows(rows, func(rr pgx.Rows) (*domain.ExtraRule, error) {
		rule := &domain.ExtraRule{}
		return rule, rr.Scan(&rule.ID, &rule.ExtraType, &rule.Points, &rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt)
	})
}

// GetByType returns the rule for the given extra type, or nil, nil when no
// row exists for that type.
func (r *PostgresExtraRuleRepository) GetByType(ctx context.Context, extraType domain.ExtraType) (*domain.ExtraRule, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+extraRuleColumns+` FROM extra_rules WHERE extra_type = $1`,
		string(extraType),
	)
	return scanExtraRule(row)
}

// Update persists a new point value and the is_active flag for an existing
// extra_type row. Returns NotFound when the type has no seeded row.
func (r *PostgresExtraRuleRepository) Update(ctx context.Context, rule *domain.ExtraRule) (*domain.ExtraRule, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE extra_rules
		    SET points     = $2,
		        is_active  = $3,
		        updated_at = NOW()
		  WHERE extra_type = $1
		  RETURNING `+extraRuleColumns,
		string(rule.ExtraType), rule.Points, rule.IsActive,
	)
	result, err := scanExtraRule(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound("extra rule not found for type: " + string(rule.ExtraType))
	}
	return result, nil
}

var _ ExtraRuleRepository = (*PostgresExtraRuleRepository)(nil)
