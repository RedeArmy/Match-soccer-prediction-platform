package events

import "context"

// Publisher is the outbound port for emitting domain events.
//
// The service layer depends on this interface rather than on a concrete
// transport implementation. This keeps business logic portable across
// different messaging backends (in-memory for tests, Redis for production)
// without any change to the service code.
//
// Publish is synchronous from the caller's perspective: it returns once the
// event has been accepted by the bus. Whether delivery to subscribers is
// synchronous or asynchronous is an implementation detail hidden behind this
// interface.
type Publisher interface {
	Publish(ctx context.Context, envelope Envelope) error
}

// Subscriber is the inbound port for consuming domain events.
//
// Subscribe registers a handler function that is invoked for each event
// whose Type matches the given EventType. A single handler may be registered
// for multiple event types by calling Subscribe once per type.
//
// The handler receives the full Envelope; the concrete payload is accessible
// via a type assertion on Envelope.Payload using the expected event struct.
//
// Handlers must return an error to signal transient failures. The bus
// implementation retries the handler with exponential backoff before routing
// the event to the dead-letter queue.
type Subscriber interface {
	Subscribe(eventType EventType, handler func(ctx context.Context, envelope Envelope) error)
}

// Bus combines Publisher and Subscriber into a single interface for
// components (such as the composition root) that need to both publish
// and register handlers on the same bus instance.
type Bus interface {
	Publisher
	Subscriber
}
