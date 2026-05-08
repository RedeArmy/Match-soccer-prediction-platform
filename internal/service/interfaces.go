// Package service contains the application's business logic.
//
// Each service orchestrates one domain concern: it reads from and writes to
// repositories, enforces business rules, and emits domain events. Services
// must not be aware of HTTP or database implementation details - they operate
// exclusively on domain entities and depend on repository interfaces defined
// in internal/repository, not on concrete PostgreSQL implementations.
//
// Service interfaces are defined in this file and are the contracts consumed
// by the handler layer. Concrete implementations live in the other files of
// this package and are wired at the composition root (cmd/api/main.go).
// This separation allows handlers to be tested with lightweight mock services
// without touching a real database.
package service

import (
	"context"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// MatchService defines operations on the Match entity.
//
// UpdateResult enforces the transition rules: a result may only be set when
// the match is in the Live or Finished status. After confirming the result
// the implementation must emit a MatchFinished domain event so that downstream
// consumers (MatchScorer, Notifier) react without being called
// directly.
type MatchService interface {
	CreateMatch(ctx context.Context, match *domain.Match) error
	GetMatch(ctx context.Context, id int) (*domain.Match, error)
	ListMatches(ctx context.Context) ([]*domain.Match, error)
	ListMatchesByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error)
	ListMatchesByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error)
	UpdateResult(ctx context.Context, id int, homeScore, awayScore int) (*domain.Match, error)
	StartMatch(ctx context.Context, id int) (*domain.Match, error)
}

// PredictionService defines operations on the Prediction entity.
//
// Submit enforces the prediction deadline and relies on the repository's
// atomic INSERT + unique index handling to reject duplicate predictions
// without a pre-read race window. Update follows the same deadline rules,
// requires the caller to own the prediction being modified, and uses
// optimistic concurrency to prevent silent last-write-wins overwrites.
type PredictionService interface {
	Submit(ctx context.Context, prediction *domain.Prediction) error
	Update(ctx context.Context, callerUserID, id int, homeScore, awayScore int) (*domain.Prediction, error)
	GetByUser(ctx context.Context, userID int) ([]*domain.Prediction, error)
	// GetByUserAndQuiniela returns all predictions for userID scoped to the
	// given quiniela. It delegates the membership gate to the repository layer
	// so the check and the data fetch are a single round-trip. A user who is
	// not an active member of quinielaID receives an empty slice.
	GetByUserAndQuiniela(ctx context.Context, userID, quinielaID int) ([]*domain.Prediction, error)
	GetByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error)
}

// MatchScorer calculates and persists points for all predictions on a
// finished match.
//
// ScoreMatch is intended to be called from a MatchFinished event handler, not
// directly from an HTTP handler, which is why it does not return a full list
// of updated predictions - the caller's context is asynchronous.
type MatchScorer interface {
	ScoreMatch(ctx context.Context, matchID int) error
}

// LeaderboardResult is the value returned by Ranker methods. It bundles the
// ranked entries with group-level metadata that the handler layer needs to
// compute prize eligibility and winner count without an additional round-trip.
//
// ActivePaidMembers is the count of members with status=active AND paid=true
// in the quiniela at query time. This is the authoritative input to
// domain.WinnerCount and domain.EligibleForPayments. It may differ from
// len(Entries) when some paid members have not yet submitted any predictions
// (they appear in the member roster but not in the leaderboard entries).
//
// WinnerCount is pre-computed from domain.WinnerCount(ActivePaidMembers) so
// callers do not need to import the domain package to derive it.
//
// EligibleForPrizes is pre-computed from domain.EligibleForPayments(ActivePaidMembers).
// When false, no prizes should be displayed or distributed regardless of the
// entry_fee setting.
type LeaderboardResult struct {
	Entries           []*domain.LeaderboardEntry
	ActivePaidMembers int
	WinnerCount       int
	EligibleForPrizes bool
}

