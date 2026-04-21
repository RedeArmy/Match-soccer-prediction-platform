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
// Authentication is delegated entirely to Clerk: users log in via Clerk's
// hosted flow and the API validates the resulting JWT. No password or
// credential is stored here. ClerkSubject is the opaque identifier Clerk
// assigns to each user (format "user_2abc…") and is the stable link between
// a Clerk identity and the internal User record.
type User struct {
	ID           int
	Name         string
	Email        string
	Role         UserRole
	ClerkSubject string // opaque Clerk user ID, e.g. "user_2abc…"; empty for legacy rows
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time // nil for active users; set when the record is soft-deleted
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

// Country represents one of the three FIFA World Cup 2026 host nations.
// Code is the ISO 3166-1 alpha-2 identifier (e.g. "US", "MX", "CA").
type Country struct {
	ID   int
	Name string
	Code string
}

// State represents a US state, Mexican state, or Canadian province that hosts
// at least one FIFA World Cup 2026 venue. Code follows the standard postal
// abbreviation for the country (e.g. "NJ", "CDMX", "BC").
type State struct {
	ID        int
	Name      string
	Code      string
	CountryID int
	Country   *Country // hydrated by the repository when reading location data
}

// City is a host city for at least one FIFA World Cup 2026 venue.
type City struct {
	ID      int
	Name    string
	StateID int
	State   *State // hydrated by the repository when reading location data
}

// Stadium represents an official FIFA World Cup 2026 venue.
//
// This is reference data: the 16 host stadiums are fixed for the tournament
// and change only in exceptional circumstances (host-city withdrawal). Capacity
// is stored for display purposes; it is not used in any business rule.
//
// CityID is the foreign key to the cities table. City is the full location
// hierarchy (city → state → country) hydrated by the repository.
type Stadium struct {
	ID        int
	Name      string
	CityID    int
	City      *City // hydrated by the repository when reading location data
	Capacity  int
	CreatedAt time.Time
	UpdatedAt time.Time
}

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
//
// GroupLabel is nil for knockout matches and holds the FIFA group letter
// ("A"–"L") for group-stage fixtures. It drives real-time standings
// calculation without a separate teams table.
type Match struct {
	ID         int
	HomeTeam   string
	AwayTeam   string
	HomeScore  *int
	AwayScore  *int
	Status     MatchStatus
	Phase      MatchPhase
	GroupLabel *string // nil for knockout; "A"–"L" for group stage
	StadiumID  *int
	Stadium    *Stadium
	KickoffAt  time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

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

// QuinielaStatus is the system-managed lifecycle state of a Quiniela.
//
// The transition rules are enforced exclusively by the membership service:
//   - QuinielaStatusActive   — group has ≥ MinMembersForActive active members;
//     eligible for payment processing and prize distribution.
//   - QuinielaStatusInactive — group has < MinMembersForActive active members;
//     predictions can still be submitted but payments are blocked.
//
// No HTTP endpoint exposes a direct status change. The status cannot be set
// to any value other than these two; the database enforces this with a CHECK
// constraint.
type QuinielaStatus string

// Allowed values for QuinielaStatus.
const (
	QuinielaStatusActive   QuinielaStatus = "active"
	QuinielaStatusInactive QuinielaStatus = "inactive"
)

// Quiniela represents a named prediction group in the tournament.
//
// Each Quiniela is created by an owner (OwnerID) who becomes its first active
// member. Other users join via the InviteCode — a short, human-friendly token
// shared out-of-band (WhatsApp, SMS). Membership records are stored in the
// group_memberships table; this struct carries only the group metadata.
//
// EntryFee and Currency support future payment-tracking workflows; they
// default to 0 / "MXN" and are never nil in a hydrated struct.
// MaxMembers is nil when the group has no size cap.
//
// InviteCodeExpiresAt is always nil: invite links never expire by design.
// RotateInviteCode can be used to invalidate a leaked link by generating a
// new one; the old code becomes unreachable immediately after rotation.
//
// Status is system-managed: the membership service sets it to
// QuinielaStatusActive when MinMembersForActive active members are present,
// and reverts to QuinielaStatusInactive when the count falls below that
// threshold. Only active groups are eligible for payments and prizes.
//
// PrizeThreshold drives proportional prize distribution:
//
//	winnerCount = max(1, floor(memberCount / PrizeThreshold))
//
// A threshold of 3 means roughly 1-in-3 active+paid members receive a prize.
// The service layer defaults this to DefaultPrizeThreshold when the caller
// omits it. Must be positive; enforced by a CHECK constraint in the database.
type Quiniela struct {
	ID                  int
	Name                string
	OwnerID             int
	InviteCode          string
	InviteCodeExpiresAt *time.Time     // always nil; invite links never expire
	Status              QuinielaStatus // system-managed: active iff ≥ MinMembersForActive active members
	EntryFee            int
	Currency            string
	MaxMembers          *int
	PrizeThreshold      int // ≥ 1; winnerCount = max(1, floor(N/PrizeThreshold))
	CreatedAt           time.Time
	UpdatedAt           time.Time
	DeletedAt           *time.Time // nil for active groups; set when the record is soft-deleted
}

// TiebreakerConfig holds the single global tiebreaker question and confirmed
// result managed by the system administrator. There is exactly one row in the
// database; the application upserts it on id=1.
//
// Question must be non-empty before members may submit predictions.
// Result is nil until the administrator confirms the actual outcome.
type TiebreakerConfig struct {
	ID        int
	Question  string
	Result    *int // nil until the administrator confirms the outcome
	CreatedAt time.Time
	UpdatedAt time.Time
}

// TiebreakerView is a read-only projection returned by TiebreakerService.GetMine.
// It combines the current global question with the caller's own prediction.
//
// Question is nil when no global question has been configured yet.
// Entry is nil when the caller has not yet submitted a prediction.
type TiebreakerView struct {
	Question *string     // from TiebreakerConfig; nil if not yet configured
	Entry    *Tiebreaker // nil when caller has not submitted
}

// LeaderboardEntry pairs a quiniela participant with their aggregated score.
// It is a read-only projection used exclusively by the ranking service and the
// leaderboard API response; it is never persisted to the database.
//
// Rank is 1-based and assigned after sorting descending by TotalPoints.
// Two entries with equal TotalPoints receive the same rank (standard
// competition ranking), and the next rank is skipped accordingly.
//
// PrizeWinner is computed by the ranking service using the Quiniela's
// PrizeThreshold: winnerCount = max(1, floor(memberCount / PrizeThreshold)).
// It is never stored in the database.
type LeaderboardEntry struct {
	User        *User
	TotalPoints int
	Rank        int
	PrizeWinner bool // true when this entry is in prize position; computed, never persisted
}

// MembershipStatus tracks the lifecycle of a user's membership in a Quiniela.
type MembershipStatus string

// Allowed values for MembershipStatus.
const (
	MembershipPending MembershipStatus = "pending"
	MembershipActive  MembershipStatus = "active"
	MembershipLeft    MembershipStatus = "left"
)

// GroupMembership records one user's participation in one Quiniela.
//
// JoinedAt is nil for pending memberships and is populated when the user
// transitions to active status. The owner of a Quiniela always receives an
// active membership at creation time so they appear in leaderboards.
//
// Paid tracks whether the entry fee has been settled. It is set to true
// automatically when the group is free (entry_fee = 0) or when the payment
// system confirms a successful transaction. Members with Paid = false may
// submit predictions, but their scores are excluded from all rankings until
// payment is confirmed.
type GroupMembership struct {
	ID         int
	QuinielaID int
	UserID     int
	Status     MembershipStatus
	Paid       bool
	JoinedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// UserPredictionStats holds aggregated prediction metrics for a single member
// within a quiniela. It is computed by the ranking service to resolve ties when
// two or more members have identical total points, using the three-rule chain:
//
//  1. Most correct predictions (CorrectCount DESC).
//  2. Fewest predictions submitted (TotalCount ASC).
//  3. Most exact-score hits (ExactCount DESC).
//
// Zero values represent a member who has made no scored predictions yet; they
// rank below any member with at least one scored prediction on rule 1, and
// above or equal to others on rules 2 and 3 only when all members share the
// same zero counts.
type UserPredictionStats struct {
	CorrectCount int // scored predictions where points > 0
	TotalCount   int // total scored predictions (points IS NOT NULL)
	ExactCount   int // predictions awarded exact-score points (PointsExactScore = 5)
}

// UserPredictionCounts is a repository projection of aggregated prediction
// metrics for a single user. It is an intermediate value consumed by
// UserStatsService to build a UserStats response and is never persisted.
type UserPredictionCounts struct {
	TotalPredictions   int
	ScoredPredictions  int        // points IS NOT NULL
	CorrectPredictions int        // points > 0
	ExactPredictions   int        // points = PointsExactScore
	TotalPoints        int        // sum of all scored points
	LastPredictionAt   *time.Time // nil when the user has never submitted a prediction
}

// UserStats is the complete performance profile for a quiniela participant.
// It is computed on demand by UserStatsService from multiple repository
// projections and is never stored in the database.
//
// Rates (AccuracyPct, AvgPointsPerPred) are both 0.0 when ScoredPredictions
// is zero to avoid division-by-zero at the service layer. Streak values are
// derived from scored predictions ordered by match kickoff time.
type UserStats struct {
	// Volume
	TotalPredictions   int
	ScoredPredictions  int
	CorrectPredictions int
	ExactPredictions   int

	// Points
	TotalPoints   int
	PointsByPhase map[MatchPhase]int // phase → total scored points; empty when no scored predictions

	// Derived rates — rounded to two decimal places
	AccuracyPct      float64 // CorrectPredictions / ScoredPredictions * 100; 0.0 if ScoredPredictions == 0
	AvgPointsPerPred float64 // TotalPoints / ScoredPredictions; 0.0 if ScoredPredictions == 0

	// Streaks (computed from scored predictions ordered by match kickoff ASC)
	CurrentStreak int // consecutive correct predictions ending at the most recent scored match
	LongestStreak int // longest ever consecutive correct run

	// Temporal
	LastPredictionAt *time.Time // nil when the user has never submitted a prediction
}

// Tiebreaker is a single user's numeric estimate for the global tiebreaker
// question. Predictions are global — one per user across all groups — because
// the question is set once by the system administrator and applies uniformly
// to every group's leaderboard. The confirmed result lives in TiebreakerConfig,
// not here.
type Tiebreaker struct {
	ID         int
	UserID     int
	Prediction int // the player's numeric estimate
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
