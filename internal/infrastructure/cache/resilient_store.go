package cache

import (
	"context"
	"errors"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

// ResilientStore wraps a Store with a circuit breaker so that a Redis outage
// does not cascade into application errors.
//
// Degradation behaviour when the circuit is open:
//   - Get:           returns ErrCacheMiss (caller fetches from the source of truth).
//   - Set:           silently dropped (next request repopulates from the source).
//   - Delete:        silently skipped (stale entries expire via their TTL).
//   - FlushByPrefix: silently skipped.
//
// ErrCacheMiss from the inner store is NOT counted as a Redis failure; a miss
// is the normal hot-path and must never contribute to opening the circuit.
// Only network errors, connection timeouts, and unexpected Redis errors count.
//
// The circuit opens after maxFails consecutive errors, stays open for openFor,
// then allows one trial request (half-open). A successful trial closes the
// circuit; a failed trial resets the cooldown.
type ResilientStore struct {
	inner   Store
	breaker *breaker.Breaker
	log     *zap.Logger
}

// NewResilientStore wraps inner with the given circuit breaker. The breaker
// should be named to identify the dependency (e.g. "redis-cache") for logs.
func NewResilientStore(inner Store, b *breaker.Breaker, log *zap.Logger) *ResilientStore {
	return &ResilientStore{inner: inner, breaker: b, log: log}
}

// Get retrieves a cached value. When the circuit is open, returns ErrCacheMiss
// immediately without calling the inner store. ErrCacheMiss from the inner
// store is passed through normally and does not count as a breaker failure.
func (s *ResilientStore) Get(ctx context.Context, key string, dest interface{}) error {
	var innerErr error
	breakerErr := s.breaker.Call(func() error {
		innerErr = s.inner.Get(ctx, key, dest)
		// A cache miss is expected behaviour, not a Redis error. Do not count
		// it against the failure threshold.
		if errors.Is(innerErr, ErrCacheMiss) {
			return nil
		}
		return innerErr
	})
	if errors.Is(breakerErr, breaker.ErrOpen) {
		s.log.Warn("cache.Get short-circuited: circuit open",
			zap.String("key", key),
			zap.String("breaker", s.breaker.Name()),
		)
		return ErrCacheMiss
	}
	return innerErr
}

// Set stores a value. Silently drops the write when the circuit is open to
// avoid hammering a degraded Redis instance with writes that will fail anyway.
func (s *ResilientStore) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	err := s.breaker.Call(func() error {
		return s.inner.Set(ctx, key, value, ttl)
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("cache.Set short-circuited: circuit open",
			zap.String("key", key),
			zap.String("breaker", s.breaker.Name()),
		)
		return nil
	}
	return err
}

// Delete removes keys. Silently skips when the circuit is open; stale entries
// expire naturally via their TTL.
func (s *ResilientStore) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	err := s.breaker.Call(func() error {
		return s.inner.Delete(ctx, keys...)
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("cache.Delete short-circuited: circuit open",
			zap.String("breaker", s.breaker.Name()),
			zap.Int("key_count", len(keys)),
		)
		return nil
	}
	return err
}

// FlushByPrefix implements PrefixFlusher. Silently skips when the circuit is
// open; stale entries for the prefix expire naturally.
func (s *ResilientStore) FlushByPrefix(ctx context.Context, prefix string) error {
	pf, ok := s.inner.(PrefixFlusher)
	if !ok {
		return nil
	}
	err := s.breaker.Call(func() error {
		return pf.FlushByPrefix(ctx, prefix)
	})
	if errors.Is(err, breaker.ErrOpen) {
		s.log.Warn("cache.FlushByPrefix short-circuited: circuit open",
			zap.String("prefix", prefix),
			zap.String("breaker", s.breaker.Name()),
		)
		return nil
	}
	return err
}

// compile-time interface assertions
var _ Store = (*ResilientStore)(nil)
var _ PrefixFlusher = (*ResilientStore)(nil)
