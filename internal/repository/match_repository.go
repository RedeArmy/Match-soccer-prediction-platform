package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresMatchRepository is the PostgreSQL-backed implementation of
// MatchRepository. It uses a *pgxpool.Pool so that connections are reused
// across requests rather than opened per-query.
type PostgresMatchRepository struct {
	db *pgxpool.Pool
}

// NewPostgresMatchRepository constructs a PostgresMatchRepository.
func NewPostgresMatchRepository(db *pgxpool.Pool) *PostgresMatchRepository {
	return &PostgresMatchRepository{db: db}
}

const matchColumns = "id, home_team, away_team, home_score, away_score, status, stadium_id, kickoff_at, created_at, updated_at"

func scanMatch(row pgx.Row) (*domain.Match, error) {
	m := &domain.Match{}
	err := row.Scan(
		&m.ID, &m.HomeTeam, &m.AwayTeam,
		&m.HomeScore, &m.AwayScore,
		&m.Status, &m.StadiumID, &m.KickoffAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return m, nil
}

func (r *PostgresMatchRepository) Create(ctx context.Context, m *domain.Match) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO matches (home_team, away_team, status, stadium_id, kickoff_at)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING `+matchColumns,
		m.HomeTeam, m.AwayTeam, m.Status, m.StadiumID, m.KickoffAt,
	)
	result, err := scanMatch(row)
	if err != nil {
		return err
	}
	*m = *result
	return nil
}

func (r *PostgresMatchRepository) GetByID(ctx context.Context, id int) (*domain.Match, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+matchColumns+` FROM matches WHERE id = $1`, id,
	)
	return scanMatch(row)
}

func (r *PostgresMatchRepository) Update(ctx context.Context, m *domain.Match) error {
	row := r.db.QueryRow(ctx,
		`UPDATE matches
		 SET home_team=$1, away_team=$2, home_score=$3, away_score=$4,
		     status=$5, stadium_id=$6, kickoff_at=$7, updated_at=NOW()
		 WHERE id=$8
		 RETURNING `+matchColumns,
		m.HomeTeam, m.AwayTeam, m.HomeScore, m.AwayScore,
		m.Status, m.StadiumID, m.KickoffAt, m.ID,
	)
	result, err := scanMatch(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound("match not found")
	}
	*m = *result
	return nil
}

func (r *PostgresMatchRepository) List(ctx context.Context) ([]*domain.Match, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+matchColumns+` FROM matches ORDER BY kickoff_at ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMatches(rows)
}

func (r *PostgresMatchRepository) ListByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+matchColumns+` FROM matches WHERE status=$1 ORDER BY kickoff_at ASC`, status,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMatches(rows)
}

func collectMatches(rows pgx.Rows) ([]*domain.Match, error) {
	var matches []*domain.Match
	for rows.Next() {
		m := &domain.Match{}
		if err := rows.Scan(
			&m.ID, &m.HomeTeam, &m.AwayTeam,
			&m.HomeScore, &m.AwayScore,
			&m.Status, &m.StadiumID, &m.KickoffAt,
			&m.CreatedAt, &m.UpdatedAt,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return matches, nil
}
