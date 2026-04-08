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

// matchColumns is used in RETURNING clauses for INSERT/UPDATE (no table alias).
const matchColumns = "id, home_team, away_team, home_score, away_score, status, phase, stadium_id, kickoff_at, created_at, updated_at"

// matchReadColumns selects match + stadium fields for read queries that LEFT JOIN stadiums.
const matchReadColumns = "m.id, m.home_team, m.away_team, m.home_score, m.away_score, m.status, m.phase, m.stadium_id, m.kickoff_at, m.created_at, m.updated_at," +
	" s.id, s.name, s.city, s.country, s.capacity"

const matchFromStadium = " FROM matches m LEFT JOIN stadiums s ON s.id = m.stadium_id"

// scanMatch scans a row returned by INSERT/UPDATE RETURNING (no stadium columns).
func scanMatch(row pgx.Row) (*domain.Match, error) {
	m := &domain.Match{}
	err := row.Scan(
		&m.ID, &m.HomeTeam, &m.AwayTeam,
		&m.HomeScore, &m.AwayScore,
		&m.Status, &m.Phase, &m.StadiumID, &m.KickoffAt,
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

// scanMatchWithStadium scans a row from a SELECT … LEFT JOIN stadiums query.
func scanMatchWithStadium(row pgx.Row) (*domain.Match, error) {
	m := &domain.Match{}
	var sID *int
	var sName, sCity, sCountry *string
	var sCapacity *int
	err := row.Scan(
		&m.ID, &m.HomeTeam, &m.AwayTeam,
		&m.HomeScore, &m.AwayScore,
		&m.Status, &m.Phase, &m.StadiumID, &m.KickoffAt,
		&m.CreatedAt, &m.UpdatedAt,
		&sID, &sName, &sCity, &sCountry, &sCapacity,
	)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	if sID != nil {
		m.Stadium = &domain.Stadium{
			ID: *sID, Name: *sName, City: *sCity, Country: *sCountry, Capacity: *sCapacity,
		}
	}
	return m, nil
}

func (r *PostgresMatchRepository) Create(ctx context.Context, m *domain.Match) error {
	row := r.db.QueryRow(ctx,
		`INSERT INTO matches (home_team, away_team, status, phase, stadium_id, kickoff_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING `+matchColumns,
		m.HomeTeam, m.AwayTeam, m.Status, m.Phase, m.StadiumID, m.KickoffAt,
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
		`SELECT `+matchReadColumns+matchFromStadium+` WHERE m.id = $1`, id,
	)
	return scanMatchWithStadium(row)
}

func (r *PostgresMatchRepository) Update(ctx context.Context, m *domain.Match) error {
	row := r.db.QueryRow(ctx,
		`UPDATE matches
		 SET home_team=$1, away_team=$2, home_score=$3, away_score=$4,
		     status=$5, phase=$6, stadium_id=$7, kickoff_at=$8, updated_at=NOW()
		 WHERE id=$9
		 RETURNING `+matchColumns,
		m.HomeTeam, m.AwayTeam, m.HomeScore, m.AwayScore,
		m.Status, m.Phase, m.StadiumID, m.KickoffAt, m.ID,
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
		`SELECT `+matchReadColumns+matchFromStadium+` ORDER BY m.kickoff_at ASC`,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMatches(rows)
}

func (r *PostgresMatchRepository) ListByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+matchReadColumns+matchFromStadium+` WHERE m.phase=$1 ORDER BY m.kickoff_at ASC`, phase,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMatches(rows)
}

func (r *PostgresMatchRepository) ListByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+matchReadColumns+matchFromStadium+` WHERE m.status=$1 ORDER BY m.kickoff_at ASC`, status,
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
		var sID *int
		var sName, sCity, sCountry *string
		var sCapacity *int
		if err := rows.Scan(
			&m.ID, &m.HomeTeam, &m.AwayTeam,
			&m.HomeScore, &m.AwayScore,
			&m.Status, &m.Phase, &m.StadiumID, &m.KickoffAt,
			&m.CreatedAt, &m.UpdatedAt,
			&sID, &sName, &sCity, &sCountry, &sCapacity,
		); err != nil {
			return nil, apperrors.Internal(err)
		}
		if sID != nil {
			m.Stadium = &domain.Stadium{
				ID: *sID, Name: *sName, City: *sCity, Country: *sCountry, Capacity: *sCapacity,
			}
		}
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return matches, nil
}
