package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisSessionStartCache wraps a SessionStarter with a Redis read-through cache.
// On a cache hit the DB round-trip is skipped; on a Redis error the DB is queried
// directly (fail-open). This reduces per-request DB load in horizontal deployments
// where multiple replicas share the same Redis cluster — the in-process startCache
// in PolicyProvider absorbs repeated lookups within a single replica, while this
// layer absorbs the first lookup after a replica restart or load-balancer switch.
//
// Key schema: "ss:{sid}", value: Unix timestamp in seconds (int64, string-encoded).
// TTL: auth.session_max_age_seconds + 86400 — entries expire after the session
// itself can no longer pass the max-age check, bounding Redis memory growth.
type RedisSessionStartCache struct {
	inner      SessionStarter
	rc         redis.Cmdable
	maxAgeSecs int
	log        *zap.Logger
}

// NewRedisSessionStartCache constructs a caching wrapper around inner.
// maxAgeSecs should be auth.session_max_age_seconds read at startup.
func NewRedisSessionStartCache(inner SessionStarter, rc redis.Cmdable, maxAgeSecs int, log *zap.Logger) *RedisSessionStartCache {
	return &RedisSessionStartCache{inner: inner, rc: rc, maxAgeSecs: maxAgeSecs, log: log}
}

// UpsertSessionStart returns the session origin timestamp for sid. It checks
// Redis first; on a cache miss it delegates to the inner starter, then stores
// the result in Redis. Redis errors are fail-open: a Redis failure falls through
// to the inner starter without propagating an error to the caller.
func (c *RedisSessionStartCache) UpsertSessionStart(ctx context.Context, sid, userID string) (time.Time, error) {
	key := fmt.Sprintf("ss:%s", sid)

	raw, err := c.rc.Get(ctx, key).Result()
	if err == nil {
		if unix, parseErr := strconv.ParseInt(raw, 10, 64); parseErr == nil {
			return time.Unix(unix, 0), nil
		}
	} else if err != redis.Nil {
		c.log.Warn("session start cache: Redis GET failed, falling through to DB",
			zap.String("sid", sid), zap.Error(err))
	}

	t, err := c.inner.UpsertSessionStart(ctx, sid, userID)
	if err != nil {
		return t, err
	}

	ttl := time.Duration(c.maxAgeSecs+86400) * time.Second
	if setErr := c.rc.Set(ctx, key, strconv.FormatInt(t.Unix(), 10), ttl).Err(); setErr != nil {
		c.log.Warn("session start cache: Redis SET failed; continuing without cache",
			zap.String("sid", sid), zap.Error(setErr))
	}
	return t, nil
}

var _ SessionStarter = (*RedisSessionStartCache)(nil)
