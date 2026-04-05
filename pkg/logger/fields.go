package logger

import "go.uber.org/zap"

// Reusable zap.Field constructors with canonical key names.
//
// Using these constructors rather than inline zap.String / zap.Int calls
// ensures that the same field name is used wherever a concept is logged,
// preventing key-name drift that breaks log-aggregation queries.

// UserID returns a structured log field for a Clerk user identifier.
func UserID(id string) zap.Field { return zap.String("user_id", id) }

// MatchID returns a structured log field for a match identifier.
func MatchID(id int) zap.Field { return zap.Int("match_id", id) }

// QuinielaID returns a structured log field for a quiniela identifier.
func QuinielaID(id int) zap.Field { return zap.Int("quiniela_id", id) }

// PredictionID returns a structured log field for a prediction identifier.
func PredictionID(id int) zap.Field { return zap.Int("prediction_id", id) }

// RequestID returns a structured log field for an HTTP request identifier.
func RequestID(id string) zap.Field { return zap.String("request_id", id) }

// EventType returns a structured log field for a domain event type string.
func EventType(t string) zap.Field { return zap.String("event_type", t) }

// LatencyMS returns a structured log field for a duration expressed in
// milliseconds.
func LatencyMS(ms int64) zap.Field { return zap.Int64("latency_ms", ms) }
