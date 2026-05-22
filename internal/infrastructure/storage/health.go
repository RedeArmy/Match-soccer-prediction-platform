package storage

import (
	"context"
	"errors"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

const healthProbeKey = "__health_probe__"

// Checker implements health.Checker for a ResilientFileStore.
//
// When the circuit breaker is open the check short-circuits immediately and
// reports "degraded" — the provider is known-bad and no network probe is
// needed. Otherwise it probes the underlying store directly (bypassing the
// circuit breaker) so the result reflects the current state of the provider
// rather than the breaker's cached opinion.
//
// A Get for a non-existent sentinel key is treated as "ok": ErrNotFound means
// the provider is reachable; only transport-level or auth errors indicate a
// degraded provider.
type Checker struct {
	name  string
	store *ResilientFileStore
}

// NewChecker returns a Checker that probes store.
// name is the stable, lowercase key used in the /health/ready JSON response
// (e.g. "file-store").
func NewChecker(name string, store *ResilientFileStore) *Checker {
	return &Checker{name: name, store: store}
}

// Name returns the checker name used as the JSON field in /health/ready.
func (c *Checker) Name() string { return c.name }

// Check probes the underlying storage provider.
//
//   - Circuit open  → "degraded" immediately (no network call).
//   - ErrNotFound   → "ok" (provider reachable; sentinel key absent is expected).
//   - No error      → "ok" (provider reachable; sentinel key exists).
//   - Other error   → "error" with the error message.
func (c *Checker) Check(ctx context.Context) health.Result {
	if c.store.breaker.CurrentState() == breaker.StateOpen {
		return health.Result{Status: "degraded", Error: "circuit open"}
	}
	start := time.Now()
	_, _, err := c.store.inner.Get(ctx, healthProbeKey)
	if err == nil || errors.Is(err, ErrNotFound) {
		return health.Result{Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
	}
	return health.Result{Status: "error", Error: err.Error()}
}

var _ health.Checker = (*Checker)(nil)
