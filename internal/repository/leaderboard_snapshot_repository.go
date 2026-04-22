package repository

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresLeaderboardSnapshotRepository is the PostgreSQL-backed implementation
// of LeaderboardSnapshotRepository.
type PostgresLeaderboardSnapshotRepository struct {
	db *pgxpool.Pool
}

// NewPostgresLeaderboardSnapshotRepository constructs a
// PostgresLeaderboardSnapshotRepository.
func NewPostgresLeaderboardSnapshotRepository(db *pgxpool.Pool) *PostgresLeaderboardSnapshotRepository {
	return &PostgresLeaderboardSnapshotRepository{db: db}
}

const snapshotColumns = "id, quiniela_id, taken_at, entries, created_at"

func scanSnapshot(row pgx.Row) (*domain.LeaderboardSnapshot, error) {
	s := &domain.LeaderboardSnapshot{}
	var entriesBytes []byte
	err := row.Scan(&s.ID, &s.QuinielaID, &s.TakenAt, &entriesBytes, &s.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	if err := json.Unmarshal(entriesBytes, &s.Entries); err != nil {
		return nil, apperrors.Internal(err)
	}
	return s, nil
}

func collectSnapshots(rows pgx.Rows) ([]*domain.LeaderboardSnapshot, error) {
	var snapshots []*domain.LeaderboardSnapshot
	for rows.Next() {
		s := &domain.LeaderboardSnapshot{}
		var entriesBytes []byte
		if err := rows.Scan(&s.ID, &s.QuinielaID, &s.TakenAt, &entriesBytes, &s.CreatedAt); err != nil {
			return nil, apperrors.Internal(err)
		}
		if err := json.Unmarshal(entriesBytes, &s.Entries); err != nil {
			return nil, apperrors.Internal(err)
		}
		snapshots = append(snapshots, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return snapshots, nil
}

// Create persists a new leaderboard snapshot. Entries are marshaled to JSONB.
// snapshot.ID and snapshot.CreatedAt are populated on success.
func (r *PostgresLeaderboardSnapshotRepository) Create(ctx context.Context, snapshot *domain.LeaderboardSnapshot) error {
	entriesJSON, err := json.Marshal(snapshot.Entries)
	if err != nil {
		return apperrors.Internal(err)
	}
	row := r.db.QueryRow(ctx,
		`INSERT INTO leaderboard_snapshots (quiniela_id, taken_at, entries)
		 VALUES ($1, $2, $3)
		 RETURNING `+snapshotColumns,
		snapshot.QuinielaID, snapshot.TakenAt, entriesJSON,
	)
	result, err := scanSnapshot(row)
	if err != nil {
		return err
	}
	*snapshot = *result
	return nil
}

// ListByQuiniela returns the most recent limit snapshots for a quiniela,
// ordered newest first. A limit of 0 returns all snapshots.
func (r *PostgresLeaderboardSnapshotRepository) ListByQuiniela(ctx context.Context, quinielaID, limit int) ([]*domain.LeaderboardSnapshot, error) {
	q := `SELECT ` + snapshotColumns + ` FROM leaderboard_snapshots WHERE quiniela_id = $1 ORDER BY taken_at DESC`
	args := []any{quinielaID}
	if limit > 0 {
		args = append(args, limit)
		q += ` LIMIT $2`
	}
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, apperrors.Internal(err)
	}
	defer rows.Close()
	return collectSnapshots(rows)
}

// GetLatest returns the most recently taken snapshot for a quiniela. Returns
// nil, nil when no snapshot exists yet.
func (r *PostgresLeaderboardSnapshotRepository) GetLatest(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error) {
	row := r.db.QueryRow(ctx,
		`SELECT `+snapshotColumns+`
		   FROM leaderboard_snapshots
		  WHERE quiniela_id = $1
		  ORDER BY taken_at DESC
		  LIMIT 1`,
		quinielaID,
	)
	return scanSnapshot(row)
}

var _ LeaderboardSnapshotRepository = (*PostgresLeaderboardSnapshotRepository)(nil)