// Ranker computes leaderboard standings for a given Quiniela.
//
// GetLeaderboard returns the overall standings sorted descending by TotalPoints.
// GetPhaseLeaderboard returns standings restricted to a single tournament phase.
// Only active, paid members are included in both variants. Unscored predictions
// (nil points) are excluded from the aggregation. PrizeWinner is set to true on
// entries that rank within the prize positions determined by domain.WinnerCount.
//
// Both methods return a LeaderboardResult that includes group-level prize
// metadata (ActivePaidMembers, WinnerCount, EligibleForPrizes) alongside the
// ranked entries, so the handler layer never needs a second DB round-trip to
// determine whether prizes apply.
type Ranker interface {
	GetLeaderboard(ctx context.Context, quinielaID int) (*LeaderboardResult, error)
	GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) (*LeaderboardResult, error)
}

// QuinielaService defines operations on the Quiniela entity.
//
// Create generates a unique invite code and records the owner as the first
// active member (MembershipRoleCreateOwner). GetByInviteCode enables the join flow:
// the caller obtains the quiniela from a short code before creating the membership.
//
// The invite code is permanent - it is generated once at creation and never
// rotated. Groups are identified by a stable code for the tournament's duration.
type QuinielaService interface {
	Create(ctx context.Context, quiniela *domain.Quiniela) error
	GetByID(ctx context.Context, id int) (*domain.Quiniela, error)
	GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error)
	GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error)
	// RenameGroup changes the name of the given group. Only the CreateOwner
	// (MembershipRoleCreateOwner) of the group may call this; any other caller receives
	// Forbidden. Returns the updated Quiniela on success.
	RenameGroup(ctx context.Context, quinielaID, callerUserID int, name string) (*domain.Quiniela, error)
}

// GroupMembershipService manages user membership in Quinielas.
//
// Join resolves the invite code and creates a pending join request inside a
// single repository transaction - the user is NOT active until any existing
// active member calls ApproveJoin. ListByQuiniela returns the full roster.
// ListByUser returns all groups a user belongs to, regardless of status.
//
// ApproveJoin promotes a pending request to active. Any active member of the
// quiniela may approve - there is no admin-only gate. After approval the group
// status is synchronised: if active member count reaches MinMembersForActive
// the quiniela transitions from inactive to active.
//
// Leave lets a user remove themselves from a quiniela. Only the user themselves
// may call this; no admin or owner can remove another member. After leaving,
// the group status is re-evaluated and may become inactive.
//
// MarkPaid is called exclusively by the payment system after a transaction is
// confirmed. It must never be exposed as a direct API action - callers cannot
// mark themselves as paid. For free groups (entry_fee = 0), paid is set to
// true automatically at join time and this method is never invoked.
type GroupMembershipService interface {
	Join(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error)
	ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error)
	Leave(ctx context.Context, quinielaID, callerUserID int) error
	MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error)
}

// MyStatsGetter computes the performance profile for the authenticated user.
//
// GetMyStats aggregates prediction counts, points by tournament phase, and
// streak information for the given userID. It is called exclusively from the
// authenticated GET /api/v1/users/me/stats endpoint and operates on global
// predictions - not scoped to any single quiniela.
type MyStatsGetter interface {
	GetMyStats(ctx context.Context, userID int) (*domain.UserStats, error)
}

// TiebreakerService manages the global numeric tiebreaker that resolves
// ranking ties across all groups when all statistical rules (correct count,
// total count, exact count) still leave two or more members at the same rank.
//
// The lifecycle is:
//  1. System administrator calls SetQuestion to define the global tiebreaker
//     prompt (e.g. "total goals in the Final"). Until set, no member may
//     submit a prediction.
//  2. Members call Submit (or re-Submit to update) with their numeric estimate.
//     Predictions are global - one per user, applied to every group they
//     belong to.
//  3. After the tournament, the administrator calls ConfirmResult with the
//     actual value. After confirmation, Submit returns Conflict.
//
// The admin gate for SetQuestion and ConfirmResult is enforced at the HTTP
// layer via RequireRole middleware, not inside this service.
type TiebreakerService interface {
	// SetQuestion stores or replaces the global tiebreaker prompt.
	// Returns Validation when question is empty.
	SetQuestion(ctx context.Context, question string) (*domain.TiebreakerConfig, error)

	// Submit upserts the caller's global numeric prediction.
	// quinielaID is used only to verify the caller is an active member of that group.
	// Returns Conflict when the result has already been confirmed.
	// Returns Validation when no question has been configured yet.
	// Returns Forbidden when the caller is not an active member of quinielaID.
	Submit(ctx context.Context, quinielaID, callerID, prediction int) (*domain.Tiebreaker, error)

	// GetMine returns the global tiebreaker question and the caller's own
	// numeric prediction. Entry is nil when the caller has not submitted yet.
	// quinielaID is used only to verify active membership.
	// Returns Forbidden when the caller is not an active member of quinielaID.
	GetMine(ctx context.Context, quinielaID, callerID int) (*domain.TiebreakerView, error)

	// ConfirmResult records the official numeric result globally, activating
	// tiebreaker ranking for all groups. After confirmation, Submit returns
	// Conflict. Returns Validation when no question has been configured yet.
	ConfirmResult(ctx context.Context, result int) error
}

