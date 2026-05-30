// Package hub implements a thread-safe in-process SSE (Server-Sent Events) hub.
//
// The hub maintains a registry of active client channels keyed by user ID and
// a per-connection ID string (crypto/rand hex).  Connect returns a receive
// channel and a cleanup func; callers must invoke the cleanup when the HTTP
// connection closes.  Broadcast delivers to every open connection for a given
// user; when a connection's buffer is full the event is dropped and the dropped
// counter is incremented — the SSE client will resynchronise via the
// Last-Event-ID mechanism.
//
// Dead-client eviction: each connEntry tracks consecutive failed sends. When
// the count reaches evictAfterDrops, the hub closes the channel and removes the
// entry under its write lock. The SSE handler detects the closed channel via
// "case n, ok := <-ch: if !ok { return }" and terminates the goroutine cleanly.
// The HTTP connection's defer-cleanup is a no-op for already-evicted entries
// because the map deletion is guarded by the same mutex.
//
// Per-user connection cap: Connect returns a nil channel when a user already
// holds maxConnsPerUser open connections. The caller must check for nil before
// entering the event loop and reject the request with 429. A zero value for
// maxConnsPerUser disables the cap (used by New and NewWithBufSize for backward
// compatibility with tests that open many connections per user).
//
// Design constraints:
//   - Zero external dependencies.
//   - Buffered channels of size 32 per connection.
//   - 30-second heartbeat is the responsibility of the SSE handler, not the hub.
//   - RWMutex: read lock (shared) for Broadcast; write lock for Connect/Disconnect/Evict.
//   - Supports ≥ 500 concurrent connections without data races (-race clean).
package hub

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

const (
	defaultChanBufSize = 32
	evictAfterDrops    = 5
)

// DefaultMaxConnsPerUser is the default maximum number of concurrent SSE
// connections allowed per authenticated user. NewWithOptions uses this value
// when Options.MaxConnsPerUser is not explicitly set. A value of 0 disables
// the cap entirely (unlimited connections).
const DefaultMaxConnsPerUser = 5

