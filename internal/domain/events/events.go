// Package events defines the domain events emitted when significant state
// transitions occur within the quiniela system.
//
// Domain events decouple the action that causes a state change from the
// downstream reactions to it. For example, when a match transitions to
// Finished, the service layer emits a MatchFinished event rather than
// calling the ScoringService directly. This allows multiple subscribers
// (scoring, notifications, ranking recalculation) to react independently
// and be added or removed without modifying the code that raises the event.
//
// Events in this package are pure data structures. They carry no behaviour
// and have no dependencies on infrastructure — they are safe to construct
// and inspect in unit tests without any external setup.
package events

// TODO: define event structs for state transitions:
//   MatchStarted   — emitted when a match transitions to Live
//   MatchFinished  — emitted when a result is confirmed; triggers scoring
//   PredictionMade — emitted when a user submits or updates a prediction