// TournamentService manages real-time group standings and bracket slot
// administration for the FIFA World Cup.
//
// GetAllStandings and GetGroupStanding compute standings in real time from
// finished group-stage matches; no separate persistence is needed.
//
// CreateSlot and ConfirmSlot are admin-only operations gated by RequireRole
// middleware. Slots represent named bracket positions (e.g. "winner_group_a")
// that the admin fills in once FIFA announces team advancement.
type TournamentService interface {
	// GetAllStandings returns standings for every group keyed by group label.
	GetAllStandings(ctx context.Context) (map[string][]*domain.GroupStanding, error)
	// GetGroupStanding returns standings for a single group.
	// Returns Validation when group is empty; NotFound when the group has no matches.
	GetGroupStanding(ctx context.Context, group string) ([]*domain.GroupStanding, error)
	// CreateSlot creates a new named bracket position. label must be unique.
	// Returns Validation when label is empty.
	CreateSlot(ctx context.Context, label string) (*domain.TournamentSlot, error)
	// ConfirmSlot sets the advancing team for the given slot.
	// Returns Validation when team is empty; NotFound when the slot does not exist.
	ConfirmSlot(ctx context.Context, slotID, adminID int, team string) (*domain.TournamentSlot, error)
	// ListSlots returns all bracket position slots.
	ListSlots(ctx context.Context) ([]*domain.TournamentSlot, error)
}

// Notifier dispatches notifications in response to domain events.
//
// Notify is a fire-and-forget operation: failures are logged but not returned
// to the caller because notification delivery is best-effort and must not
// block or fail the primary operation that triggered the event.
type Notifier interface {
	Notify(ctx context.Context, userID int, message string) error
}

// PostScoringInvalidator is the contract for any cache layer that holds data
// derived from prediction scores. The scoring worker calls InvalidateAfterScoring
// once per MatchFinished event, after ScoreMatch succeeds, so subsequent reads
// within the same Redis cluster see fresh point totals without waiting for
// natural TTL expiry.
//
// Implementations must be non-fatal: cache errors must be logged and swallowed,
// never returned, because scoring has already committed and a brief period of
// stale cache is preferable to blocking the event pipeline.
type PostScoringInvalidator interface {
	InvalidateAfterScoring(ctx context.Context, quinielaIDs []int)
}

// MutationHookRegisterer is an optional extension of SystemParamService that
// allows callers to register post-mutation callbacks. Each hook is called
// synchronously after a successful Set or BulkSet for the matching key, and
// after the in-process cache has been evicted. Wiring code uses a type
// assertion to check for support; the hook is silently skipped on
// implementations that do not implement this interface.
type MutationHookRegisterer interface {
	RegisterMutationHook(key string, fn func(ctx context.Context))
}

// SystemParamService provides typed, cached access to runtime-configurable
// key-value settings stored in the system_params table.
//
// All Get* helpers return a typed value and fall back to their defaultVal
// argument when the key is absent or the stored string cannot be parsed.
// This means callers never receive an error from a missing param - the domain
// constant is always the fallback, so the system degrades gracefully.
//
// Set invalidates the in-memory cache entry for the affected key immediately,
// guaranteeing that the next read within the same process sees the new value.
type SystemParamService interface {
	Get(ctx context.Context, key string) (*domain.SystemParam, error)
	GetAll(ctx context.Context) ([]*domain.SystemParam, error)
	GetByCategory(ctx context.Context, cat string) ([]*domain.SystemParam, error)
	Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error)
	// GetString returns the raw string value, falling back to defaultVal.
	GetString(ctx context.Context, key, defaultVal string) string
	// GetInt parses the value as a base-10 integer, falling back to defaultVal.
	GetInt(ctx context.Context, key string, defaultVal int) int
	// GetDuration parses the value as a time.Duration string (e.g. "5m"),
	// falling back to defaultVal.
	GetDuration(ctx context.Context, key string, defaultVal time.Duration) time.Duration
	// GetBool parses the value as a boolean, falling back to defaultVal.
	GetBool(ctx context.Context, key string, defaultVal bool) bool
	// BulkSet updates multiple parameters in a single repository call.
	// Each key-value pair is upserted atomically. actorID is recorded as
	// the editor for the audit trail.
	BulkSet(ctx context.Context, params map[string]string, actorID int) error
}