// Notification is the value delivered to SSE clients.
type Notification struct {
	ID        int64  `json:"id"`
	UserID    int    `json:"user_id"`
	EventType string `json:"event_type"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	ActionURL string `json:"action_url,omitempty"`
	CreatedAt string `json:"created_at"` // RFC3339
}

// connEntry holds the channel and backpressure state for a single SSE connection.
type connEntry struct {
	ch    chan Notification
	drops atomic.Int32 // consecutive failed non-blocking sends; resets on success
}

// connSnapshot is the data captured under the read lock for a single connection.
type connSnapshot struct {
	connID string
	entry  *connEntry
}

// Options configures a Hub at construction time.
type Options struct {
	// ChanBufSize is the per-connection channel buffer. Values ≤ 0 fall back
	// to defaultChanBufSize (32).
	ChanBufSize int
	// MaxConnsPerUser is the maximum number of concurrent SSE connections
	// allowed per user. Connect returns a nil channel when this limit is
	// reached. Values ≤ 0 disable the cap (unlimited).
	MaxConnsPerUser int
}

// Hub is a thread-safe registry of SSE client channels.
type Hub struct {
	mu              sync.RWMutex
	clients         map[int]map[string]*connEntry
	metrics         hubMetrics
	chanBufSize     int
	maxConnsPerUser int // 0 = unlimited
}

type hubMetrics struct {
	connections atomic.Int64
	broadcasts  atomic.Int64
	dropped     atomic.Int64
	evicted     atomic.Int64
	rejected    atomic.Int64 // connections refused due to per-user cap
}

// New constructs an empty Hub with the default channel buffer size and no
// per-user connection cap (unlimited). Use NewWithOptions in production to
// enforce DefaultMaxConnsPerUser.
func New() *Hub {
	return NewWithOptions(Options{})
}

// NewWithBufSize constructs an empty Hub with the given per-connection channel
// buffer size and no per-user connection cap. Values ≤ 0 fall back to the
// default (32). Use NewWithOptions in production to enforce DefaultMaxConnsPerUser.
func NewWithBufSize(n int) *Hub {
	return NewWithOptions(Options{ChanBufSize: n})
}

// NewWithOptions constructs a Hub with the given options.
// ChanBufSize ≤ 0 falls back to 32; MaxConnsPerUser ≤ 0 disables the cap.
func NewWithOptions(opts Options) *Hub {
	if opts.ChanBufSize <= 0 {
		opts.ChanBufSize = defaultChanBufSize
	}
	return &Hub{
		clients:         make(map[int]map[string]*connEntry),
		chanBufSize:     opts.ChanBufSize,
		maxConnsPerUser: opts.MaxConnsPerUser,
	}
}

// Connect registers a new SSE connection for userID.
// It returns a receive-only channel and a cleanup function the caller must
// invoke (typically via defer) when the HTTP connection closes.
//
// When MaxConnsPerUser is set and the user has reached the limit, Connect
// returns (nil, no-op). The caller must check for a nil channel and reject
// the request (HTTP 429) before entering the event loop; reading from a nil
// channel blocks forever.
//
// The cleanup is safe to call even after the hub has evicted the connection:
// a missing map entry means eviction already closed the channel and decremented
// the connections counter, so the cleanup becomes a no-op.
func (h *Hub) Connect(userID int) (<-chan Notification, func()) {
	connID := newConnID()
	e := &connEntry{ch: make(chan Notification, h.chanBufSize)}

	h.mu.Lock()
	if h.maxConnsPerUser > 0 && len(h.clients[userID]) >= h.maxConnsPerUser {
		h.mu.Unlock()
		h.metrics.rejected.Add(1)
		return nil, func() {}
	}
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[string]*connEntry)
	}
	h.clients[userID][connID] = e
	h.mu.Unlock()

	h.metrics.connections.Add(1)

	cleanup := func() {
		h.mu.Lock()
		if conns, ok := h.clients[userID]; ok {
			if _, exists := conns[connID]; exists {
				delete(conns, connID)
				if len(conns) == 0 {
					delete(h.clients, userID)
				}
				close(e.ch)
				h.metrics.connections.Add(-1)
			}
			// Entry absent: hub already evicted it (closed channel, decremented counter).
		}
		h.mu.Unlock()
	}
	return e.ch, cleanup
}

// Broadcast delivers n to every open connection for userID.
//
// ctx is used to start a child span so the broadcast appears in the trace of
// the caller (e.g. the pg_notify or Redis bridge goroutine). Pass
// context.Background() when no parent trace is available.
//
// Send behaviour:
//   - Successful send: resets the connection's consecutive-drop counter to 0.
//   - Full buffer (slow consumer): increments the counter and records a drop.
//     When the counter reaches evictAfterDrops the connection is evicted under
//     the write lock: its channel is closed, its entry removed, and the
//     connections metric is decremented.
//
// Eviction is safe under concurrent cleanup: whichever of the two acquires the
// write lock first performs the deletion and close; the second finds the entry
// absent and skips.
func (h *Hub) Broadcast(ctx context.Context, userID int, n Notification) {
	_, span := otel.Tracer("hub").Start(ctx, "hub.broadcast")
	span.SetAttributes(attribute.Int("user_id", userID))
	defer span.End()
	// Snapshot channels under the read lock so that a concurrent cleanup()
	// cannot modify the inner map while we iterate.
	h.mu.RLock()
	conns := h.clients[userID]
	if len(conns) == 0 {
		h.mu.RUnlock()
		return
	}
	snapshot := make([]connSnapshot, 0, len(conns))
	for cid, e := range conns {
		snapshot = append(snapshot, connSnapshot{connID: cid, entry: e})
	}
	h.mu.RUnlock()

	h.metrics.broadcasts.Add(1)

	var toEvict []string
	for _, s := range snapshot {
		select {
		case s.entry.ch <- n:
			s.entry.drops.Store(0) // reset consecutive-drop counter on success
		default:
			h.metrics.dropped.Add(1)
			if s.entry.drops.Add(1) >= evictAfterDrops {
				toEvict = append(toEvict, s.connID)
			}
		}
	}

	if len(toEvict) == 0 {
		return
	}

	h.mu.Lock()
	conns = h.clients[userID]
	for _, cid := range toEvict {
		if e, ok := conns[cid]; ok {
			delete(conns, cid)
			close(e.ch)
			h.metrics.connections.Add(-1)
			h.metrics.evicted.Add(1)
		}
		// Entry absent: cleanup() raced ahead or a prior Broadcast iteration
		// already evicted it; both are safe no-ops here.
	}
	if len(conns) == 0 {
		delete(h.clients, userID)
	}
	h.mu.Unlock()
}

// HasLocalConnection reports whether userID has at least one open SSE
// connection on this replica. In a multi-replica deployment this method
// returns false for users whose SSE connection is open on a different replica;
// cluster-level presence tracking requires a shared store (e.g. Redis).
// Use this only for replica-local optimisations, never for routing decisions.
func (h *Hub) HasLocalConnection(userID int) bool {
	h.mu.RLock()
	n := len(h.clients[userID])
	h.mu.RUnlock()
	return n > 0
}

// Metrics returns a snapshot of hub counters for observability.
func (h *Hub) Metrics() (connections, broadcasts, dropped int64) {
	return h.metrics.connections.Load(),
		h.metrics.broadcasts.Load(),
		h.metrics.dropped.Load()
}

// RejectedConnections returns the total number of Connect calls refused due to
// the per-user connection cap. Useful for alerting on abuse patterns.
func (h *Hub) RejectedConnections() int64 {
	return h.metrics.rejected.Load()
}

// RegisterMetrics wires up OTel instruments for the hub's counters.
// Call once at startup after the meter provider is initialised; it is a no-op
// when meter is nil. Exported metrics:
//
//   - notification.sse.connections   (UpDownCounter) current open SSE connections
//   - notification.sse.broadcasts    (Counter) cumulative Broadcast calls
//   - notification.sse.dropped       (Counter) cumulative dropped events
//   - notification.sse.evicted       (Counter) cumulative dead-client evictions
//
// In a multi-replica deployment, Prometheus aggregates per-replica gauges so
// operators can observe both the per-instance count and the cluster total via
// sum(notification_sse_connections).
func (h *Hub) RegisterMetrics(meter metric.Meter) error {
	if meter == nil {
		return nil
	}

	_, err := meter.Int64ObservableUpDownCounter(
		"notification.sse.connections",
		metric.WithDescription("Number of currently open SSE connections on this replica."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(h.metrics.connections.Load())
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableCounter(
		"notification.sse.broadcasts",
		metric.WithDescription("Cumulative SSE Broadcast calls since process start."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(h.metrics.broadcasts.Load())
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableCounter(
		"notification.sse.dropped",
		metric.WithDescription("Cumulative SSE events dropped due to full client buffers since process start."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(h.metrics.dropped.Load())
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableCounter(
		"notification.sse.evicted",
		metric.WithDescription("Cumulative SSE connections evicted for exceeding the consecutive-drop threshold."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(h.metrics.evicted.Load())
			return nil
		}),
	)
	if err != nil {
		return err
	}

	_, err = meter.Int64ObservableCounter(
		"notification.sse.rejected",
		metric.WithDescription("Cumulative SSE Connect calls refused due to the per-user connection cap."),
		metric.WithInt64Callback(func(_ context.Context, o metric.Int64Observer) error {
			o.Observe(h.metrics.rejected.Load())
			return nil
		}),
	)
	return err
}

func newConnID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
