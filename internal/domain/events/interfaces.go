package events

// TODO: define the Publisher and Subscriber interfaces for the event bus.
//
// Publisher is the outbound port: the service layer depends on it to emit
// domain events without knowing whether the underlying transport is an
// in-memory channel, Redis pub/sub, or a message broker.
//
// Subscriber is the inbound port: background workers implement it to
// consume events and trigger side effects (scoring, notifications).
//
// Keeping these interfaces in the domain package rather than in
// internal/infrastructure/messaging ensures that the domain layer owns
// the contract and infrastructure implementations adapt to it,
// not the other way around.
