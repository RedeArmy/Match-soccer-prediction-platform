// Package idempotency provides a generic idempotency store and the types used
// by the HTTP middleware in internal/middleware/idempotency.go.
//
// The store records the lifecycle of a client-supplied Idempotency-Key through
// two states:
//
//   - InFlight: the first request for this key is being processed; concurrent
//     requests with the same key are rejected with 409 Conflict.
//   - Committed: the request completed with a 2xx status; subsequent requests
//     with the same key receive the cached response without re-executing the
//     handler.
//
// Only successful (2xx) responses are committed. Error responses release the
// reservation so clients can retry after fixing the underlying problem.
package idempotency

import (
	"context"
	"net/http"
	"time"
)

// EntryState describes the lifecycle phase of an idempotency key.
type EntryState int8

const (
	// InFlight marks a key whose first request is still being processed.
	InFlight EntryState = 1
	// Committed marks a key whose first request completed with a 2xx response.
	Committed EntryState = 2
)

// Entry holds either an in-flight sentinel or a captured HTTP response.
type Entry struct {
	State      EntryState
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// Store manages idempotency entries. Implementations must be safe for
// concurrent use from multiple goroutines.
type Store interface {
	// Load returns the entry for key and true if the key is known, or zero
	// Entry and false if the key has never been seen (or has expired).
	Load(ctx context.Context, key string) (Entry, bool, error)

	// Reserve atomically records key as InFlight if it is not already known.
	// Returns (true, nil) when the reservation is granted, (false, nil) when
	// the key already exists (either InFlight from a concurrent request or
	// Committed from a prior completed request).
	Reserve(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// Commit transitions key from InFlight to Committed and stores the
	// captured response. Must be called only after a successful Reserve.
	Commit(ctx context.Context, key string, e Entry, ttl time.Duration) error

	// Release removes an InFlight reservation without storing a response.
	// Called when the handler returns an error response so the client can
	// retry with the same key after resolving the underlying issue.
	Release(ctx context.Context, key string) error
}