// AuditLogger records significant administrative and system actions to an
// immutable audit trail.
//
// Log is fire-and-forget: it never returns an error. Failures are logged at
// WARN level and silently discarded so that a transient database issue cannot
// roll back or fail an already-committed business operation.
//
// actorID is nil for system-generated events (e.g. scheduled jobs).
// resourceType and resourceID identify the affected entity; both are nil for
// system-level actions. metadata carries any extra context that does not fit
// into the structured columns.
type AuditLogger interface {
	Log(
		ctx context.Context,
		actorID *int,
		actorRole *domain.UserRole,
		action string,
		resourceType *string,
		resourceID *int,
		metadata map[string]any,
	)
}

// PaymentService manages entry-fee payment records for quiniela groups.
//
// CreateRecord creates a pending record that must later be confirmed by an
// admin via ValidateDeposit. RejectDeposit marks a pending payment as denied
// without capturing funds. Only an admin may call Validate or Reject; the
// caller's identity is enforced at the HTTP layer via RequireRole.
type PaymentService interface {
	CreateRecord(ctx context.Context, quinielaID, userID, amount int, currency, reference string) (*domain.PaymentRecord, error)
	ValidateDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error)
	RejectDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error)
	ListPending(ctx context.Context) ([]*domain.PaymentRecord, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.PaymentRecord, error)
	// List returns all payment records matching the given filters with pagination.
	List(ctx context.Context, f repository.PaymentFilters, p repository.Pagination) ([]*domain.PaymentRecord, error)
}

// AdminGroupService exposes administrative operations on Quiniela groups that
// are not available to regular members.
//
// All methods require an adminID that is stored in the audit trail. The admin
// role gate is enforced at the HTTP layer via RequireRole - this service does
// not re-check it internally.
type AdminGroupService interface {
	// DeleteGroup soft-deletes the quiniela. Returns NotFound when it does not
	// exist or is already deleted.
	DeleteGroup(ctx context.Context, quinielaID, adminID int) error
	// RemoveMember sets the membership status to 'left'. Returns NotFound for
	// inactive or non-existent memberships.
	RemoveMember(ctx context.Context, membershipID, adminID int) error
	// UpdateGroupSettings changes the entry_fee for a group. Returns the updated Quiniela.
	UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee, adminID int) (*domain.Quiniela, error)
	// TransferOwnership assigns MembershipRoleCreateOwner to newOwnerUserID and
	// demotes the current owner to MembershipRoleMember. Returns NotFound when
	// quinielaID does not exist or newOwnerUserID is not an active member.
	TransferOwnership(ctx context.Context, quinielaID, newOwnerUserID, adminID int) error
	// BulkDeleteGroups soft-deletes multiple quinielas. Succeeded contains IDs
	// that were deleted; Failed contains IDs already deleted or not found.
	BulkDeleteGroups(ctx context.Context, ids []int, adminID int) (BulkOperationResult, error)
	// BulkRemoveMembers sets multiple memberships to 'left'. Only memberships
	// that belong to quinielaID are affected; IDs from other groups are silently
	// ignored. Succeeded contains removed IDs; Failed contains IDs already
	// inactive, not found, or belonging to a different group.
	BulkRemoveMembers(ctx context.Context, quinielaID int, ids []int, adminID int) (BulkOperationResult, error)
	// RecalculateLeaderboard triggers an immediate leaderboard snapshot for the
	// given quiniela. Returns the newly created snapshot.
	RecalculateLeaderboard(ctx context.Context, quinielaID, adminID int) (*domain.LeaderboardSnapshot, error)
}

