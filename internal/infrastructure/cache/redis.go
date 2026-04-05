// Package cache provides the Redis client used for caching and pub/sub
// messaging across the application.
//
// Redis serves two distinct purposes in this service:
//  1. Response caching — reducing repeated database reads for frequently
//     accessed, slowly changing data such as the published match schedule.
//  2. Pub/sub messaging — delivering domain events between the API server
//     and the worker process when in-order, at-least-once delivery is
//     acceptable and the overhead of a dedicated broker is not justified.
//
// If these two concerns grow sufficiently complex, consider separating them
// into distinct packages with their own connection pools.
package cache

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Config holds the parameters required to connect to Redis.
type Config struct {
	Addr     string
	Password string
	DB       int
}

// NewClient constructs a *redis.Client and verifies connectivity via Ping
// before returning it to the caller.
//
// A failed Ping causes an immediate error so that the application fails fast
// at startup rather than surfacing connection issues on the first real
// operation.
func NewClient(ctx context.Context, cfg Config) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis: ping: %w", err)
	}
	return client, nil
}
