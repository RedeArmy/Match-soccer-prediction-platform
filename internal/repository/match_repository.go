package repository

import (
	"context"
	"time"

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

const errMatchNotFound = "match not found"

// matchColumns is used in RETURNING clauses for INSERT/UPDATE (no table alias).
const matchColumns = "id, home_team, away_team, home_score, away_score, status, phase, group_label, win_method, stadium_id, kickoff_at, created_at, updated_at, external_provider, external_match_id, last_synced_at, home_slot_id, away_slot_id"

// matchReadColumns selects match + full stadium location hierarchy for read
// queries that LEFT JOIN stadiums, cities, states, and countries.
const matchReadColumns = "m.id, m.home_team, m.away_team, m.home_score, m.away_score, m.status, m.phase, m.group_label, m.win_method, m.stadium_id, m.kickoff_at, m.created_at, m.updated_at, m.external_provider, m.external_match_id, m.last_synced_at, m.home_slot_id, m.away_slot_id," +
	" s.id, s.name, s.capacity, ci.id, ci.name, st.id, st.name, st.code, co.id, co.name, co.code"

const matchFromStadium = " FROM matches m" +
	" LEFT JOIN stadiums  s  ON s.id  = m.stadium_id" +
	" LEFT JOIN cities    ci ON ci.id = s.city_id" +
	" LEFT JOIN states    st ON st.id = ci.state_id" +
	" LEFT JOIN countries co ON co.id = st.country_id"

// matchScanDests returns the ordered scan destinations for the core match
// columns (matchColumns / matchReadColumns prefix). Used by scanMatch,
// scanMatchWithStadium, and collectMatches to avoid repeating the same
// 18-field list.
func matchScanDests(m *domain.Match) []any {
	return []any{
		&m.ID, &m.HomeTeam, &m.AwayTeam,
		&m.HomeScore, &m.AwayScore,
		&m.Status, &m.Phase, &m.GroupLabel, &m.WinMethod, &m.StadiumID, &m.KickoffAt,
		&m.CreatedAt, &m.UpdatedAt,
		&m.ExternalProvider, &m.ExternalMatchID, &m.LastSyncedAt,
		&m.HomeSlotID, &m.AwaySlotID,
	}
}

// scanMatch scans a row returned by INSERT/UPDATE RETURNING (no stadium columns).
func scanMatch(row pgx.Row) (*domain.Match, error) {
	m := &domain.Match{}
	err := row.Scan(matchScanDests(m)...)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return m, nil
}

// stadiumCols holds the nullable columns projected by the LEFT JOIN on
// stadiums, cities, states, and countries. Grouping them in a struct keeps
// scan call-sites readable and limits hydrateStadium to a single parameter.
type stadiumCols struct {
	sID, sCapacity, ciID, stID, coID              *int
	sName, ciName, stName, stCode, coName, coCode *string
}

// hydrateStadium builds a Stadium with its full location hierarchy from the
// nullable columns returned by the LEFT JOIN on stadiums/cities/states/countries.
// Returns nil when sID is nil (no stadium is assigned to the match).
func hydrateStadium(c stadiumCols) *domain.Stadium {
	if c.sID == nil {
		return nil
	}
	s := &domain.Stadium{ID: *c.sID, Name: *c.sName, Capacity: *c.sCapacity}
	if c.ciID != nil {
		s.City = &domain.City{ID: *c.ciID, Name: *c.ciName}
		if c.stID != nil {
			s.City.State = &domain.State{ID: *c.stID, Name: *c.stName, Code: *c.stCode}
			if c.coID != nil {
				s.City.State.Country = &domain.Country{ID: *c.coID, Name: *c.coName, Code: *c.coCode}
			}
		}
	}
	return s
}

// scanMatchWithStadium scans a row from a SELECT … LEFT JOIN stadiums/cities/states/countries query.
func scanMatchWithStadium(row pgx.Row) (*domain.Match, error) {
	m := &domain.Match{}
	var sc stadiumCols
	err := row.Scan(append(matchScanDests(m),
		&sc.sID, &sc.sName, &sc.sCapacity,
		&sc.ciID, &sc.ciName,
		&sc.stID, &sc.stName, &sc.stCode,
		&sc.coID, &sc.coName, &sc.coCode,
	)...)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	m.Stadium = hydrateStadium(sc)
	return m, nil
}

func (r *PostgresMatchRepository) Create(ctx context.Context, m *domain.Match) error {
	tx, err := r.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return apperrors.Internal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	row := tx.QueryRow(ctx,
		`INSERT INTO matches (home_team, away_team, status, phase, group_label, win_method, stadium_id, kickoff_at, home_slot_id, away_slot_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 RETURNING `+matchColumns,
		m.HomeTeam, m.AwayTeam, m.Status, m.Phase, m.GroupLabel, m.WinMethod, m.StadiumID, m.KickoffAt,
		m.HomeSlotID, m.AwaySlotID,
	)
	result, err := scanMatch(row)
	if err != nil {
		if isUniqueViolation(err) {
			return apperrors.Conflict("a match between these teams at this kickoff time already exists")
		}
		return err
	}

	if m.HomeSlotID != nil && m.HomeTeam != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE tournament_slots SET team=$1, confirmed_at=NOW(), updated_at=NOW() WHERE id=$2`,
			m.HomeTeam, *m.HomeSlotID,
		); err != nil {
			return apperrors.Internal(err)
		}
	}
	if m.AwaySlotID != nil && m.AwayTeam != "" {
		if _, err := tx.Exec(ctx,
			`UPDATE tournament_slots SET team=$1, confirmed_at=NOW(), updated_at=NOW() WHERE id=$2`,
			m.AwayTeam, *m.AwaySlotID,
		); err != nil {
			return apperrors.Internal(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	*m = *result
	return nil
}

func (r *PostgresMatchRepository) LinkExternal(ctx context.Context, matchID int, provider string, externalID int64) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE matches
		 SET external_provider=$1, external_match_id=$2, last_synced_at=NULL, updated_at=NOW()
		 WHERE id=$3`,
		provider, externalID, matchID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(errMatchNotFound)
	}
	return nil
}

