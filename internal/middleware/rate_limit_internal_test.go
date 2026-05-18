package middleware

import (
	"testing"
	"time"

	"golang.org/x/time/rate"
)

// TestLimiterStore_EvictLocked_RemovesStaleEntries calls evictLocked directly
// to verify that entries whose lastSeen exceeds the TTL are removed from the map.
// This is an internal (white-box) test because evictLocked is unexported and
// only reachable from within the package.
func TestLimiterStore_EvictLocked_RemovesStaleEntries(t *testing.T) {
	store := NewLimiterStore(10.0, 10)

	// Manually insert two entries with different lastSeen times.
	past := time.Now().Add(-(limiterTTL + time.Second))
	store.mu.Lock()
	store.entries["stale"] = &limiterEntry{lim: rate.NewLimiter(store.r, store.burst), lastSeen: past}
	store.entries["fresh"] = &limiterEntry{lim: rate.NewLimiter(store.r, store.burst), lastSeen: time.Now()}
	store.evictLocked()
	store.mu.Unlock()

	if _, ok := store.entries["stale"]; ok {
		t.Error("stale entry was not evicted")
	}
	if _, ok := store.entries["fresh"]; !ok {
		t.Error("fresh entry was incorrectly evicted")
	}
}
