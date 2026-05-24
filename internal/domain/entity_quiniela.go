package domain

import "time"

// ── Quiniela ──────────────────────────────────────────────────────────────────

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

// ── Group membership ──────────────────────────────────────────────────────────

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

// ── Prediction & leaderboard ──────────────────────────────────────────────────

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

// PredictionScoreLog is an immutable audit record written each time ScoreMatch
// recalculates points for a prediction. It captures the full scoring context —
// match result, prediction submitted, and active scoring config — so that any
// points dispute can be reconstructed from this table alone.
//
// OldPoints is nil on the first scoring run; subsequent re-scores set it to
// the value that was overwritten. The delta (new_points - COALESCE(old_points,0))
// is computed by the database as a generated column.
type PredictionScoreLog struct {
	PredictionID      int
	MatchID           int
	UserID            int
	OldPoints         *int // nil on first scoring
	NewPoints         int
	MatchHomeScore    int
	MatchAwayScore    int
	MatchWinMethod    *WinMethod // nil for group-stage fixtures
	MatchPhase        MatchPhase
	PredHomeScore     int
	PredAwayScore     int
	PredWinMethod     *WinMethod // nil when user did not guess a win method
	CfgExactScore     int
	CfgCorrectOutcome int
	CfgGoalDiff       int
	CfgExtraTimeBonus int
	CfgPenaltiesBonus int
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

// GlobalLeaderboardEntry is a read-only projection used by the admin global
// leaderboard endpoint. It aggregates total prediction points across all
// quinielas for a single user, ranked descending.
type GlobalLeaderboardEntry struct {
	UserID      int
	UserName    string
	TotalPoints int
	Rank        int
}

// ── User prediction statistics ────────────────────────────────────────────────

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

// ── Tiebreaker ────────────────────────────────────────────────────────────────

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

// ── Leaderboard snapshot ──────────────────────────────────────────────────────

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
