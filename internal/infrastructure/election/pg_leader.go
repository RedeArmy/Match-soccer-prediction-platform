package election

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// lockDB abstracts the two advisory-lock queries so PgLeaderElection can be
// tested without a real database connection.
type lockDB interface {
	queryBool(ctx context.Context, sql string, args ...any) (bool, error)
	close(ctx context.Context) error
}

type pgConnDB struct{ c *pgx.Conn }

func (d *pgConnDB) queryBool(ctx context.Context, sql string, args ...any) (bool, error) {
	var b bool
	return b, d.c.QueryRow(ctx, sql, args...).Scan(&b)
}

func (d *pgConnDB) close(ctx context.Context) error { return d.c.Close(ctx) }

// PgLeaderElection implements LeaderElection using a PostgreSQL session-level
// advisory lock. The lock is held for the lifetime of the dedicated connection
// and is released automatically when the connection closes — a process crash
// releases the lock without any TTL or explicit cleanup.
//
// Once acquired, TryAcquire returns true on every subsequent call without
// re-querying the database, so callers can check leadership on every tick
// without extra round-trips.
//
// A dedicated connection is used (not a pool connection) because advisory locks
// are session-scoped; sharing a pool connection would make the lock lifetime
// non-deterministic as connections are returned and reused.
type PgLeaderElection struct {
	db     lockDB
	lockID int64
	log    *zap.Logger
	held   bool
}

// NewPgLeaderElection opens a dedicated PostgreSQL connection and returns a
// PgLeaderElection. The connection is kept open for the process lifetime and
// closed by Close. Returns an error if the connection cannot be established.
func NewPgLeaderElection(ctx context.Context, dsn string, lockID int64, log *zap.Logger) (*PgLeaderElection, error) {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("election: open advisory lock connection: %w", err)
	}
	return &PgLeaderElection{db: &pgConnDB{c: conn}, lockID: lockID, log: log}, nil
}

// TryAcquire attempts to acquire the PostgreSQL session-level advisory lock
// identified by lockID using pg_try_advisory_lock. Returns true when this
// instance holds the lock; false when another session holds it or the query
// fails. Once the lock is held, subsequent calls return true without querying
// the database.
//
// TryAcquire satisfies LeaderElection.
func (e *PgLeaderElection) TryAcquire(ctx context.Context) bool {
	if e.held {
		return true
	}
	acquired, err := e.db.queryBool(ctx, "SELECT pg_try_advisory_lock($1)", e.lockID)
	if err != nil {
		e.log.Warn("election: pg_try_advisory_lock failed", zap.Error(err))
		return false
	}
	e.held = acquired
	if acquired {
		e.log.Info("election: acquired advisory lock", zap.Int64("lock_id", e.lockID))
	}
	return acquired
}

// Close explicitly releases the advisory lock with pg_advisory_unlock and then
// closes the dedicated connection. Safe to call when the lock was never
// acquired. Close satisfies LeaderElection.
func (e *PgLeaderElection) Close(ctx context.Context) {
	if e.held {
		if _, err := e.db.queryBool(ctx, "SELECT pg_advisory_unlock($1)", e.lockID); err != nil {
			e.log.Warn("election: pg_advisory_unlock failed", zap.Error(err))
		}
		e.held = false
	}
	if err := e.db.close(ctx); err != nil {
		e.log.Warn("election: close advisory lock connection failed", zap.Error(err))
	}
}
