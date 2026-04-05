package events

// EventType identifies the kind of domain event carried by an Envelope.
//
// Using a named string type rather than bare strings prevents silent routing
// failures caused by typos: a mismatch between publisher and subscriber
// becomes a compile-time error when constants are used exclusively.
type EventType string

const (
	// EventMatchStarted is emitted when a match transitions to Live status.
	// Subscribers may use this to lock predictions for the match.
	EventMatchStarted EventType = "match.started"

	// EventMatchFinished is emitted when a match result is confirmed.
	// The ScoringService subscribes to this event to calculate player points.
	EventMatchFinished EventType = "match.finished"

	// EventPredictionMade is emitted when a user submits or updates a prediction.
	// Useful for analytics and for sending confirmation notifications.
	EventPredictionMade EventType = "prediction.made"
)
