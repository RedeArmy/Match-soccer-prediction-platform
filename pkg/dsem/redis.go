// Package dsem provides a Redis-backed distributed counting semaphore for
// limiting cluster-wide concurrency across multiple process replicas.
//
// The semaphore is implemented as a Redis counter with atomic INCR/DECR via
// Lua scripts. A TTL refreshed on every Acquire acts as a crash-safety net:
// if a holder process crashes before calling Release, the counter decays
// automatically within the configured TTL.
//
// Typical usage:
//
//	sem := dsem.New(rc, "myapp:sem:snapshots", 16, 5*time.Minute)
//	if err := sem.Acquire(ctx); err != nil {
//	    // Redis unavailable or ctx cancelled
//	}
//	defer sem.Release()
package dsem

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRetryInterval = 50 * time.Millisecond

// Semaphore is the distributed concurrency-limiting interface.
type Semaphore interface {
	// Acquire blocks until a slot is available or ctx is cancelled.
	// Returns ctx.Err() on cancellation; returns a wrapped error on Redis failure.
	// On Redis failure callers should decide whether to proceed without the
	// distributed limit (graceful degradation) or to propagate.
	Acquire(ctx context.Context) error
	// Release decrements the cluster-wide counter. Errors are silently discarded;
	// the key TTL provides eventual-consistency recovery on process crash.
	Release()
}

// RedisSemaphore implements Semaphore using a Redis counter. It is safe for
// concurrent use from multiple goroutines.
type RedisSemaphore struct {
	client        redis.Cmdable
	key           string
	limit         int64
	ttl           time.Duration
	retryInterval time.Duration
}

// acquireScript atomically increments the counter; if the new value exceeds
// the limit it decrements back and returns 0 (not acquired). Returns 1 on
// success. The EXPIRE is refreshed on every call so the TTL resets as long as
// work is being claimed — the key only truly expires when all holders have
// released or crashed.
//
// KEYS[1] = counter key; ARGV[1] = limit; ARGV[2] = TTL in whole seconds.
var acquireScript = redis.NewScript(`
local c = redis.call('INCR', KEYS[1])
redis.call('EXPIRE', KEYS[1], ARGV[2])
if c > tonumber(ARGV[1]) then
	redis.call('DECR', KEYS[1])
	return 0
end
return 1
`)

// releaseScript decrements the counter, flooring at zero to guard against
// double-release bugs and crash-recovery races where the key already expired.
//
// KEYS[1] = counter key.
var releaseScript = redis.NewScript(`
local c = redis.call('GET', KEYS[1])
if c and tonumber(c) > 0 then
	redis.call('DECR', KEYS[1])
end
return 1
`)

// New constructs a RedisSemaphore.
//
//   - client is a redis.Cmdable (Client, ClusterClient, or miniredis stub).
//   - key    is the Redis key used as the cluster-wide counter.
//   - limit  is the maximum number of concurrent slots across all replicas.
//   - ttl    is the key TTL refreshed on each Acquire; must exceed the
//     worst-case operation duration so the counter self-heals after a crash.
func New(client redis.Cmdable, key string, limit int64, ttl time.Duration) *RedisSemaphore {
	return &RedisSemaphore{
		client:        client,
		key:           key,
		limit:         limit,
		ttl:           ttl,
		retryInterval: defaultRetryInterval,
	}
}

// Acquire blocks until a slot is available, ctx is cancelled, or Redis returns
// an error. Calls spin with retryInterval between attempts.
func (s *RedisSemaphore) Acquire(ctx context.Context) error {
	ttlSec := int64(s.ttl.Seconds())
	if ttlSec < 1 {
		ttlSec = 1
	}
	for {
		acquired, err := acquireScript.Run(ctx, s.client, []string{s.key}, s.limit, ttlSec).Int64()
		if err != nil {
			return fmt.Errorf("dsem: acquire %q: %w", s.key, err)
		}
		if acquired == 1 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.retryInterval):
		}
	}
}

// Release decrements the cluster-wide counter. Uses context.Background()
// internally because the caller's context may already be cancelled by the time
// Release fires from a defer. Errors are silently discarded; the TTL acts as
// the safety net.
func (s *RedisSemaphore) Release() {
	_, _ = releaseScript.Run(context.Background(), s.client, []string{s.key}).Int64()
}

var _ Semaphore = (*RedisSemaphore)(nil)