// BulkOperationResult is the outcome of a bulk administrative operation.
// Succeeded holds the IDs of entities that were processed; Failed holds IDs
// that could not be processed (not found or already in a terminal state).
type BulkOperationResult struct {
	Succeeded []int
	Failed    []int
}

// BulkBanError records a single ban failure within a BulkBan call.
type BulkBanError struct {
	UserID  int
	Message string
}

// BulkBanResult is the outcome of a BulkBan call. Banned holds the IDs of
// every user that was successfully banned; Failed holds the IDs and reasons
// for every user whose ban could not be completed. The caller is responsible
// for deciding the appropriate HTTP status (200 vs 207 Multi-Status).
type BulkBanResult struct {
	Banned []int
	Failed []BulkBanError
}

// AdminUserService exposes administrative operations on User accounts.
//
// BanUser and BulkBan automatically transfer group ownership when the banned
// user holds MembershipRoleCreateOwner in any quiniela - see
// GroupMembershipService for the transfer algorithm. The admin role gate is
// enforced at the HTTP layer; this service does not re-check it.
type AdminUserService interface {
	BanUser(ctx context.Context, targetUserID, adminID int, reason string) (*domain.User, error)
	UnbanUser(ctx context.Context, targetUserID, adminID int) (*domain.User, error)
	ListUsers(ctx context.Context) ([]*domain.User, error)
	// BulkBan bans every user in userIDs with the same reason. It processes
	// bans sequentially so that a failure on one user does not block the
	// remaining bans. Per-user failures are reported in BulkBanResult.Failed;
	// the outer error is reserved for unexpected, request-level failures.
	BulkBan(ctx context.Context, userIDs []int, adminID int, reason string) (BulkBanResult, error)
	// ListFiltered returns users matching the given filters with pagination.
	// Supersedes ListUsers for the admin panel where filters and paging are needed.
	ListFiltered(ctx context.Context, f repository.UserFilters, p repository.Pagination) ([]*domain.User, error)
	// GetProfile returns the full admin view of a user: base profile, active
	// group memberships, and payment records.
	GetProfile(ctx context.Context, userID int) (*AdminUserProfile, error)
}

// AdminUserProfile aggregates the data needed by the admin user-detail endpoint:
// the user row, all group memberships, and all payment records.
type AdminUserProfile struct {
	User        *domain.User
	Memberships []*domain.GroupMembership
	Payments    []*domain.PaymentRecord
}

// Snapshotter persists point-in-time leaderboard copies for a
// quiniela. It is called by the scoring worker immediately after ScoreMatch
// completes so the latest rankings are available without re-computing them.
type Snapshotter interface {
	Snapshot(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error)
}

// AuditReader exposes read-only access to the audit log for admin endpoints.
// It is implemented by the same concrete type as AuditLogger but is kept
// separate to apply the Interface Segregation Principle: callers that only
// write audits receive AuditLogger; those that need reads receive AuditReader.
type AuditReader interface {
	ListAuditLogs(ctx context.Context, f repository.AuditLogFilters, p repository.Pagination) ([]*domain.AuditLog, error)
	ListAuditLogsByEntity(ctx context.Context, resourceType string, resourceID int, p repository.Pagination) ([]*domain.AuditLog, error)
}

// AuditService is the combined interface for callers that need both write and
// read access to the audit trail - primarily the composition root (server.go)
// where the service is wired and passed to both audit-writing and
// audit-reading handlers.
type AuditService interface {
	AuditLogger
	AuditReader
	// Drain blocks until all in-flight audit Log goroutines complete. Must be
	// called during graceful shutdown before closing the database connection
	// pool to prevent losing audit entries that were queued but not yet persisted.
	Drain()
}

// DLQStat summarises the dead-letter queue for one event type.
type DLQStat struct {
	EventType string     `json:"event_type"`
	Count     int64      `json:"count"`
	OldestAt  *time.Time `json:"oldest_at,omitempty"`
	Sample    []DLQEntry `json:"sample"`
}

// DLQEntry is the payload of a single dead-lettered event, as returned by
// DLQService.Stats for inspection.
type DLQEntry struct {
	DeadLetteredAt time.Time      `json:"dead_lettered_at"`
	HandlerErr     string         `json:"handler_err"`
	Payload        map[string]any `json:"payload"`
}

