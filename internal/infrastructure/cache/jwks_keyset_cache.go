package cache

import (
	"context"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
)

const jwksCacheKey = "auth:jwks:keyset"

// JWKSKeysetCache persists the Clerk JWKS JSON payload in Redis so that
// JWKSProvider can seed its in-memory fallback after a process restart during
// a Clerk outage. It satisfies the auth.KeysetPersister interface without
// importing the auth package (dependency direction: wiring layer depends on both,
// neither depends on the other).
type JWKSKeysetCache struct {
	rc redis.UniversalClient
}

// NewJWKSKeysetCache constructs a JWKSKeysetCache backed by rc.
func NewJWKSKeysetCache(rc redis.UniversalClient) *JWKSKeysetCache {
	return &JWKSKeysetCache{rc: rc}
}

// Save writes payload to Redis with the given TTL. If TTL is zero the key is
// stored without expiry, which is not recommended for production.
func (c *JWKSKeysetCache) Save(ctx context.Context, payload []byte, ttl time.Duration) error {
	return c.rc.Set(ctx, jwksCacheKey, payload, ttl).Err()
}

// Load retrieves the last-saved JWKS payload. Returns (nil, nil) on a cache
// miss so that the caller can distinguish "no persisted keyset" from an error.
func (c *JWKSKeysetCache) Load(ctx context.Context) ([]byte, error) {
	val, err := c.rc.Get(ctx, jwksCacheKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	return val, err
}
