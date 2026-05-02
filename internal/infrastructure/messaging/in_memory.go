// Package messaging provides implementations of the event bus interfaces
// defined in internal/domain/events.
//
// Multiple backends are available and selected at startup via configuration:
//   - in_memory.go  - synchronous, in-process delivery; used in tests and
//     single-replica local development only. Events are lost
//     on process restart and cannot cross process boundaries.
//   - redis.go      - asynchronous pub/sub via Redis; suitable for multi-replica
//     deployments where low-latency delivery matters more than
//     durability.
//
// All implementations satisfy the events.Bus interface defined in
// internal/domain/events/interfaces.go.
package messaging

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"go.uber.org/zap"

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
	handlers map[events.EventType][]func(context.Context, events.Envelope) error
	log      *zap.Logger
}

// NewInMemoryBus constructs an empty InMemoryBus ready for use.
// If log is nil a no-op logger is used so that tests do not need to provide one.
func NewInMemoryBus(log *zap.Logger) *InMemoryBus {
	if log == nil {
		log = zap.NewNop()
	}
	return &InMemoryBus{
		handlers: make(map[events.EventType][]func(context.Context, events.Envelope) error),
		log:      log,
	}
}

// Subscribe registers handler to be called for every event of the given type.
// Multiple handlers may be registered for the same type; they are called in
// registration order. Handlers must return an error to signal transient
// failures; the bus will retry up to maxHandlerAttempts times before logging
// the event as a dead-letter.
func (b *InMemoryBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope) error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventType] = append(b.handlers[eventType], handler)
}

// Publish delivers envelope synchronously to all handlers registered for its
// type. Each handler is retried up to maxHandlerAttempts times on failure;
// events that exhaust all attempts are logged as dead-letters so they can be
// replayed manually. If no handler is registered the call is a no-op.
func (b *InMemoryBus) Publish(ctx context.Context, envelope events.Envelope) error {
	b.mu.RLock()
	handlers := b.handlers[envelope.Type]
	b.mu.RUnlock()

	// Detach cancellation from the caller's context so that a cancelled HTTP
	// request does not abort event side-effects (e.g. scoring must complete
	// even if the client disconnected). context.WithoutCancel preserves any
	// values carried by ctx (trace IDs, request IDs) so they remain available
	// to handlers for structured logging and distributed tracing - something
	// context.Background() would silently discard.
	// handlerCtx strips cancellation so a cancelled HTTP request does not
	// abort event side-effects, while preserving trace/request-ID values.
	// The original ctx (which may be cancelled) is passed to callWithRetry
	// so that the inter-attempt sleep is interrupted if the caller cancels,
	// preventing a stuck goroutine during tests or graceful shutdown.
	handlerCtx := context.WithoutCancel(ctx)
	for _, h := range handlers {
		h := h // capture loop variable
		if err := callWithRetry(ctx, func() error { return h(handlerCtx, envelope) }); err != nil {
			b.logDLQ(envelope, err)
		}
	}
	return nil
}

// logDLQ records a dead-letter event as a structured error log. InMemoryBus
// has no external store, so this log entry is the only recovery artifact.
// Operators can use the logged envelope JSON to replay the event manually.
func (b *InMemoryBus) logDLQ(envelope events.Envelope, handlerErr error) {
	raw, _ := json.Marshal(envelope)
	b.log.Error("event handler failed after all retries - dead-lettered",
		zap.String("event_type", string(envelope.Type)),
		zap.Time("occurred_at", envelope.OccurredAt),
		zap.Time("dead_lettered_at", time.Now().UTC()),
		zap.Int("attempts", maxHandlerAttempts),
		zap.String("envelope_json", string(raw)),
		zap.Error(handlerErr),
	)
}
