package cache

import (
	"context"
	"time"
)

// ErrCacheMiss is returned by Store.Get when the requested key does not exist
// in the cache or has expired. Callers must treat this as a normal condition
// (not an error requiring alerting) and fall back to the authoritative source.
var ErrCacheMiss = errCacheMiss("cache miss")

type errCacheMiss string

func (e errCacheMiss) Error() string { return string(e) }

// Store defines a generic key-value cache with TTL-based expiry.
//
// Implementations must be safe for concurrent use by multiple goroutines.
// Get deserialises the cached value into dest; dest must be a non-nil pointer.
// Set serialises value and stores it under key for at most ttl. A ttl of zero
// means "store forever" on implementations that support it, but callers should
// always pass a finite TTL to prevent unbounded cache growth.
// Delete removes one or more keys in a single round-trip where possible.
//
// The Store interface is intentionally narrow. Operations that are not needed
// by the application today (e.g. atomic increments, list pushes) are omitted
// to keep the interface stable and implementors minimal.
type Store interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Delete(ctx context.Context, keys ...string) error
}
