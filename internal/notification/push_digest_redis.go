package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisPushDigestGate implements DigestGate using Redis as the shared backing
// store for push-delivery window counters. Unlike the in-memory PushDigestGate,
// it enforces the threshold cluster-wide: all worker replicas share the same
// counter so a user cannot receive more than threshold individual pushes per
// window regardless of which replica handles their outbox entries.
//
// Key schema: "push_digest:{userID}", TTL = windowSec seconds.
// The TTL is set only on the first increment of each window; subsequent
// increments within the same window do not reset the expiry. This matches
// the semantics of PushDigestGate: the window is anchored to the first
// delivery, not the most recent one.
//
// Redis errors degrade gracefully: when the INCR script fails, Record returns
// (true, 0) so the push is delivered individually rather than dropped. This is
// intentionally conservative — missing a digest collapse is less harmful than
// silently dropping a notification due to a transient Redis failure.
type RedisPushDigestGate struct {
	client    redis.Cmdable
	windowSec int64
	threshold int32
}

// incrWindowScript atomically increments the per-user push counter and sets
// the window TTL only when the key is new (count == 1). Anchoring the TTL
// to the first increment means the window is fixed-width from the first push,
// not sliding — matching the in-memory implementation's expiry semantics.
//
// KEYS[1] = counter key; ARGV[1] = TTL in whole seconds.
// Returns the new counter value after the increment.
var incrWindowScript = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then
    redis.call('EXPIRE', KEYS[1], ARGV[1])
end
return count
`)

// NewRedisPushDigestGate constructs a gate backed by the given Redis client.
// windowSec is the window duration in seconds; threshold is the maximum number
// of individual pushes allowed per user per window before the digest path is
// activated. Both values should be read from system parameters at startup.
func NewRedisPushDigestGate(client redis.Cmdable, windowSec int64, threshold int32) *RedisPushDigestGate {
	return &RedisPushDigestGate{client: client, windowSec: windowSec, threshold: threshold}
}

// Record classifies a push delivery attempt and increments the Redis window
// counter for the given user. now is accepted for DigestGate interface
// compatibility and is not used; window expiry is managed by Redis TTL.
//
// On Redis error, Record degrades to (true, 0): the push is delivered
// individually. Callers must not interpret a Redis failure as a reason to drop.
func (g *RedisPushDigestGate) Record(ctx context.Context, userID int, p Priority, _ time.Time) (sendIndividual bool, digestCount int32) {
	if p <= PriorityP1High {
		return true, 0
	}

	key := fmt.Sprintf("push_digest:%d", userID)
	count, err := incrWindowScript.Run(ctx, g.client, []string{key}, g.windowSec).Int64()
	if err != nil {
		return true, 0 // degrade: send individual push rather than drop
	}

	c := int32(count)
	switch {
	case c <= g.threshold:
		return true, 0
	case c == g.threshold+1:
		return false, c
	default:
		return false, 0
	}
}

var _ DigestGate = (*RedisPushDigestGate)(nil)