func (r *PostgresMatchRepository) UnlinkExternal(ctx context.Context, matchID int) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE matches
		 SET external_provider=NULL, external_match_id=NULL, last_synced_at=NULL, updated_at=NOW()
		 WHERE id=$1`,
		matchID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	if tag.RowsAffected() == 0 {
		return apperrors.NotFound(errMatchNotFound)
	}
	return nil
}

func (r *PostgresMatchRepository) ListSyncCandidates(ctx context.Context, prematchWindowMin int) ([]*domain.Match, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+matchReadColumns+matchFromStadium+
			` WHERE m.external_match_id IS NOT NULL
			   AND (
			         m.status = 'live'
			         OR (m.status = 'scheduled' AND ($1 = 0 OR m.kickoff_at <= NOW() + ($1 * interval '1 minute')))
			       )
			 ORDER BY m.kickoff_at ASC`,
		prematchWindowMin,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectMatches(rows)
}

func (r *PostgresMatchRepository) FindByTeams(ctx context.Context, homeTeam, awayTeam string) (*domain.Match, error) {
	// Inline subqueries resolve each provider name through team_name_aliases.
	// COALESCE falls back to the raw parameter when no alias row exists, so
	// every team that already uses its canonical name continues to match without
	// requiring an alias entry.  Inline subqueries are used instead of a CTE +
	// CROSS JOIN to avoid adding extra columns to the result set that would
	// shift the column positions expected by scanMatchWithStadium.
	row := r.db.QueryRow(ctx,
		`SELECT `+matchReadColumns+matchFromStadium+`
		 WHERE lower(m.home_team) = lower(
		     COALESCE(
		         (SELECT canonical_name FROM team_name_aliases
		          WHERE lower(provider_name) = lower($1) LIMIT 1),
		         $1
		     )
		 )
		 AND lower(m.away_team) = lower(
		     COALESCE(
		         (SELECT canonical_name FROM team_name_aliases
		          WHERE lower(provider_name) = lower($2) LIMIT 1),
		         $2
		     )
		 )
		 LIMIT 1`,
		homeTeam, awayTeam,
	)
	m, err := scanMatchWithStadium(row)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return m, nil
}

func (r *PostgresMatchRepository) UpdateKickoff(ctx context.Context, matchID int, kickoffAt time.Time) error {
	_, err := r.db.Exec(ctx,
		`UPDATE matches SET kickoff_at=$1, updated_at=NOW() WHERE id=$2`,
		kickoffAt, matchID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (r *PostgresMatchRepository) UpdateSyncState(ctx context.Context, matchID int) error {
	_, err := r.db.Exec(ctx,
		`UPDATE matches SET last_synced_at=NOW() WHERE id=$1`,
		matchID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
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
		     status=$5, phase=$6, group_label=$7, win_method=$8, stadium_id=$9, kickoff_at=$10, updated_at=NOW()
		 WHERE id=$11
		 RETURNING `+matchColumns,
		// external_provider / external_match_id / last_synced_at are NOT updated
		// here; they are managed exclusively via LinkExternal, UnlinkExternal,
		// and UpdateSyncState to prevent the match-update path from accidentally
		// clearing the provider link.
		m.HomeTeam, m.AwayTeam, m.HomeScore, m.AwayScore,
		m.Status, m.Phase, m.GroupLabel, m.WinMethod, m.StadiumID, m.KickoffAt, m.ID,
	)
	result, err := scanMatch(row)
	if err != nil {
		return err
	}
	if result == nil {
		return apperrors.NotFound(errMatchNotFound)
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
		var sc stadiumCols
		if err := rows.Scan(append(matchScanDests(m),
			&sc.sID, &sc.sName, &sc.sCapacity,
			&sc.ciID, &sc.ciName,
			&sc.stID, &sc.stName, &sc.stCode,
			&sc.coID, &sc.coName, &sc.coCode,
		)...); err != nil {
			return nil, apperrors.Internal(err)
		}
		m.Stadium = hydrateStadium(sc)
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return matches, nil
}

func (r *PostgresMatchRepository) UpdateSlots(ctx context.Context, matchID int, homeSlotID, awaySlotID *int) (*domain.Match, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE matches
		SET home_slot_id = $1,
		    away_slot_id = $2,
		    home_team = COALESCE((SELECT team FROM tournament_slots WHERE id = $1 AND team IS NOT NULL), home_team),
		    away_team = COALESCE((SELECT team FROM tournament_slots WHERE id = $2 AND team IS NOT NULL), away_team),
		    updated_at = NOW()
		WHERE id = $3
		RETURNING `+matchColumns,
		homeSlotID, awaySlotID, matchID,
	)
	result, err := scanMatch(row)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, apperrors.NotFound(errMatchNotFound)
	}
	return result, nil
}

var _ MatchRepository = (*PostgresMatchRepository)(nil)
