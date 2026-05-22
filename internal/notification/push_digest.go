// Package notification provides the push digest gate: a per-user sliding-window
// rate limiter that collapses burst P2/P3 push notifications into a single
// summary push, preventing notification spam for active users during concurrent
// match events.
package notification

import (
	"context"
	"sync"
	"time"
)

// DigestGate classifies web-push delivery attempts for a given user and
// priority, collapsing burst P2/P3 notifications into a single digest push.
// Implementations must be safe for concurrent use from multiple goroutines.
//
// Two implementations are provided:
//   - PushDigestGate: in-memory, zero-dependency. Correct for a single-process
//     deployment; in a multi-replica worker each replica has independent state.
//   - RedisPushDigestGate: Redis-backed, cluster-wide counter. Enforces the
//     threshold across all worker replicas so users cannot receive more than
//     threshold individual pushes per window regardless of which replica handles
//     their events. Prefer this when Redis is available.
type DigestGate interface {
	// Record classifies a push delivery attempt for userID at priority p.
	// now is the current wall-clock time; in-process implementations use it
	// for window expiry, Redis-backed implementations ignore it (TTL is
	// managed by Redis).
	//
	// Returns:
	//   (true,  0)     P0/P1 — bypass gate; send individual push immediately.
	//   (true,  0)     P2/P3, count ≤ threshold — send individual push.
	//   (false, count) P2/P3, first overflow — send one digest push.
	//   (false, 0)     P2/P3, subsequent overflow — drop silently.
	Record(ctx context.Context, userID int, p Priority, now time.Time) (sendIndividual bool, digestCount int32)
}

// PushDigestGate implements a per-user sliding-window gate for web-push
// delivery. P0 and P1 events always bypass the gate. For P2 and P3 events it
// tracks how many pushes were sent to a user within the last windowSec seconds:
//
//   - count ≤ threshold   → send individual push (normal path)
//   - count == threshold+1 → send exactly one digest push ("you have N updates")
//   - count > threshold+1  → drop silently (digest already sent for this window)
//
// PushDigestGate is safe for concurrent use by multiple goroutines.
// It has zero external dependencies and makes no network or DB calls.
type PushDigestGate struct {
	windowSec int64
	threshold int32

	mu      sync.Mutex
	windows map[int]*digestWindow
}

type digestWindow struct {
	count     int32
	expiresAt time.Time
}

// NewPushDigestGate constructs a gate with a sliding window of windowSec seconds
// and a per-window delivery threshold. When threshold pushes have been sent to
// a user within the window, subsequent P2/P3 events trigger a single digest push
// instead of individual deliveries.
func NewPushDigestGate(windowSec int64, threshold int32) *PushDigestGate {
	return &PushDigestGate{
		windowSec: windowSec,
		threshold: threshold,
		windows:   make(map[int]*digestWindow),
	}
}

// Record classifies and records a push delivery attempt for userID at priority p.
// ctx is accepted for DigestGate interface compatibility and is not used.
func (g *PushDigestGate) Record(_ context.Context, userID int, p Priority, now time.Time) (sendIndividual bool, digestCount int32) {
	if p <= PriorityP1High {
		return true, 0
	}

	g.mu.Lock()
	defer g.mu.Unlock()

	w := g.windows[userID]
	if w == nil || now.After(w.expiresAt) {
		w = &digestWindow{expiresAt: now.Add(time.Duration(g.windowSec) * time.Second)}
		g.windows[userID] = w
	}

	w.count++

	switch {
	case w.count <= g.threshold:
		return true, 0
	case w.count == g.threshold+1:
		return false, w.count
	default:
		return false, 0
	}
}

var _ DigestGate = (*PushDigestGate)(nil)

// Prune removes digest windows whose sliding interval has expired. It should be
// called periodically (e.g., by a background ticker) to bound memory growth for
// large user bases. Users with an active window are never removed.
// Not part of the DigestGate interface; only PushDigestGate (in-process) needs
// explicit pruning. Redis-backed implementations rely on key TTL for cleanup.
func (g *PushDigestGate) Prune(now time.Time) {
	g.mu.Lock()
	defer g.mu.Unlock()
	for uid, w := range g.windows {
		if now.After(w.expiresAt) {
			delete(g.windows, uid)
		}
	}
}
