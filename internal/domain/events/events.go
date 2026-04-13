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

import (
	"time"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

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

// Validate checks that the Envelope carries the minimum required metadata.
// Type must be non-empty, OccurredAt must be set, and Payload must not be nil.
func (e Envelope) Validate() error {
	if e.Type == "" {
		return apperrors.Validation("envelope: Type must not be empty")
	}
	if e.OccurredAt.IsZero() {
		return apperrors.Validation("envelope: OccurredAt must not be zero")
	}
	if e.Payload == nil {
		return apperrors.Validation("envelope: Payload must not be nil")
	}
	return nil
}

// MatchStarted is emitted when a match transitions from Scheduled to Live.
type MatchStarted struct {
	MatchID   int
	HomeTeam  string
	AwayTeam  string
	KickoffAt time.Time
}

// Validate enforces the invariants of MatchStarted.
// MatchID must be positive, team names must be non-empty, and KickoffAt must be set.
func (e MatchStarted) Validate() error {
	if e.MatchID <= 0 {
		return apperrors.Validation("MatchStarted: MatchID must be positive")
	}
	if e.HomeTeam == "" {
		return apperrors.Validation("MatchStarted: HomeTeam must not be empty")
	}
	if e.AwayTeam == "" {
		return apperrors.Validation("MatchStarted: AwayTeam must not be empty")
	}
	if e.KickoffAt.IsZero() {
		return apperrors.Validation("MatchStarted: KickoffAt must not be zero")
	}
	return nil
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

// Validate enforces the invariants of MatchFinished.
// MatchID must be positive, team names must be non-empty, and scores must be non-negative.
func (e MatchFinished) Validate() error {
	if e.MatchID <= 0 {
		return apperrors.Validation("MatchFinished: MatchID must be positive")
	}
	if e.HomeTeam == "" {
		return apperrors.Validation("MatchFinished: HomeTeam must not be empty")
	}
	if e.AwayTeam == "" {
		return apperrors.Validation("MatchFinished: AwayTeam must not be empty")
	}
	if e.HomeScore < 0 {
		return apperrors.Validation("MatchFinished: HomeScore must not be negative")
	}
	if e.AwayScore < 0 {
		return apperrors.Validation("MatchFinished: AwayScore must not be negative")
	}
	return nil
}

// PredictionMade is emitted when a user submits or updates a prediction.
type PredictionMade struct {
	PredictionID int
	UserID       int
	MatchID      int
	HomeScore    int
	AwayScore    int
}

// Validate enforces the invariants of PredictionMade.
// PredictionID, UserID, and MatchID must be positive; scores must be non-negative.
func (e PredictionMade) Validate() error {
	if e.PredictionID <= 0 {
		return apperrors.Validation("PredictionMade: PredictionID must be positive")
	}
	if e.UserID <= 0 {
		return apperrors.Validation("PredictionMade: UserID must be positive")
	}
	if e.MatchID <= 0 {
		return apperrors.Validation("PredictionMade: MatchID must be positive")
	}
	if e.HomeScore < 0 {
		return apperrors.Validation("PredictionMade: HomeScore must not be negative")
	}
	if e.AwayScore < 0 {
		return apperrors.Validation("PredictionMade: AwayScore must not be negative")
	}
	return nil
}
