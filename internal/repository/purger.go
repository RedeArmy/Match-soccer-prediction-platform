package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresPurger implements Purger against a PostgreSQL database.
type PostgresPurger struct {
	db *pgxpool.Pool
}

// NewPostgresPurger constructs a PostgresPurger backed by db.
func NewPostgresPurger(db *pgxpool.Pool) *PostgresPurger {
	return &PostgresPurger{db: db}
}

func (p *PostgresPurger) PurgeDeletedUsers(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := p.db.Exec(ctx,
		`DELETE FROM users WHERE deleted_at IS NOT NULL AND deleted_at < $1`, olderThan)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

func (p *PostgresPurger) PurgeDeletedQuinielas(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := p.db.Exec(ctx,
		`DELETE FROM quinielas WHERE deleted_at IS NOT NULL AND deleted_at < $1`, olderThan)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

// PurgeOldSnapshots deletes leaderboard snapshots that lie outside the most-
// recent keepLatestN window for each quiniela. The window function ranks rows
// newest-first within each partition, so row 1 is always kept and only the
// tail beyond keepLatestN is removed. This bounds the table to at most
// (active_quinielas × keepLatestN) rows regardless of how many matches are
// played.
func (p *PostgresPurger) PurgeOldSnapshots(ctx context.Context, keepLatestN int) (int64, error) {
	tag, err := p.db.Exec(ctx, `
		DELETE FROM leaderboard_snapshots
		WHERE id NOT IN (
			SELECT id FROM (
				SELECT id,
				       ROW_NUMBER() OVER (
				           PARTITION BY quiniela_id
				           ORDER BY taken_at DESC
				       ) AS rn
				FROM leaderboard_snapshots
			) ranked
			WHERE rn <= $1
		)`, keepLatestN)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ Purger = (*PostgresPurger)(nil)
