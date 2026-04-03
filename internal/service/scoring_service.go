package service

// TODO: implement ScoringService.
//
// Scoring rules (to be defined in internal/domain/constants.go):
//   - Exact score correct  → full points
//   - Correct outcome only (win/draw/loss) → partial points
//   - Incorrect outcome    → zero points
//
// ScoreMatch is triggered by consuming a MatchFinished domain event.
// It must be idempotent: if called twice for the same match (e.g. due to
// at-least-once event delivery), it must not double-award points.
