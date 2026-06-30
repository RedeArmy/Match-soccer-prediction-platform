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

// RevokeAllUserSessions bulk-inserts all session_starts rows for userID into
// revoked_sessions, effectively force-logging out all known OAuth sessions for
// the account. Returns the number of sessions revoked.
//
// Rows without a session_starts entry (fva-present sessions that never hit the
// OAuth path) are not affected; callers should also revoke the current session
// via RevokeSession to cover the initiating device.
func (r *PostgresSessionRepository) RevokeAllUserSessions(ctx context.Context, userID string) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`INSERT INTO revoked_sessions (sid, user_id, revoked_at)
		 SELECT sid, user_id, NOW()
		 FROM session_starts
		 WHERE user_id = $1
		 ON CONFLICT (sid) DO NOTHING`,
		userID,
	)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ SessionRepository = (*PostgresSessionRepository)(nil)

// PostgresSessionStartRepository is the PostgreSQL-backed implementation of
// SessionStartRepository.
type PostgresSessionStartRepository struct {
	db      *pgxpool.Pool
	metrics *sessionStartMetrics // nil until RegisterSessionStartMetrics is called
}

// NewPostgresSessionStartRepository constructs a PostgresSessionStartRepository.
func NewPostgresSessionStartRepository(db *pgxpool.Pool) *PostgresSessionStartRepository {
	return &PostgresSessionStartRepository{db: db}
}

// UpsertSessionStart inserts a new row for sid with the current timestamp on
// first call, then returns the stored started_at. The upsert uses
// "DO UPDATE SET started_at = session_starts.started_at" — a no-op update
// that forces Postgres to include the existing row in the RETURNING clause,
// making this a single round-trip regardless of whether the row is new.
// user_id is stored on insert only; subsequent upserts (ON CONFLICT) leave
// the stored user_id unchanged so the original attribution is preserved.
func (r *PostgresSessionStartRepository) UpsertSessionStart(ctx context.Context, sid, userID string) (time.Time, error) {
	start := time.Now()
	clientIP := nullStr(ClientIPFromContext(ctx))
	userAgent := nullStr(UserAgentFromContext(ctx))
	var t time.Time
	err := r.db.QueryRow(ctx,
		`INSERT INTO session_starts (sid, user_id, started_at, client_ip, user_agent)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (sid) DO UPDATE
		   SET started_at = session_starts.started_at
		 RETURNING started_at`,
		sid, userID, start, clientIP, userAgent,
	).Scan(&t)
	if err != nil {
		return time.Time{}, apperrors.Internal(err)
	}
	r.recordUpsert(ctx, start)
	return t, nil
}

// PruneSessionStarts deletes records created before olderThan and returns the
// number of rows removed.
func (r *PostgresSessionStartRepository) PruneSessionStarts(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM session_starts WHERE started_at < $1`,
		olderThan,
	)
	if err != nil {
		return 0, apperrors.Internal(err)
	}
	return tag.RowsAffected(), nil
}

var _ SessionStartRepository = (*PostgresSessionStartRepository)(nil)

// nullStr converts a non-empty string to a *string for nullable SQL columns.
// An empty string (absent header or unauthenticated path) becomes nil → NULL.
func nullStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
