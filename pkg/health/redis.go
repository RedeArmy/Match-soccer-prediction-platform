package health

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisChecker implements Checker for a Redis client.
// It sends a PING command and waits for the PONG reply.
type RedisChecker struct {
	client *redis.Client
}

// NewRedisChecker returns a RedisChecker backed by client.
func NewRedisChecker(client *redis.Client) *RedisChecker {
	return &RedisChecker{client: client}
}

// Name returns "redis", the key used in the readiness response JSON.
func (c *RedisChecker) Name() string { return "redis" }

// Check sends a PING to Redis and returns the result. Latency is measured
// from the moment PING is issued to the moment PONG is received.
func (c *RedisChecker) Check(ctx context.Context) Result {
	start := time.Now()
	if err := c.client.Ping(ctx).Err(); err != nil {
		return Result{Status: "error", Error: err.Error()}
	}
	return Result{Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
}
