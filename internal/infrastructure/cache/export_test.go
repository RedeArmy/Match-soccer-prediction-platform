package cache

import "time"

// NewMemoryStoreWithClock constructs a MemoryStore using nowFn instead of
// time.Now. Exposed for package-external tests that need to drive TTL expiry
// without real sleeps.
func NewMemoryStoreWithClock(nowFn func() time.Time) *MemoryStore {
	return newMemoryStoreWithClock(nowFn)
}
