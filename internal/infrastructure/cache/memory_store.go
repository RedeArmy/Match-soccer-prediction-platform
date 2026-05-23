package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// MemoryStore is a non-expiring, goroutine-safe in-memory implementation of
// Store and PrefixFlusher. TTLs are accepted but not enforced — entries never
// expire automatically.
//
// Intended for two use cases:
//  1. Unit tests that need a Store without starting a Redis process.
//  2. The no-Redis code path in the composition root, where a short-lived
//     in-process cache is preferable to nil and avoids nil-guard branches
//     throughout the codebase.
//
// Never use MemoryStore in multi-replica production deployments: invalidations
// are local and entries grow without bound. Use RedisStore (wrapped in
// ResilientStore) for all production scenarios.
type MemoryStore struct {
	mu   sync.RWMutex
	data map[string][]byte
}

// NewMemoryStore constructs an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{data: make(map[string][]byte)}
}

// Get retrieves the value stored under key and JSON-unmarshals it into dest.
// Returns ErrCacheMiss when the key does not exist.
func (s *MemoryStore) Get(_ context.Context, key string, dest interface{}) error {
	s.mu.RLock()
	raw, ok := s.data[key]
	s.mu.RUnlock()
	if !ok {
		return ErrCacheMiss
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		return ErrCacheMiss
	}
	return nil
}

// Set JSON-marshals value and stores it under key. The ttl argument is
// accepted for interface compatibility but is not enforced — the value
// persists until explicitly deleted or the store is discarded.
func (s *MemoryStore) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("memory_store set %q: %w", key, err)
	}
	s.mu.Lock()
	s.data[key] = data
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

// FlushByPrefix deletes all keys whose names begin with prefix.
// The operation is atomic with respect to concurrent Get/Set/Delete calls.
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

// Len returns the number of entries currently in the store.
// Intended for test assertions only.
func (s *MemoryStore) Len() int {
	s.mu.RLock()
	n := len(s.data)
	s.mu.RUnlock()
	return n
}

var _ Store = (*MemoryStore)(nil)
var _ PrefixFlusher = (*MemoryStore)(nil)
