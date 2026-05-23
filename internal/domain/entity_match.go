package domain

import "time"

// ── Match phase ───────────────────────────────────────────────────────────────

// MatchPhase identifies the round of the tournament a fixture belongs to.
// FIFA World Cup 2026 expands to 48 teams, adding a round_of_32 between the
// group stage and the traditional round_of_16.
type MatchPhase string

// Allowed values for MatchPhase, ordered by tournament progression.
const (
	PhaseGroupStage   MatchPhase = "group_stage"
	PhaseRoundOf32    MatchPhase = "round_of_32"
	PhaseRoundOf16    MatchPhase = "round_of_16"
	PhaseQuarterFinal MatchPhase = "quarter_final"
	PhaseSemiFinal    MatchPhase = "semi_final"
	PhaseThirdPlace   MatchPhase = "third_place"
	PhaseFinal        MatchPhase = "final"
)

// ── Win method ────────────────────────────────────────────────────────────────

// WinMethod indicates how the winning team secured the knockout-phase result.
// It is nil for group-stage matches and for matches still in progress.
//
// WinMethodExtraTime means the winner scored a deciding goal during extra time.
// WinMethodPenalties means the match was level after extra time and the winner
// prevailed on a penalty shootout.
type WinMethod string

// Allowed values for WinMethod.
const (
	WinMethodNormal    WinMethod = "normal"     // decided within 90 minutes
	WinMethodExtraTime WinMethod = "extra_time" // decided in extra time (AET)
	WinMethodPenalties WinMethod = "penalties"  // decided on penalties (PSO)
)

// ── Match status ──────────────────────────────────────────────────────────────

// MatchStatus tracks the lifecycle of a fixture from announcement to result.
//
// Downstream services use this field to enforce business rules: predictions
// are only accepted whilst a match is Scheduled, scoring jobs are triggered
// when a match transitions to Finished, and live-score updates are streamed
// during the Live phase. Changes to this type must be coordinated with the
// corresponding database enum and any event consumers that branch on status.
type MatchStatus string

// Allowed values for MatchStatus.
const (
	MatchStatusScheduled MatchStatus = "scheduled"
	MatchStatusLive      MatchStatus = "live"
	MatchStatusFinished  MatchStatus = "finished"
)

// ── Match ─────────────────────────────────────────────────────────────────────

// Match represents a single World Cup fixture in the tournament schedule.
//
// HomeScore and AwayScore are pointers because a nil value is semantically
// distinct from zero: a score of 0-0 is a valid final result, whereas nil
// means the match has not yet been played or the result has not been
// confirmed. Using pointers makes this nullable semantics explicit at the
// type level, avoiding the need for a sentinel value (e.g. -1) that could
// be confused with a real score by accident.
//
// StadiumID is nullable: knockout-stage fixtures may be created before their
// venue is confirmed. Stadium is hydrated by the repository when loading a
// match with venue detail; it is nil when only the match metadata is needed.
//
// GroupLabel is nil for knockout matches and holds the FIFA group letter
// ("A"-"L") for group-stage fixtures. It drives real-time standings
// calculation without a separate teams table.
type Match struct {
	ID         int
	HomeTeam   string
	AwayTeam   string
	HomeScore  *int
	AwayScore  *int
	Status     MatchStatus
	Phase      MatchPhase
	GroupLabel *string    // nil for knockout; "A"-"L" for group stage
	WinMethod  *WinMethod // nil until match is finished; always nil for group-stage matches
	StadiumID  *int
	Stadium    *Stadium
	KickoffAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ── Scoring rule ──────────────────────────────────────────────────────────────

// ScoringRule defines the point values awarded for each prediction outcome
// within a specific tournament phase. Knockout rounds carry progressively
// higher point values than the group stage, rewarding correct predictions
// on higher-stakes fixtures.
//
// ExactScore, CorrectOutcome, and GoalDifference mirror the flat
// domain.PointsExact*, PointsCorrect*, PointsGoalDiff* constants but are
// scoped to a single phase row, allowing operators to adjust knockout-stage
// rewards through the admin API without redeploying the service.
//
// ExtraTimeBonus and PenaltiesBonus are additive bonuses applied on top of
// base points when a user correctly predicts that the winning team will advance
// via extra time (+ExtraTimeBonus) or penalties (+PenaltiesBonus). These bonuses
// are only applicable to knockout phases; group-stage rows must keep them at 0.
//
// IsActive provides a soft-disable switch: setting a phase to inactive falls
// back to the global system_params values at scoring time so a misconfigured
// rule can be rolled back immediately without a migration.
type ScoringRule struct {
	ID             int
	Phase          MatchPhase
	ExactScore     int
	CorrectOutcome int
	GoalDifference int
	ExtraTimeBonus int
	PenaltiesBonus int
	IsActive       bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ScoringRuleInput carries the mutable fields for a scoring rule update.
// Grouping them avoids an excessively long parameter list on Update.
type ScoringRuleInput struct {
	ExactScore     int
	CorrectOutcome int
	GoalDifference int
	ExtraTimeBonus int
	PenaltiesBonus int
	IsActive       bool
}

// ── Group standing ────────────────────────────────────────────────────────────

// GroupStanding is a read-only projection of one team's position within a
// FIFA World Cup group. It is computed in real time from finished group-stage
// matches and is never persisted to the database.
//
// Sorting within a group follows the official FIFA criteria:
//  1. Points DESC
//  2. Goal difference DESC
//  3. Goals scored DESC
//  4. Team name ASC (stable tie-break; FIFA uses head-to-head and then draw)
type GroupStanding struct {
	Group  string
	Team   string
	Played int
	Won    int
	Drawn  int
	Lost   int
	GF     int // goals for
	GC     int // goals against
	GD     int // goal difference = GF - GC
	Points int
}

// ── Tournament slot ───────────────────────────────────────────────────────────

// TournamentSlot represents one named bracket position confirmed by the system
// administrator once FIFA announces team advancement. A slot is created by the
// admin (e.g. "winner_group_a", "best_3rd_1") and its Team field is populated
// after the group stage concludes.
//
// Team is nil until the administrator calls ConfirmSlot; it becomes non-nil
// once the advancing team is confirmed.
type TournamentSlot struct {
	ID                int
	Label             string     // human-readable bracket position
	Team              *string    // nil until confirmed
	ConfirmedAt       *time.Time // nil until confirmed
	ConfirmedByUserID *int       // nil until confirmed
	CreatedAt         time.Time
	UpdatedAt         time.Time
}
