package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// RedisRateStore implements Allower using a Redis fixed-window counter.
// Each 1-second window is a separate key: "rl:{key}:{unix_second}".
// At most burst requests are allowed within each window.
//
// Redis errors are fail-open: a connectivity problem does not block traffic.
// The EXPIRE is set to 2 seconds (one full window beyond the current second)
// to guarantee the key is cleaned up while avoiding premature eviction.
type RedisRateStore struct {
	rc    redis.UniversalClient
	burst int
	log   *zap.Logger
}

// NewRedisRateStore constructs a RedisRateStore. ratePerSec is accepted for
// API symmetry with NewLimiterStore but is unused — the burst cap controls the
// per-second allowance in the fixed-window model.
func NewRedisRateStore(rc redis.UniversalClient, _ float64, burst int, log *zap.Logger) *RedisRateStore {
	return &RedisRateStore{rc: rc, burst: burst, log: log}
}

// Allow increments the fixed-window counter for key and reports whether the
// request is within the burst limit for the current second.
// Fail-open: returns (true, 0) on any Redis error.
func (s *RedisRateStore) Allow(ctx context.Context, key string) (bool, int) {
	sec := time.Now().Unix()
	rk := fmt.Sprintf("rl:%s:%d", key, sec)

	count, err := s.rc.Incr(ctx, rk).Result()
	if err != nil {
		s.log.Warn("redis rate limiter: INCR failed, failing open",
			zap.String("key", key), zap.Error(err))
		return true, 0
	}
	if count == 1 {
		if err := s.rc.Expire(ctx, rk, 2*time.Second).Err(); err != nil {
			s.log.Warn("redis rate limiter: EXPIRE failed",
				zap.String("key", key), zap.Error(err))
		}
	}
	if int(count) > s.burst {
		return false, 1
	}
	return true, 0
}
