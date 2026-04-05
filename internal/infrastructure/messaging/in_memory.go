// Package messaging provides implementations of the event bus interfaces
// defined in internal/domain/events.
//
// Multiple backends are available and selected at startup via configuration:
//   - in_memory.go  — synchronous, in-process delivery; used in tests and
//     single-replica local development only. Events are lost
//     on process restart and cannot cross process boundaries.
//   - redis.go      — asynchronous pub/sub via Redis; suitable for multi-replica
//     deployments where low-latency delivery matters more than
//     durability.
//
// All implementations satisfy the events.Bus interface defined in
// internal/domain/events/interfaces.go.
package messaging

import (
	"context"
	"sync"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// InMemoryBus is a synchronous, in-process implementation of events.Bus.
//
// Handlers registered via Subscribe are called directly within the Publish
// call on the publisher's goroutine. This makes the bus entirely synchronous
// and deterministic, which is ideal for unit tests that need to assert on
// side effects caused by domain events without introducing concurrency.
//
// InMemoryBus must not be used in multi-replica deployments because events
// published by one process are invisible to processes running on other hosts.
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[events.EventType][]func(context.Context, events.Envelope)
}

// NewInMemoryBus constructs an empty InMemoryBus ready for use.
func NewInMemoryBus() *InMemoryBus {
	return &InMemoryBus{
		handlers: make(map[events.EventType][]func(context.Context, events.Envelope)),
	}
}

// Subscribe registers handler to be called for every event of the given type.
// Multiple handlers may be registered for the same type; they are called in
// registration order.
func (b *InMemoryBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish delivers envelope synchronously to all handlers registered for its
// type. If no handler is registered the call is a no-op. Handler panics are
// not recovered here; the caller's recovery middleware handles them.
func (b *InMemoryBus) Publish(_ context.Context, envelope events.Envelope) error {
	b.mu.RLock()
	handlers := b.handlers[envelope.Type]
	b.mu.RUnlock()

	// Use a fresh background context for handler execution so that a
	// cancelled request context does not prevent event side effects from
	// completing (e.g. scoring must finish even if the HTTP client disconnected).
	ctx := context.Background()
	for _, h := range handlers {
		h(ctx, envelope)
	}
	return nil
}
