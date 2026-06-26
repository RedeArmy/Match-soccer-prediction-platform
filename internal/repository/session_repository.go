package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PostgresSessionRepository is the PostgreSQL-backed implementation of SessionRepository.
type PostgresSessionRepository struct {
	db *pgxpool.Pool
}

// NewPostgresSessionRepository constructs a PostgresSessionRepository.
func NewPostgresSessionRepository(db *pgxpool.Pool) *PostgresSessionRepository {
	return &PostgresSessionRepository{db: db}
}

// RevokeSession inserts a revocation record for sid. Idempotent via ON CONFLICT DO NOTHING.
func (r *PostgresSessionRepository) RevokeSession(ctx context.Context, sid, userID string) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO revoked_sessions (sid, user_id, revoked_at)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (sid) DO NOTHING`,
		sid, userID,
	)
	if err != nil {
		return apperrors.Internal(err)
	}
	return nil
}

// IsRevoked reports whether sid is present in the revocation table.
// Fails open on any storage error so that a transient DB outage does not
// lock out all authenticated users; the iat-based max-age check in
// PolicyProvider still enforces session lifetime independently.
func (r *PostgresSessionRepository) IsRevoked(ctx context.Context, sid string) (bool, error) {
	var found bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM revoked_sessions WHERE sid = $1)`,
		sid,
	).Scan(&found)
	if err != nil {
		return false, nil // fail-open
	}
	return found, nil
}

// PruneRevoked deletes revocation records created before olderThan and returns
// the number of rows removed.
func (r *PostgresSessionRepository) PruneRevoked(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM revoked_sessions WHERE revoked_at < $1`,
		olderThan,
	)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ SessionRepository = (*PostgresSessionRepository)(nil)
