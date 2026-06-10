package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresQuinielaRoundEntryRepository is the PostgreSQL-backed implementation
// of QuinielaRoundEntryRepository.
type PostgresQuinielaRoundEntryRepository struct {
	db *pgxpool.Pool
}

// NewPostgresQuinielaRoundEntryRepository constructs a repository backed by db.
func NewPostgresQuinielaRoundEntryRepository(db *pgxpool.Pool) *PostgresQuinielaRoundEntryRepository {
	return &PostgresQuinielaRoundEntryRepository{db: db}
}

func scanRoundEntry(row pgx.Row) (*domain.QuinielaRoundEntry, error) {
	e := &domain.QuinielaRoundEntry{}
	err := row.Scan(&e.ID, &e.QuinielaID, &e.UserID, &e.RoundKey, &e.PaidAt, &e.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	return e, nil
}

const roundEntryColumns = "id, quiniela_id, user_id, round_key, paid_at, created_at"

// Upsert inserts a round-entry row; if it already exists the existing row is
// returned (idempotent via ON CONFLICT DO NOTHING + subsequent SELECT).
func (r *PostgresQuinielaRoundEntryRepository) Upsert(
	ctx context.Context, quinielaID, userID int, roundKey string,
) (*domain.QuinielaRoundEntry, error) {
	// Attempt insert; skip if already present.
	_, err := r.db.Exec(ctx,
		`INSERT INTO quiniela_round_entries (quiniela_id, user_id, round_key)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (quiniela_id, user_id, round_key) DO NOTHING`,
		quinielaID, userID, roundKey,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	// Fetch the authoritative row (handles both insert and pre-existing case).
	row := r.db.QueryRow(ctx,
		`SELECT `+roundEntryColumns+`
		   FROM quiniela_round_entries
		  WHERE quiniela_id = $1 AND user_id = $2 AND round_key = $3`,
		quinielaID, userID, roundKey,
	)
	entry, err := scanRoundEntry(row)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, apperrors.Internal(nil)
	}
	return entry, nil
}

// ListByRound returns all entries for (quinielaID, roundKey), ordered by paid_at.
func (r *PostgresQuinielaRoundEntryRepository) ListByRound(
	ctx context.Context, quinielaID int, roundKey string,
) ([]*domain.QuinielaRoundEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+roundEntryColumns+`
		   FROM quiniela_round_entries
		  WHERE quiniela_id = $1 AND round_key = $2
		  ORDER BY paid_at`,
		quinielaID, roundKey,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectRoundEntries(rows)
}

// HasEntry reports whether (quinielaID, userID, roundKey) exists.
func (r *PostgresQuinielaRoundEntryRepository) HasEntry(
	ctx context.Context, quinielaID, userID int, roundKey string,
) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(
		   SELECT 1 FROM quiniela_round_entries
		    WHERE quiniela_id = $1 AND user_id = $2 AND round_key = $3
		 )`,
		quinielaID, userID, roundKey,
	).Scan(&exists)
	if err != nil {
		return false, apperrors.Internal(err)
	}
	return exists, nil
}

// ListByQuiniela returns all entries for a group, ordered by round_key, user_id.
func (r *PostgresQuinielaRoundEntryRepository) ListByQuiniela(
	ctx context.Context, quinielaID int,
) ([]*domain.QuinielaRoundEntry, error) {
	rows, err := r.db.Query(ctx,
		`SELECT `+roundEntryColumns+`
		   FROM quiniela_round_entries
		  WHERE quiniela_id = $1
		  ORDER BY round_key, user_id`,
		quinielaID,
	)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectRoundEntries(rows)
}

func collectRoundEntries(rows pgx.Rows) ([]*domain.QuinielaRoundEntry, error) {
	var out []*domain.QuinielaRoundEntry
	for rows.Next() {
		e := &domain.QuinielaRoundEntry{}
		if err := rows.Scan(&e.ID, &e.QuinielaID, &e.UserID, &e.RoundKey, &e.PaidAt, &e.CreatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return out, nil
}

var _ QuinielaRoundEntryRepository = (*PostgresQuinielaRoundEntryRepository)(nil)
