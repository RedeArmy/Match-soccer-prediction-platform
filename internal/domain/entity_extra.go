package domain

import "time"

// ExtraType identifies one of the fixed set of match "extras" — optional
// bonus predictions beyond the scoreline that award points when guessed
// correctly. Unlike ScoringRule (one row per tournament phase), the same
// fixed pair of extra types applies uniformly to every match; there is no
// per-match or per-phase configuration.
type ExtraType string

// Allowed values for ExtraType.
const (
	// ExtraTypeFirstScorer asks which team scores first. Valid answers are
	// "home", "away", or "none" (the match finished 0-0).
	ExtraTypeFirstScorer ExtraType = "first_scorer"
	// ExtraTypeHalftimeResult asks who is ahead at half-time. Valid answers
	// are "home", "draw", or "away".
	ExtraTypeHalftimeResult ExtraType = "halftime_result"
)

// AllExtraTypes is the ordered list of every ExtraType. Single source of
// truth for admin listing endpoints and validation.
var AllExtraTypes = [...]ExtraType{ExtraTypeFirstScorer, ExtraTypeHalftimeResult}

// Default point values used when no active extra_rules row exists for a
// type. Mirrors the ScoringRule IsActive fallback safety net, but skips the
// intermediate system_params layer scoring_rules needed for seven phases —
// two fixed, global knobs don't warrant that extra plumbing.
const (
	DefaultExtraFirstScorerPoints    = 3 // 3-way guess resolved from goal-event data
	DefaultExtraHalftimeResultPoints = 2 // 3-way guess, comparable odds to a basic outcome pick
)

// ExtraRule defines the point value awarded for correctly guessing one
// extra type. IsActive provides the same soft-disable switch as ScoringRule:
// setting it false falls back to the compile-time default above.
type ExtraRule struct {
	ID        int
	ExtraType ExtraType
	Points    int
	IsActive  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ExtraRuleInput carries the mutable fields for an extra rule update,
// mirroring ScoringRuleInput's shape.
type ExtraRuleInput struct {
	Points   int
	IsActive bool
}

// ExtraPrediction is one user's guess for one extra type on one match.
// Points is nil until ExtraScorer.ScoreExtras resolves it — either because
// the match has not finished yet, or because the match's underlying answer
// field (Match.FirstScoringTeam / halftime scores) was never resolved (e.g.
// a match finished before this feature existed). ScoredAt distinguishes
// "not yet scored" from "scored with zero points earned".
type ExtraPrediction struct {
	ID        int
	UserID    int
	MatchID   int
	ExtraType ExtraType
	Answer    string // validated against ExtraType's allowed value set at the service layer
	Points    *int
	ScoredAt  *time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}
