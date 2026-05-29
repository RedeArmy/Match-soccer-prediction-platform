package middleware

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
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
	rc            redis.UniversalClient
	burst         int
	log           *zap.Logger
	failOpenTotal metric.Int64Counter // incremented on every Redis-unavailable fail-open
}

// NewRedisRateStore constructs a RedisRateStore. ratePerSec is accepted for
// API symmetry with NewLimiterStore but is unused — the burst cap controls the
// per-second allowance in the fixed-window model.
func NewRedisRateStore(rc redis.UniversalClient, _ float64, burst int, log *zap.Logger) *RedisRateStore {
	return &RedisRateStore{rc: rc, burst: burst, log: log}
}

// RegisterMetrics wires OTel instruments into the store. Call once at startup
// after the global meter provider is initialised (same pattern as
// paymentWebhook.RegisterMetrics). Safe to skip in tests; nil counter is a
// no-op in Allow.
func (s *RedisRateStore) RegisterMetrics(meter metric.Meter) error {
	c, err := meter.Int64Counter(
		"wcq_rate_limit_fail_open_total",
		metric.WithDescription("Number of requests that bypassed Redis rate limiting because Redis was unavailable. "+
			"Non-zero values indicate degraded multi-replica rate limiting."),
	)
	if err != nil {
		return fmt.Errorf("register wcq_rate_limit_fail_open_total: %w", err)
	}
	s.failOpenTotal = c
	return nil
}

// Allow increments the fixed-window counter for key and reports whether the
// request is within the burst limit for the current second.
// Fail-open: returns (true, 0) on any Redis error and increments failOpenTotal.
func (s *RedisRateStore) Allow(ctx context.Context, key string) (bool, int) {
	sec := time.Now().Unix()
	rk := fmt.Sprintf("rl:%s:%d", key, sec)

	count, err := s.rc.Incr(ctx, rk).Result()
	if err != nil {
		s.log.Warn("redis rate limiter: INCR failed, failing open",
			zap.String("key", key), zap.Error(err))
		if s.failOpenTotal != nil {
			s.failOpenTotal.Add(ctx, 1)
		}
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
