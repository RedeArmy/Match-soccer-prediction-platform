package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// memEntry holds a cached value together with its expiry time.
type memEntry struct {
	data      []byte
	expiresAt time.Time // zero value = never expires
}

func (e memEntry) expired(now time.Time) bool {
	return !e.expiresAt.IsZero() && now.After(e.expiresAt)
}

// MemoryStore is a goroutine-safe in-memory implementation of Store and
// PrefixFlusher with lazy TTL eviction.
//
// Entries set with a positive TTL expire on the next Get after their deadline
// passes (lazy eviction — no background goroutine). Entries set with a zero TTL
// never expire, preserving the behaviour expected by unit tests and the
// composition-root no-Redis fallback.
//
// Intended for two use cases:
//  1. Unit tests that need a Store without starting a Redis process.
//  2. The no-Redis code path in the composition root, where a short-lived
//     in-process cache is preferable to nil and avoids nil-guard branches
//     throughout the codebase.
//
// Never use MemoryStore in multi-replica production deployments: invalidations
// are local. Use RedisStore (wrapped in ResilientStore) for all production
// scenarios.
type MemoryStore struct {
	mu      sync.RWMutex
	data    map[string]memEntry
	nowFunc func() time.Time // injectable for tests; defaults to time.Now
}

// NewMemoryStore constructs an empty MemoryStore backed by the real wall clock.
func NewMemoryStore() *MemoryStore {
	return newMemoryStoreWithClock(time.Now)
}

// newMemoryStoreWithClock constructs a MemoryStore that calls nowFn instead of
// time.Now. Used in tests to drive expiry without real sleeps.
func newMemoryStoreWithClock(nowFn func() time.Time) *MemoryStore {
	return &MemoryStore{
		data:    make(map[string]memEntry),
		nowFunc: nowFn,
	}
}

// Get retrieves the value stored under key and JSON-unmarshals it into dest.
// Returns ErrCacheMiss when the key does not exist or has expired (the expired
// entry is lazily removed on first access).
func (s *MemoryStore) Get(_ context.Context, key string, dest interface{}) error {
	now := s.nowFunc()

	// Fast path: read lock for non-expired entries.
	s.mu.RLock()
	e, ok := s.data[key]
	s.mu.RUnlock()

	if !ok {
		return ErrCacheMiss
	}
	if e.expired(now) {
		// Lazy eviction: upgrade to write lock and delete the stale entry.
		s.mu.Lock()
		if entry, still := s.data[key]; still && entry.expired(now) {
			delete(s.data, key)
		}
		s.mu.Unlock()
		return ErrCacheMiss
	}
	if err := json.Unmarshal(e.data, dest); err != nil {
		return ErrCacheMiss
	}
	return nil
}

// Set JSON-marshals value and stores it under key with the given TTL.
// A zero TTL stores the entry without an expiry (it persists until explicitly
// deleted or the store is discarded). A positive TTL causes the entry to be
// treated as expired after the deadline passes and lazily evicted on next Get.
func (s *MemoryStore) Set(_ context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("memory_store set %q: %w", key, err)
	}
	e := memEntry{data: data}
	if ttl > 0 {
		e.expiresAt = s.nowFunc().Add(ttl)
	}
	s.mu.Lock()
	s.data[key] = e
	s.mu.Unlock()
	return nil
}

// Delete removes one or more keys. Missing keys are silently ignored.
func (s *MemoryStore) Delete(_ context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	s.mu.Lock()
	for _, k := range keys {
		delete(s.data, k)
	}
	s.mu.Unlock()
	return nil
}

// FlushByPrefix deletes all keys whose names begin with prefix, regardless of
// their expiry state. The operation is atomic with respect to concurrent
// Get/Set/Delete calls.
func (s *MemoryStore) FlushByPrefix(_ context.Context, prefix string) error {
	s.mu.Lock()
	for k := range s.data {
		if strings.HasPrefix(k, prefix) {
			delete(s.data, k)
		}
	}
	s.mu.Unlock()
	return nil
}

// Len returns the number of entries currently in the store, including entries
// that have expired but have not yet been lazily evicted.
// Intended for test assertions only.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	n := len(s.data)
	s.mu.RUnlock()
	return n
}

var _ Store = (*MemoryStore)(nil)
var _ PrefixFlusher = (*MemoryStore)(nil)
