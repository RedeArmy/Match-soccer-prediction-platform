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

import "time"

// Envelope wraps any domain event payload with routing metadata.
//
// The Type field is used by the bus to route the event to the correct
// subscribers. OccurredAt records when the state change happened, not when
// the event was delivered, so that consumers can build accurate audit trails
// even if delivery is delayed.
type Envelope struct {
	Type       EventType
	OccurredAt time.Time
	Payload    any
}

// MatchStarted is emitted when a match transitions from Scheduled to Live.
type MatchStarted struct {
	MatchID   int
	HomeTeam  string
	AwayTeam  string
	KickoffAt time.Time
}

// MatchFinished is emitted when a match result is confirmed.
//
// HomeScore and AwayScore carry the final scoreline so that consumers
// (ScoringService) do not need to re-fetch the match from the database
// for the common case where only the score is needed.
type MatchFinished struct {
	MatchID   int
	HomeTeam  string
	AwayTeam  string
	HomeScore int
	AwayScore int
}

// PredictionMade is emitted when a user submits or updates a prediction.
type PredictionMade struct {
	PredictionID int
	UserID       int
	MatchID      int
	HomeScore    int
	AwayScore    int
}