// DLQService exposes management operations on the dead-letter queue.
// Implementations are driver-specific (Redis vs in-memory); pass a no-op
// implementation when the DLQ feature is not supported.
type DLQService interface {
	// Stats returns the count, oldest entry age, and a sample of messages
	// for each known event type.
	Stats(ctx context.Context) ([]DLQStat, error)
	// Replay re-enqueues up to limit entries from all DLQ keys back onto
	// their original streams. Returns the total number replayed.
	Replay(ctx context.Context, limit int) (int, error)
	// Purge deletes all entries from all DLQ keys.
	// Returns the total number of entries removed.
	Purge(ctx context.Context) (int64, error)
}

// TiebreakerSubmissionView pairs a tiebreaker prediction with its author's
// display name. Used by the admin tiebreaker submissions endpoint to avoid
// N+1 user lookups in the handler layer.
type TiebreakerSubmissionView struct {
	Submission *domain.Tiebreaker
	UserName   string
}

// AdminReadService handles cross-domain read queries used by admin panel
// endpoints that cannot be satisfied by a single existing service.
type AdminReadService interface {
	// GlobalLeaderboard returns the top limit users ranked by total scored
	// points across all quinielas.
	GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error)
	// ListPredictions returns predictions matching the given admin filters
	// with pagination.
	ListPredictions(ctx context.Context, f repository.PredictionAdminFilters, p repository.Pagination) ([]*domain.Prediction, error)
	// ListTiebreakerSubmissions returns all tiebreaker predictions with user
	// names resolved, paginated.
	ListTiebreakerSubmissions(ctx context.Context, p repository.Pagination) ([]TiebreakerSubmissionView, error)
	// ListSnapshotHistory returns the most recent limit snapshots for a quiniela.
	ListSnapshotHistory(ctx context.Context, quinielaID, limit int) ([]*domain.LeaderboardSnapshot, error)
	// GetDashboardStats returns aggregate counts for groups, users, and payments.
	// Intended for the admin dashboard home screen to populate summary widgets.
	GetDashboardStats(ctx context.Context) (*domain.DashboardStats, error)
}

// ConflictTypeSummary aggregates detected conflicts for a single conflict type.
type ConflictTypeSummary struct {
	Type       domain.ConflictType
	Count      int
	AvgAgeDays *float64 // nil when no age information is available for this type
}

// ConflictSummaryResult is the outcome of a ConflictSummary call.
// It provides per-type counts and average ages, enabling dashboards to surface
// an alert when unresolved conflicts are accumulating or getting stale.
type ConflictSummaryResult struct {
	TotalUnresolved int
	ByType          []ConflictTypeSummary
	// LimitReached is true when the conflict backlog equals or exceeds max_scan,
	// meaning the summary is incomplete and some conflicts were not included.
	// Dashboard widgets should display a warning when this flag is set.
	LimitReached bool
	// MaxScan is the configured limit applied to this scan (conflict.max_scan).
	// Provided for context when interpreting TotalUnresolved and LimitReached.
	MaxScan int
}

// ConflictService detects and resolves operational inconsistencies that require
// administrative attention. Conflicts are computed on demand; they are not
// persisted. Resolution records an audit log entry and is intended to
// acknowledge the conflict - the underlying issue must be fixed separately.
type ConflictService interface {
	// ListConflicts returns currently detected conflicts across all conflict
	// categories, sliced by p. A zero Pagination returns the full list.
	ListConflicts(ctx context.Context, p repository.Pagination) ([]domain.Conflict, error)
	// ConflictSummary returns an aggregated view of all detected conflicts
	// grouped by type, with count and average age per type. Intended for
	// dashboard alert widgets that need a lightweight summary without the
	// full conflict detail list.
	ConflictSummary(ctx context.Context) (*ConflictSummaryResult, error)
	// ResolveConflict records an admin action on the given conflict. action must
	// be "ack" (acknowledgement only) or "auto_fix" (attempt automatic remediation
	// - transfers ownership, rejects stale payments, or removes stale memberships).
	ResolveConflict(ctx context.Context, conflictType string, entityID, adminID int, action, note string) error
}
