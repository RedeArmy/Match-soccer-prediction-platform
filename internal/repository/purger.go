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
// newest-first within each partition; only the tail beyond keepLatestN is
// removed. This bounds the table to at most (active_quinielas × keepLatestN)
// rows regardless of how many matches are played.
//
// Scaling note: the existing composite index (quiniela_id, taken_at DESC)
// satisfies the window function sort without a sort step. The DELETE...USING
// pattern avoids the NOT IN antipattern, which degrades to O(n²) on large
// result sets because Postgres re-evaluates the subquery for each candidate
// row. At FIFA 2026 scale (≤ 64 matches × ~100 quinielas ≈ 6,400 rows) the
// difference is negligible; if the quiniela count grows beyond ~500, consider
// partitioning leaderboard_snapshots by quiniela_id to keep the per-partition
// scan bounded.
func (p *PostgresPurger) PurgeOldSnapshots(ctx context.Context, keepLatestN int) (int64, error) {
	tag, err := p.db.Exec(ctx, `
		DELETE FROM leaderboard_snapshots
		USING (
			SELECT id
			FROM (
				SELECT id,
				       ROW_NUMBER() OVER (
				           PARTITION BY quiniela_id
				           ORDER BY taken_at DESC
				       ) AS rn
				FROM leaderboard_snapshots
			) ranked
			WHERE rn > $1
		) to_delete
		WHERE leaderboard_snapshots.id = to_delete.id
	`, keepLatestN)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

// EraseUserPII anonymises or deletes all personal data for userID within a
// single transaction. It must be called before PurgeDeletedUsers to avoid
// ON DELETE RESTRICT violations from payment_records.user_id.
//
// Operations follow the retention tiers in domain.RetentionTier:
//
//	audit_log.actor_id → NULL       (ImmutableAnonymise)
//	payment_records.user_id → NULL  (ImmutableAnonymise)
//	predictions rows deleted        (OperationalDelete)
//	tiebreakers rows deleted        (OperationalDelete)
func (p *PostgresPurger) EraseUserPII(ctx context.Context, userID int) error {
	tx, err := p.db.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	steps := []struct {
		query string
	}{
		{`UPDATE audit_log SET actor_id = NULL WHERE actor_id = $1`},
		{`UPDATE payment_records SET user_id = NULL WHERE user_id = $1`},
		{`DELETE FROM predictions WHERE user_id = $1`},
		{`DELETE FROM tiebreakers WHERE user_id = $1`},
	}
	for _, s := range steps {
		if _, err := tx.Exec(ctx, s.query, userID); err != nil {
			return apperrors.Internal(err)
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

func (p *PostgresPurger) PurgeOldParamHistory(ctx context.Context, before time.Time) (int64, error) {
	tag, err := p.db.Exec(ctx,
		`DELETE FROM system_params_history WHERE changed_at < $1`, before)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ Purger = (*PostgresPurger)(nil)
