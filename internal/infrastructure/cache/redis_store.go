package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/metric"
)

// RedisStore implements Store using Redis GET/SET/DEL commands with JSON
// serialisation. It is a thin wrapper around a *redis.Client and shares the
// same connection pool used by the event bus, keeping the dependency count low.
//
// All operations propagate context cancellation so a cancelled HTTP request
// does not block waiting for a slow Redis round-trip.
type RedisStore struct {
	client *redis.Client
	hits   metric.Int64Counter // nil when metrics not registered
	misses metric.Int64Counter
}

// NewRedisStore constructs a RedisStore backed by the provided client.
// The client is not owned by the store; the caller must close it after the
// store is no longer needed.
func NewRedisStore(client *redis.Client) *RedisStore {
	return &RedisStore{client: client}
}

// RegisterMetrics wires hit/miss counters into the provided meter.  Call once
// after construction; safe to skip in tests or when metrics are disabled.
func (s *RedisStore) RegisterMetrics(meter metric.Meter) error {
	var err error
	s.hits, err = meter.Int64Counter(
		"redis.cache.hits",
		metric.WithDescription("Number of Redis cache hits (key found and decoded successfully)."),
	)
	if err != nil {
		return err
	}
	s.misses, err = meter.Int64Counter(
		"redis.cache.misses",
		metric.WithDescription("Number of Redis cache misses (key absent, expired, or undecodeable)."),
	)
	return err
}

// Get retrieves the value stored under key and JSON-unmarshals it into dest.
// Returns ErrCacheMiss when the key does not exist or has expired.
func (s *RedisStore) Get(ctx context.Context, key string, dest interface{}) error {
	raw, err := s.client.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		if s.misses != nil {
			s.misses.Add(ctx, 1)
		}
		return ErrCacheMiss
	}
	if err != nil {
		return fmt.Errorf("cache get %q: %w", key, err)
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		// Corrupted or schema-changed cached value - treat as a miss so the
		// caller can repopulate with the current schema.
		if s.misses != nil {
			s.misses.Add(ctx, 1)
		}
		return ErrCacheMiss
	}
	if s.hits != nil {
		s.hits.Add(ctx, 1)
	}
	return nil
}

// Set JSON-marshals value and stores it under key with the given TTL.
// A ttl of zero stores the value without expiry (use sparingly).
func (s *RedisStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("cache set %q: marshal: %w", key, err)
	}
	if err := s.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %q: %w", key, err)
	}
	return nil
}

// Delete removes one or more keys in a single DEL command.
// Missing keys are silently ignored (Redis DEL behaviour).
func (s *RedisStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := s.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete: %w", err)
	}
	return nil
}

// FlushByPrefix deletes all keys whose names begin with prefix using a
// cursor-based SCAN loop. Each page of matches is deleted in a single DEL to
// minimise round-trips. The operation is best-effort and not atomic: keys
// written concurrently during the scan may not be evicted.
func (s *RedisStore) FlushByPrefix(ctx context.Context, prefix string) error {
	var cursor uint64
	for {
		keys, next, err := s.client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return fmt.Errorf("cache flush prefix %q: scan: %w", prefix, err)
		}
		if len(keys) > 0 {
			if err := s.client.Del(ctx, keys...).Err(); err != nil {
				return fmt.Errorf("cache flush prefix %q: del: %w", prefix, err)
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}

var _ PrefixFlusher = (*RedisStore)(nil)
