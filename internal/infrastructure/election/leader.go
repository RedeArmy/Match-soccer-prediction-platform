// Package election provides leader election primitives for multi-replica
// worker deployments.
//
// In a multi-replica deployment each replica runs the same goroutines,
// including the DLQ monitor. Without coordination every replica would log DLQ
// state at the same interval, producing duplicate log lines and making
// log-based alerting unreliable. A LeaderElection implementation ensures that
// only one replica executes guarded sections per interval.
//
// Two implementations are provided:
//
//   - PgLeaderElection: PostgreSQL session-level advisory lock. The lock is
//     held for the lifetime of the dedicated connection and is released
//     automatically on crash or restart — no TTL management required. Preferred
//     for new deployments because it adds no additional infrastructure dependency.
//
//   - RedisLeaderElection: Redis SET NX PX lock. Tick-scoped: each call to
//     TryAcquire competes fresh. Conservative on Redis failure (returns false).
//     Retained for backwards compatibility with environments where PostgreSQL
//     session locks are not suitable.
package election

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// LeaderElection is the interface for leader election primitives. Implementations
// must be safe for concurrent use.
type LeaderElection interface {
	// TryAcquire attempts to acquire the leader lock. Returns true when this
	// instance holds the lock, false when another holder owns it or acquisition
	// fails.
	TryAcquire(ctx context.Context) bool
	// Close releases any held lock and frees associated resources. It is safe
	// to call Close when the lock was never acquired.
	Close(ctx context.Context)
}

// RedisLeaderElection is a single-use, tick-scoped leader lock backed by
// Redis SET NX PX. Each call to TryAcquire attempts to claim the lock for
// ttl duration; only one replica wins per ttl window.
type RedisLeaderElection struct {
	rc  *redis.Client
	key string
	ttl time.Duration
	log *zap.Logger
}

// NewRedisLeaderElection constructs a RedisLeaderElection.
//
// key must be unique to the role being guarded (e.g. "worker:dlq-monitor:leader").
// ttl should be slightly longer than the interval between TryAcquire calls so
// the lock expires before the next tick even on a crashed winner.
func NewRedisLeaderElection(rc *redis.Client, key string, ttl time.Duration, log *zap.Logger) *RedisLeaderElection {
	return &RedisLeaderElection{rc: rc, key: key, ttl: ttl, log: log}
}

// TryAcquire attempts to acquire the leader lock for one tick window.
// Returns true when this replica holds the lock; false when another replica
// already holds it or Redis is unreachable. The conservative false-on-error
// behaviour prevents split-brain: when Redis is down, no replica runs the
// guarded section rather than all of them running it simultaneously.
//
// TryAcquire satisfies LeaderElection.
func (e *RedisLeaderElection) TryAcquire(ctx context.Context) bool {
	// SET key value NX PX ttl — atomically set only when key does not exist.
	// Returns "OK" when the lock was acquired, redis.Nil when another holder
	// already owns it. Any other error is treated as a failure to acquire.
	val, err := e.rc.SetArgs(ctx, e.key, "1", redis.SetArgs{
		Mode: "NX",
		TTL:  e.ttl,
	}).Result()
	if err != nil && err != redis.Nil {
		e.log.Warn("leader election: SET NX failed, skipping tick",
			zap.String("key", e.key),
			zap.Error(err),
		)
		return false
	}
	return val == "OK"
}

// Close is intentionally empty for RedisLeaderElection. The SET NX PX lock
// expires naturally when its TTL elapses, so no explicit release call is
// needed. Attempting to DEL the key here would introduce a race: a slow
// winner could release a lock already re-acquired by a new leader on the
// next tick. Close exists solely to satisfy the LeaderElection interface.
func (e *RedisLeaderElection) Close(_ context.Context) {}
