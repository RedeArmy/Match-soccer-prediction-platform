// Package domain contains the core business entities and rules of the
// World Cup quiniela system.
//
// This package must remain entirely free of infrastructure concerns: no
// database drivers, no HTTP types, no serialisation tags, no external
// library dependencies. The entities here represent concepts that the
// business cares about - Users, Matches, Predictions - not how they are
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
//
// BannedAt/BannedBy/BanReason track administrative bans. A non-nil BannedAt
// means the user is currently banned and must be blocked from all write
// operations. BannedBy is the ID of the admin who issued the ban; BanReason
// is a human-readable explanation stored for audit purposes.
type User struct {
	ID            int
	Name          string
	Email         string
	Role          UserRole
	ClerkSubject  string // opaque Clerk user ID, e.g. "user_2abc…"; empty for legacy rows
	BannedAt      *time.Time
	BannedBy      *int
	BanReason     string
	BalanceCents  int // spendable funds in minor currency units; never negative
	ReservedCents int // funds locked for pending withdrawal requests
	CreatedAt     time.Time
	UpdatedAt     time.Time
	DeletedAt     *time.Time // nil for active users; set when the record is soft-deleted
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
	RoleAdmin UserRole = "admin"
	RoleUser  UserRole = "user"
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
// hierarchy (city -> state -> country) hydrated by the repository.
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
	ID                 int
	UserID             int
	MatchID            int
	HomeScore          int
	AwayScore          int
	PredictedWinMethod *WinMethod // optional; nil means no win-method prediction was submitted
	Points             *int
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// QuinielaStatus is the system-managed lifecycle state of a Quiniela.
//
// The transition rules are enforced exclusively by the membership service:
//   - QuinielaStatusActive   - group has ≥ MinMembersForActive active members;
//     eligible for payment processing and prize distribution.
//   - QuinielaStatusInactive - group has < MinMembersForActive active members;
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
// member. Other users join via the InviteCode - a short, human-friendly token
// shared out-of-band (WhatsApp, SMS). Membership records are stored in the
// group_memberships table; this struct carries only the group metadata.
//
// EntryFee and Currency support payment-tracking workflows; they default to
// 0 / "GTQ" and are never nil in a hydrated struct.
//
// InviteCodeExpiresAt is always nil: invite links never expire by design.
// RotateInviteCode can be used to invalidate a leaked link by generating a
// new one; the old code becomes unreachable immediately after rotation.
//
// Status is system-managed: the membership service sets it to
// QuinielaStatusActive when MinMembersPerGroup active members are present,
// and reverts to QuinielaStatusInactive when the count falls below that
// threshold. Only active groups are eligible for payments and prizes.
//
// The number of prize winners is determined by WinnerCount(activePaidMembers).
// The platform enforces a hard cap of MaxMembersPerGroup (20) active members;
// the minimum for prize eligibility is MinMembersPerGroup (5).
type Quiniela struct {
	ID                  int
	Name                string
	OwnerID             int
	InviteCode          string
	InviteCodeExpiresAt *time.Time     // always nil; invite links never expire
	Status              QuinielaStatus // system-managed: active iff ≥ MinMembersPerGroup active members
	EntryFee            int
	Currency            string
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
//
// Phase and QuinielaID narrow the scope of the question:
//   - both nil  → global (the original platform-wide singleton at id=1)
//   - Phase only → platform-wide question for that tournament phase
//   - QuinielaID only → group-specific question for that quiniela
//   - both set  → question scoped to a specific group and phase
type TiebreakerConfig struct {
	ID         int
	Question   string
	Phase      *MatchPhase // nil = not phase-scoped
	QuinielaID *int        // nil = platform-wide
	Result     *int        // nil until the administrator confirms the outcome
	CreatedAt  time.Time
	UpdatedAt  time.Time
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
// PrizeWinner is computed by the ranking service using WinnerCount(n) where n
// is the number of active paid members. It is never stored in the database.
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

// MembershipRole distinguishes the group creator from regular members within a
// single Quiniela. It is a group-scoped role that is orthogonal to the system-
// wide UserRole: a CreateOwner is always a RoleUser at the system level and
// never inherits any system-admin permissions.
//
// The owner role is assigned once at group creation and transferred automatically
// when the owner leaves or is banned (oldest active member becomes the new owner).
// System administrators may also transfer ownership manually to resolve conflicts.
type MembershipRole string

// Allowed values for MembershipRole.
const (
	MembershipRoleMember      MembershipRole = "member" // regular participant
	MembershipRoleCreateOwner MembershipRole = "owner"  // group creator / current owner
)

// GroupMembership records one user's participation in one Quiniela.
//
// Role distinguishes the group owner (MembershipRoleCreateOwner) from regular members.
// The owner can rename the group; other actions on the group are reserved for
// system administrators (RoleAdmin).
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
//
// RemovedAt and RemovedBy record the audit trail for soft-deleted memberships
// (status = 'left'). RemovedBy is nil when the member left voluntarily; it
// holds the admin's user ID when an administrator forced the removal.
type GroupMembership struct {
	ID         int
	QuinielaID int
	UserID     int
	Role       MembershipRole
	Status     MembershipStatus
	Paid       bool
	JoinedAt   *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
	RemovedAt  *time.Time // nil unless status = 'left'
	RemovedBy  *int       // nil = voluntary exit; non-nil = admin-forced removal
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
	PointsByPhase map[MatchPhase]int // phase -> total scored points; empty when no scored predictions

	// Derived rates - rounded to two decimal places
	AccuracyPct      float64 // CorrectPredictions / ScoredPredictions * 100; 0.0 if ScoredPredictions == 0
	AvgPointsPerPred float64 // TotalPoints / ScoredPredictions; 0.0 if ScoredPredictions == 0

	// Streaks (computed from scored predictions ordered by match kickoff ASC)
	CurrentStreak int // consecutive correct predictions ending at the most recent scored match
	LongestStreak int // longest ever consecutive correct run

	// Temporal
	LastPredictionAt *time.Time // nil when the user has never submitted a prediction
}

// Tiebreaker is a single user's numeric estimate for the global tiebreaker
// question. A user may submit at most one prediction per TiebreakerConfig;
// TiebreakerConfigID identifies which config (global, phase-scoped, or
// group-scoped) this prediction answers.
type Tiebreaker struct {
	ID                 int
	UserID             int
	TiebreakerConfigID int // which config this prediction answers
	Prediction         int // the player's numeric estimate
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// SystemParamType constrains the Value interpretation for a SystemParam row.
// The infrastructure layer is responsible for parsing the raw text Value into
// the appropriate Go type before handing it to the service layer.
type SystemParamType string

// Allowed values for SystemParamType.
const (
	SystemParamTypeString   SystemParamType = "string"
	SystemParamTypeInt      SystemParamType = "int"
	SystemParamTypeBool     SystemParamType = "bool"
	SystemParamTypeDuration SystemParamType = "duration"
)

// SystemParam is a key-value configuration entry managed at runtime by
// administrators without requiring a deployment. IsRuntime = true means the
// service layer re-reads the value on each request (or on cache miss); false
// means the value is treated as boot-time configuration and a restart is
// needed to pick up changes.
//
// Category groups related params (e.g. "scoring", "payment", "leaderboard")
// to simplify admin UI rendering and bulk-fetch patterns.
type SystemParam struct {
	Key          string
	Value        string
	DefaultValue string
	Type         SystemParamType
	Category     string
	IsRuntime    bool
	Description  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// AuditLog is an immutable record of a significant administrative or system
// action. Rows are append-only - no UPDATE or DELETE is ever issued against
// this table. ActorID is nil when the action was triggered by the system
// itself (e.g. a scheduled job). ResourceType / ResourceID identify the
// entity that was affected; both are nil for system-level actions.
//
// Metadata holds action-specific context serialised as a free-form key-value
// map; the infrastructure layer marshals it to/from JSONB. No business logic
// should depend on the content of Metadata - it is for human review only.
type AuditLog struct {
	ID           int
	ActorID      *int
	ActorRole    *UserRole
	Action       string
	ResourceType *string
	ResourceID   *int
	Metadata     map[string]any
	CreatedAt    time.Time
}

// PaymentStatus tracks the lifecycle of a payment transaction.
type PaymentStatus string

// Allowed values for PaymentStatus.
const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusConfirmed PaymentStatus = "confirmed"
	PaymentStatusRefunded  PaymentStatus = "refunded"
	// PaymentStatusRejected indicates the payment was reviewed and denied
	// before any funds were captured. Distinct from refunded, which implies
	// money was received and subsequently returned.
	PaymentStatusRejected PaymentStatus = "rejected"
)

// PaymentRecord tracks a single entry-fee payment for one member of a
// Quiniela. Amount is stored in the minor unit of Currency (e.g. centavos
// for MXN) to avoid floating-point representation issues.
//
// Reference is the opaque identifier returned by the external payment
// provider; it is nil until the payment provider issues a transaction ID.
// ConfirmedAt is nil while the payment is pending or rejected.
// ReviewedBy and Notes are populated by the admin who called Validate or
// Reject; both are nil / empty until a review action is taken.
type PaymentRecord struct {
	ID          int
	QuinielaID  int
	UserID      int
	Amount      int // in minor units (e.g. centavos)
	Currency    string
	Status      PaymentStatus
	Reference   *string    // nil until the payment provider assigns a transaction ID
	ReviewedBy  *int       // nil until validated or rejected by an admin
	Notes       string     // empty until an admin adds review notes
	ConfirmedAt *time.Time // nil for pending / rejected payments
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// LeaderboardSnapshotEntry is one participant's frozen ranking within a
// LeaderboardSnapshot. It mirrors the runtime LeaderboardEntry but is
// serialised to JSONB in the database, so it carries only scalar fields.
type LeaderboardSnapshotEntry struct {
	UserID      int
	Rank        int
	TotalPoints int
	PrizeWinner bool
}

// LeaderboardSnapshot is a point-in-time copy of a Quiniela's leaderboard
// taken by the ranking service (e.g. at phase boundaries or payment cutoffs).
// Entries are stored as JSONB; the infrastructure layer handles marshaling.
//
// Snapshots are immutable after creation. TakenAt records the logical time
// the snapshot represents, which may differ slightly from CreatedAt when
// backfill jobs are used.
//
// SchemaVersion identifies the JSONB encoding of Entries (see SnapshotSchemaV1).
// The repository uses it to select the correct deserialiser when reading
// historical rows, decoupling struct evolution from backwards compatibility.
type LeaderboardSnapshot struct {
	ID                 int
	QuinielaID         int
	TakenAt            time.Time
	Entries            []LeaderboardSnapshotEntry
	SchemaVersion      int
	CreatedAt          time.Time
	TriggeredByMatchID *int // non-nil for worker-triggered snapshots; nil for admin/manual
}

// GlobalLeaderboardEntry is a read-only projection used by the admin global
// leaderboard endpoint. It aggregates total prediction points across all
// quinielas for a single user, ranked descending.
type GlobalLeaderboardEntry struct {
	UserID      int
	UserName    string
	TotalPoints int
	Rank        int
}

// ConflictType classifies the kind of operational conflict detected by the
// system for administrative review.
type ConflictType string

const (
	// ConflictGroupNoOwner indicates an active quiniela with no active
	// CreateOwner membership - ownership may have been lost after a ban.
	ConflictGroupNoOwner ConflictType = "group_without_owner"
	// ConflictPaymentStale indicates a payment record stuck in "pending"
	// beyond the configured staleness threshold.
	ConflictPaymentStale ConflictType = "payment_stale"
	// ConflictMembershipStale indicates a group join request stuck in
	// "pending" beyond the configured staleness threshold.
	ConflictMembershipStale ConflictType = "membership_stale"
)

// Conflict is a computed, non-persisted operational issue detected by the
// ConflictService that requires administrative attention.
type Conflict struct {
	Type       ConflictType
	EntityID   int
	EntityType string
	Details    map[string]any
	DetectedAt time.Time
	// AgeDays is the number of days the underlying entity has been in a
	// conflicting state. Nil for conflict types where age is not meaningful
	// (e.g. group_without_owner, where the ownership-loss timestamp is unknown).
	AgeDays *int
}

// DashboardStats is the aggregate view returned by GET /admin/stats, intended
// for the admin dashboard's key-metric widgets.
type DashboardStats struct {
	Groups   GroupDashboardStats
	Users    UserDashboardStats
	Payments PaymentDashboardStats
}

// GroupDashboardStats counts quinielas by lifecycle status.
type GroupDashboardStats struct {
	Total    int
	Active   int
	Inactive int
	Deleted  int
}

// UserDashboardStats counts registered users by lifecycle status.
type UserDashboardStats struct {
	Total  int
	Active int
	Banned int
}

// PaymentDashboardStats counts payment records by lifecycle status.
// TotalCollected is the sum of confirmed payment amounts in minor currency units.
type PaymentDashboardStats struct {
	Pending        int
	Confirmed      int
	Rejected       int
	TotalCollected int
}

// ── Balance ledger ────────────────────────────────────────────────────────────

// BalanceLedgerKind identifies the business operation that caused a balance
// mutation. Each row in balance_ledger carries exactly one kind so that the
// full history of an account can be audited and categorised without joining
// to multiple source tables.
type BalanceLedgerKind string

// Balance ledger kind constants enumerate every operation that can produce a
// balance_ledger row.
const (
	LedgerKindWebhookRecurrente BalanceLedgerKind = "webhook_recurrente"
	LedgerKindWebhookPayPal     BalanceLedgerKind = "webhook_paypal"
	LedgerKindBankTransfer      BalanceLedgerKind = "bank_transfer"
	LedgerKindEntryFee          BalanceLedgerKind = "entry_fee"
	LedgerKindPrize             BalanceLedgerKind = "prize"
	LedgerKindWithdrawalReserve BalanceLedgerKind = "withdrawal_reserve"
	LedgerKindWithdrawalRelease BalanceLedgerKind = "withdrawal_release"
	LedgerKindWithdrawalDeduct  BalanceLedgerKind = "withdrawal_deduct"
)

// BalanceLedger is a single, immutable row recording one atomic balance change.
// Rows are append-only; they are never updated or deleted.
type BalanceLedger struct {
	ID           int64
	UserID       int
	DeltaCents   int // positive = credit, negative = debit/reserve
	Kind         BalanceLedgerKind
	BalanceAfter int     // users.balance_cents after the mutation
	RefID        *int64  // primary key of the originating record
	RefType      *string // "payment_record" | "bank_transfer_proof" | "withdrawal_request"
	CreatedBy    *int    // nil = system / webhook
	CreatedAt    time.Time
}

// ── Bank transfer proofs ──────────────────────────────────────────────────────

// BankTransferStatus is the lifecycle state of a bank transfer proof.
type BankTransferStatus string

// Bank transfer proof lifecycle states.
const (
	BankTransferPending  BankTransferStatus = "pending"
	BankTransferApproved BankTransferStatus = "approved"
	BankTransferRejected BankTransferStatus = "rejected"
)

// BankTransferProof records a user-uploaded payment receipt for a Guatemalan
// bank transfer. An admin reviews the proof, verifies the declared amount, and
// either approves (crediting the user's balance) or rejects it.
//
// StorageKey is an opaque reference to the file inside the configured FileStore;
// raw file bytes are never stored in the database.
type BankTransferProof struct {
	ID          int64
	UserID      int
	AmountCents int
	Currency    string
	StorageKey  string
	ContentType string
	FileSize    int
	Status      BankTransferStatus
	ReviewedBy  *int
	Notes       string
	ApprovedAt  *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ── Withdrawal requests ───────────────────────────────────────────────────────

// WithdrawalMethod specifies the channel through which funds are paid out.
type WithdrawalMethod string

// Supported payout channels for withdrawal requests.
const (
	WithdrawalMethodBankGT WithdrawalMethod = "bank_gt" // Guatemalan bank account
	WithdrawalMethodPayPal WithdrawalMethod = "paypal"  // international PayPal
)

// WithdrawalStatus is the lifecycle state of a withdrawal request.
type WithdrawalStatus string

// Withdrawal request lifecycle states.
const (
	WithdrawalPending   WithdrawalStatus = "pending"
	WithdrawalApproved  WithdrawalStatus = "approved"
	WithdrawalRejected  WithdrawalStatus = "rejected"
	WithdrawalProcessed WithdrawalStatus = "processed"
)

// WithdrawalRequest is a user-initiated payout request.
//
// On creation: AmountCents is moved from balance_cents to reserved_cents.
// On approval: reserved_cents is committed (balance_cents permanently reduced).
// On rejection: reserved_cents is released back to available balance.
//
// PayoutDetails holds method-specific fields:
//   - bank_gt : {"account_number":"…","bank_name":"…"}
//   - paypal  : {"paypal_email":"…"}
type WithdrawalRequest struct {
	ID            int
	UserID        int
	AmountCents   int
	Currency      string
	Method        WithdrawalMethod
	PayoutDetails map[string]string
	Status        WithdrawalStatus
	ReviewedBy    *int
	Notes         string
	ProcessedAt   *time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// ── Admin notification log ────────────────────────────────────────────────────

// AdminNotifStatus is the delivery outcome of an admin email dispatch.
type AdminNotifStatus string

// Delivery outcomes recorded in admin_notification_log.
const (
	AdminNotifStatusSent   AdminNotifStatus = "sent"
	AdminNotifStatusFailed AdminNotifStatus = "failed"
)

// AdminNotificationLog is an immutable record of a single admin email dispatch.
// The table is append-only; rows are never updated or deleted.
type AdminNotificationLog struct {
	ID          int64
	EventType   string
	Recipients  []string
	Subject     string
	Status      AdminNotifStatus
	ResendMsgID string    // populated on successful delivery
	ErrorDetail string    // populated on failure
	CreatedAt   time.Time // set by the database on INSERT
}

// ── Notification DLQ ─────────────────────────────────────────────────────────

// NotificationDLQEntry records a delivery failure for a specific channel
// after all retry attempts have been exhausted.  Used for manual replay and
// ops alerting.
type NotificationDLQEntry struct {
	ID          int64
	OutboxID    *int64 // references domain_outbox.id; nil when the outbox row was purged
	Channel     string // 'email' | 'push' | 'sse'
	UserID      *int   // nil for admin/system events that have no target user
	EventType   string
	Payload     []byte // raw JSON payload from the outbox entry
	ErrorDetail string
	CreatedAt   time.Time
}

// ── User notifications ────────────────────────────────────────────────────────

// UserNotification is a persisted inbox entry for a single user.
// The idempotency_key prevents duplicate rows when the outbox worker retries.
type UserNotification struct {
	ID             int64
	UserID         int
	EventType      string
	Title          string
	Body           string
	ActionURL      string
	Metadata       map[string]any
	IdempotencyKey string
	ReadAt         *time.Time
	CreatedAt      time.Time
}

// IsRead reports whether the notification has been acknowledged by the user.
func (n *UserNotification) IsRead() bool { return n.ReadAt != nil }

// ── Notification preferences ──────────────────────────────────────────────────

// NotificationPreference controls per-user, per-event-type channel opt-in.
// Missing rows default to all channels enabled (opt-out model).
type NotificationPreference struct {
	UserID       int
	EventType    string
	ChannelEmail bool
	ChannelPush  bool
	ChannelInApp bool
	UpdatedAt    time.Time
}

// ── Push subscriptions ────────────────────────────────────────────────────────

// PushSubscription is a Web Push (VAPID) subscription registered by a browser
// or device.  A user may have multiple active subscriptions.
type PushSubscription struct {
	ID         int64
	UserID     int
	Endpoint   string
	P256dhKey  string
	AuthKey    string
	UserAgent  string
	Active     bool
	CreatedAt  time.Time
	LastUsedAt *time.Time
}
