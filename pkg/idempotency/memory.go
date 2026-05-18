package idempotency

import (
	"context"
	"sync"
	"time"
)

type memEntry struct {
	Entry
	expiresAt time.Time
}

// MemoryStore is a concurrency-safe, in-process idempotency store backed by a
// plain map. Expired entries are pruned lazily on Load and Reserve so no
// background goroutine is required.
//
// Suitable for single-process deployments. Multi-instance deployments should
// use a shared store (e.g. Redis) so that reservations and committed responses
// are visible across all replicas.
type MemoryStore struct {
	mu      sync.Mutex
	entries map[string]memEntry
}

// NewMemoryStore constructs an empty MemoryStore.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{entries: make(map[string]memEntry)}
}

// Load returns the entry for key if it exists and has not expired.
func (s *MemoryStore) Load(_ context.Context, key string) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[key]
	if !ok || time.Now().After(e.expiresAt) {
		delete(s.entries, key) // prune expired entry
		return Entry{}, false, nil
	}
	return e.Entry, true, nil
}

// Reserve atomically creates an InFlight entry for key if none exists.
// Returns (true, nil) when the reservation is granted; (false, nil) when key
// is already present and has not expired.
func (s *MemoryStore) Reserve(_ context.Context, key string, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if e, ok := s.entries[key]; ok && !time.Now().After(e.expiresAt) {
		return false, nil // already claimed
	}
	s.entries[key] = memEntry{
		Entry:     Entry{State: InFlight},
		expiresAt: time.Now().Add(ttl),
	}
	return true, nil
}

// Commit replaces an InFlight entry with the committed response.
func (s *MemoryStore) Commit(_ context.Context, key string, e Entry, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = memEntry{Entry: e, expiresAt: time.Now().Add(ttl)}
	return nil
}

// Release removes the InFlight reservation for key.
func (s *MemoryStore) Release(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

var _ Store = (*MemoryStore)(nil)
