// Package messaging provides implementations of the event bus interfaces
// defined in internal/domain/events.
//
// Multiple backends are available and selected at startup via configuration:
//   - in_memory.go  — synchronous, in-process delivery; used in tests and
//                     single-replica local development only. Events are lost
//                     on process restart and cannot cross process boundaries.
//   - redis.go      — asynchronous pub/sub via Redis; suitable for multi-replica
//                     deployments where low-latency delivery matters more than
//                     durability.
//
// All implementations must satisfy the events.Publisher and events.Subscriber
// interfaces defined in internal/domain/events/interfaces.go.
package messaging

// TODO: implement InMemoryPublisher and InMemorySubscriber.
// Use a sync.Map or a map of channels keyed by event type.
// This implementation is intentionally simple and non-durable;
// it is the correct choice for unit and integration tests.
