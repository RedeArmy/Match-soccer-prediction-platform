package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresScoringRuleRepository is the PostgreSQL-backed implementation of
// ScoringRuleRepository.
type PostgresScoringRuleRepository struct {
	db *pgxpool.Pool
}

// NewPostgresScoringRuleRepository constructs a PostgresScoringRuleRepository.
func NewPostgresScoringRuleRepository(db *pgxpool.Pool) *PostgresScoringRuleRepository {
	return &PostgresScoringRuleRepository{db: db}
}

const scoringRuleColumns = "id, phase, exact_score, correct_outcome, goal_difference, extra_time_bonus, penalties_bonus, is_active, created_at, updated_at"

func scanScoringRule(row pgx.Row) (*domain.ScoringRule, error) {
	r := &domain.ScoringRule{}
	err := row.Scan(
		&r.ID, &r.Phase, &r.ExactScore, &r.CorrectOutcome,
		&r.GoalDifference, &r.ExtraTimeBonus, &r.PenaltiesBonus,
		&r.IsActive, &r.CreatedAt, &r.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return r, nil
}

// List returns all scoring rules ordered by their natural tournament
// progression (group_stage → final).
func (r *PostgresScoringRuleRepository) List(ctx context.Context) ([]*domain.ScoringRule, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+scoringRuleColumns+`
		   FROM scoring_rules
		  ORDER BY CASE phase
		      WHEN 'group_stage'   THEN 1
		      WHEN 'round_of_32'   THEN 2
		      WHEN 'round_of_16'   THEN 3
		      WHEN 'quarter_final' THEN 4
		      WHEN 'semi_final'    THEN 5
		      WHEN 'third_place'   THEN 6
		      WHEN 'final'         THEN 7
		      ELSE 8
		  END`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()

	var rules []*domain.ScoringRule
	for rows.Next() {
		rule := &domain.ScoringRule{}
		if err := rows.Scan(
			&rule.ID, &rule.Phase, &rule.ExactScore, &rule.CorrectOutcome,
			&rule.GoalDifference, &rule.ExtraTimeBonus, &rule.PenaltiesBonus,
			&rule.IsActive, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return rules, nil
}

// GetByPhase returns the scoring rule for the given phase, or nil, nil when no
// row exists for that phase.
func (r *PostgresScoringRuleRepository) GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.ScoringRule, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+scoringRuleColumns+` FROM scoring_rules WHERE phase = $1`,
		string(phase),
	)
	return scanScoringRule(row)
}

// Update persists new point values and the is_active flag for an existing phase
// row. Returns NotFound when the phase has no seeded row.
func (r *PostgresScoringRuleRepository) Update(ctx context.Context, rule *domain.ScoringRule) (*domain.ScoringRule, error) {
	row := r.db.QueryRow(ctx,
		`UPDATE scoring_rules
		    SET exact_score      = $2,
		        correct_outcome  = $3,
		        goal_difference  = $4,
		        extra_time_bonus = $5,
		        penalties_bonus  = $6,
		        is_active        = $7,
		        updated_at       = NOW()
		  WHERE phase = $1
		  RETURNING `+scoringRuleColumns,
		string(rule.Phase), rule.ExactScore, rule.CorrectOutcome,
		rule.GoalDifference, rule.ExtraTimeBonus, rule.PenaltiesBonus, rule.IsActive,
	)
	result, err := scanScoringRule(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound("scoring rule not found for phase: " + string(rule.Phase))
	}
	return result, nil
}

var _ ScoringRuleRepository = (*PostgresScoringRuleRepository)(nil)
