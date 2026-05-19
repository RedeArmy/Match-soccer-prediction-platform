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
// Design constraints:
//   - Zero external dependencies.
//   - Buffered channels of size 32 per connection.
//   - 30-second heartbeat is the responsibility of the SSE handler, not the hub.
//   - RWMutex: read lock (shared) for Broadcast; write lock for Connect/Disconnect.
//   - Supports ≥ 500 concurrent connections without data races (-race clean).
package hub

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"sync/atomic"
)

const chanBufSize = 32

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

// Hub is a thread-safe registry of SSE client channels.
type Hub struct {
	mu      sync.RWMutex
	clients map[int]map[string]chan Notification
	metrics hubMetrics
}

type hubMetrics struct {
	connections atomic.Int64
	broadcasts  atomic.Int64
	dropped     atomic.Int64
}

// New constructs an empty Hub ready for use.
func New() *Hub {
	return &Hub{clients: make(map[int]map[string]chan Notification)}
}

// Connect registers a new SSE connection for userID.
// It returns a receive-only channel and a cleanup function the caller must
// invoke (typically via defer) when the HTTP connection closes.
func (h *Hub) Connect(userID int) (<-chan Notification, func()) {
	connID := newConnID()
	ch := make(chan Notification, chanBufSize)

	h.mu.Lock()
	if h.clients[userID] == nil {
		h.clients[userID] = make(map[string]chan Notification)
	}
	h.clients[userID][connID] = ch
	h.mu.Unlock()

	h.metrics.connections.Add(1)

	cleanup := func() {
		h.mu.Lock()
		if conns, ok := h.clients[userID]; ok {
			delete(conns, connID)
			if len(conns) == 0 {
				delete(h.clients, userID)
			}
		}
		h.mu.Unlock()
		h.metrics.connections.Add(-1)
		close(ch)
	}
	return ch, cleanup
}

// Broadcast delivers n to every open connection for userID.
// If a connection's buffer is full the notification is dropped for that
// connection and the dropped metric is incremented.
func (h *Hub) Broadcast(userID int, n Notification) {
	// Snapshot channels under the read lock so that a concurrent cleanup()
	// cannot modify the inner map while we iterate.  Sending to the buffered
	// channels happens after the lock is released to keep the critical section
	// short; the channels themselves are goroutine-safe.
	h.mu.RLock()
	conns := h.clients[userID]
	if len(conns) == 0 {
		h.mu.RUnlock()
		return
	}
	snapshot := make([]chan Notification, 0, len(conns))
	for _, ch := range conns {
		snapshot = append(snapshot, ch)
	}
	h.mu.RUnlock()

	h.metrics.broadcasts.Add(1)
	for _, ch := range snapshot {
		select {
		case ch <- n:
		default:
			h.metrics.dropped.Add(1)
		}
	}
}

// HasConnection reports whether userID has at least one open SSE connection.
func (h *Hub) HasConnection(userID int) bool {
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

func newConnID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
