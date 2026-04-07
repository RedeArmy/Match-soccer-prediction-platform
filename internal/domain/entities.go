// Package domain contains the core business entities and rules of the
// World Cup quiniela system.
//
// This package must remain entirely free of infrastructure concerns: no
// database drivers, no HTTP types, no serialisation tags, no external
// library dependencies. The entities here represent concepts that the
// business cares about — Users, Matches, Predictions — not how they are
// stored in PostgreSQL or transported over HTTP.
//
// This boundary is what makes the business logic testable in isolation
// and portable across different storage or transport technologies. If you
// find yourself importing a third-party package here, stop and reconsider
// the design: that dependency almost certainly belongs in the infrastructure
// or service layer instead.
package domain

import "time"

// User represents a registered participant in the quiniela platform.
//
// PasswordHash stores the output of a one-way hashing function (bcrypt or
// argon2id). The plain-text password is discarded immediately after hashing
// at the service layer and must never appear in logs, error messages, or
// any persistence layer. Storing the hash directly on User is a pragmatic
// choice for this project's scale; revisit this if the authentication model
// grows to support multiple credential types or external identity providers.
type User struct {
	ID            int
	Name          string
	Email         string
	PasswordHash  string
	Role          UserRole
	ClerkSubject  string // opaque Clerk user ID, e.g. "user_2abc…"; empty for legacy rows
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// UserRole is a typed string that constrains the roles a User may hold.
//
// Using a named type rather than a bare string prevents accidental comparisons
// against untyped string literals and makes exhaustive switch statements
// possible when combined with a linter that enforces exhaustiveness checks.
// New roles must be added to this block explicitly; they cannot be introduced
// silently by passing an arbitrary string.
type UserRole string

// Allowed values for UserRole.
const (
	RoleAdmin  UserRole = "admin"
	RolePlayer UserRole = "player"
)

// Stadium represents an official FIFA World Cup 2026 venue.
//
// This is reference data: the 16 host stadiums are fixed for the tournament
// and change only in exceptional circumstances (host-city withdrawal). Capacity
// is stored for display purposes; it is not used in any business rule.
type Stadium struct {
	ID        int
	Name      string
	City      string
	Country   string
	Capacity  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Match represents a single World Cup fixture in the tournament schedule.
//
// HomeScore and AwayScore are pointers because a nil value is semantically
// distinct from zero: a score of 0–0 is a valid final result, whereas nil
// means the match has not yet been played or the result has not been
// confirmed. Using pointers makes this nullable semantics explicit at the
// type level, avoiding the need for a sentinel value (e.g. -1) that could
// be confused with a real score by accident.
//
// StadiumID is nullable: knockout-stage fixtures may be created before their
// venue is confirmed. Stadium is hydrated by the repository when loading a
// match with venue detail; it is nil when only the match metadata is needed.
type Match struct {
	ID        int
	HomeTeam  string
	AwayTeam  string
	HomeScore *int
	AwayScore *int
	Status    MatchStatus
	StadiumID *int
	Stadium   *Stadium
	KickoffAt time.Time
	CreatedAt time.Time
	UpdatedAt time.Time
}

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

// Prediction is a user's forecast for the exact score of a specific match.
//
// Points is a pointer because a nil value means scoring has not yet been
// calculated for this prediction (the match is still pending or live),
// whilst a value of 0 means the prediction was scored and earned no points.
// The ranking service must treat these two cases differently: unscored
// predictions are excluded from interim rankings, whilst zero-point
// predictions are included. Collapsing both cases into the integer 0 would
// introduce a subtle ranking bug that only manifests after kick-off.
type Prediction struct {
	ID        int
	UserID    int
	MatchID   int
	HomeScore int
	AwayScore int
	Points    *int
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Quiniela is a named collection of predictions submitted by one user.
//
// Predictions is embedded here as a denormalised slice for in-memory
// operations (scoring, ranking calculations). In the persistence layer,
// predictions are stored in a separate table referenced by quiniela_id;
// the repository is responsible for hydrating this field only when the
// caller explicitly requests it, rather than eager-loading on every read.
// Callers that do not need predictions should use repository methods that
// omit the hydration step to avoid unnecessary database round-trips.
type Quiniela struct {
	ID          int
	Name        string
	OwnerID     int
	Predictions []Prediction
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Tiebreaker is an auxiliary forecast used to break ranking ties between
// players who have earned the same number of points from match predictions.
//
// The tiebreaker question is a numeric estimate defined per Quiniela by its
// administrator — for example, total goals scored in the tournament, or the
// exact number of goals in the final. The player whose Prediction is closest
// to the actual Result without exceeding it ranks higher. Result is nil until
// the tournament outcome is known and the administrator confirms the value.
type Tiebreaker struct {
	ID         int
	UserID     int
	QuinielaID int
	Prediction int  // the player's numeric estimate
	Result     *int // nil until the tiebreaker outcome is officially confirmed
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
