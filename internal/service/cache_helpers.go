package service

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
)

const msgCacheGetFallingThrough = "cache get failed, falling through to db"

// cacheGet checks the cache for key and deserialises the result into *v.
// Returns (value, true) on a hit. On a miss it returns the zero value and
// false. On any other error it logs a warning and also returns false so the
// caller falls through to the database - cache errors are never fatal.
func cacheGet[T any](ctx context.Context, store cache.Store, key string, log *zap.Logger) (T, bool) {
	var v T
	if err := store.Get(ctx, key, &v); err == nil {
		return v, true
	} else if !errors.Is(err, cache.ErrCacheMiss) {
		log.Warn(msgCacheGetFallingThrough, zap.String("key", key), zap.Error(err))
	}
	return v, false
}

// cacheSet writes value to the cache with the given TTL. Failures are logged
// as warnings and swallowed: a write failure means the next request pays a
// cache miss, not that the response is wrong.
func cacheSet(ctx context.Context, store cache.Store, key string, value any, ttl time.Duration, log *zap.Logger) {
	if err := store.Set(ctx, key, value, ttl); err != nil {
		log.Warn("cache set failed", zap.String("key", key), zap.Error(err))
	}
}
