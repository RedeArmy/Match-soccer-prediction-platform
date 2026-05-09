// Package election provides a Redis-backed leader election primitive.
//
// In a multi-replica worker deployment each replica runs the same goroutines,
// including the DLQ monitor. Without coordination every replica would log DLQ
// state at the same interval, producing duplicate log lines and making
// log-based alerting unreliable. RedisLeaderElection solves this by turning
// each tick into a competition: only the replica that wins a Redis SET NX PX
// lock executes the guarded section for that interval.
//
// This is a "best-effort" leader, not a strict lease:
//   - If Redis is unavailable, TryAcquire returns false conservatively, so the
//     guarded section is skipped rather than run by all replicas.
//   - Lock TTL is set to slightly longer than the tick interval so the lock
//     expires before the next tick even if the winner replica crashes without
//     explicitly releasing it.
//   - There is no explicit Release: the lock expires naturally, avoiding the
//     release-under-crash edge case.
package election

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

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
