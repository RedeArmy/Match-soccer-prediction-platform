// Package repository defines the persistence contracts for the application's
// domain entities.
//
// Each interface here represents the complete set of data operations that the
// service layer requires. Defining interfaces in this package - rather than
// alongside the concrete implementations - is the Dependency Inversion
// Principle applied deliberately: the high-level policy (service) does not
// depend on the low-level detail (PostgreSQL); both depend on the abstraction
// defined here.
//
// Concrete implementations live in internal/infrastructure/database and are
// wired to these interfaces at the composition root (cmd/api/main.go). This
// separation means the service layer can be tested with a simple in-memory
// stub that satisfies the interface, without spinning up a database.
package repository

import (
	"context"
	"errors"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// ErrPaymentIntentAlreadyCaptured is returned by PaymentIntentRepository.CaptureAndCredit
// when the intent was previously captured by the same captureID. The caller
// should treat this as a successful no-op (idempotent webhook re-delivery)
// and respond HTTP 204 without crediting the balance a second time.
var ErrPaymentIntentAlreadyCaptured = errors.New("payment intent already captured")

// UserRepository defines the persistence operations for the User entity.
//
// All methods accept context.Context as the first argument. This is not
// optional or ceremonial: the context carries cancellation signals from
// the HTTP layer (client disconnects, request timeouts). Propagating it
// to every database call ensures that a cancelled request releases its
// database connection promptly rather than holding it until the query
// completes naturally - a critical property under sustained load.
//
// Methods return pointer types (*domain.User) rather than value types to
// avoid copying potentially large structs on every call, and to allow nil
// as an unambiguous "not found" signal in callers that prefer that over a
// named sentinel error. The choice between nil-return and sentinel error is
// a team convention; whichever is chosen must be applied consistently across
// all repository interfaces to avoid surprising callers.
type UserRepository interface {
	Create(ctx context.Context, user *domain.User) error
	GetByID(ctx context.Context, id int) (*domain.User, error)
	// GetByClerkSubject resolves a Clerk opaque subject (e.g. "user_2abc…") to
	// an internal User. Returns nil, nil when no matching row exists so callers
	// can distinguish "not found" from a database error without importing
	// apperrors directly.
	GetByClerkSubject(ctx context.Context, subject string) (*domain.User, error)
	Update(ctx context.Context, user *domain.User) error
	Delete(ctx context.Context, id int) error
	List(ctx context.Context) ([]*domain.User, error)
	// ListByIDs fetches multiple users by primary key in a single query.
	// Used by the ranking service to hydrate leaderboard entries without N+1
	// queries. An empty ids slice returns nil, nil without hitting the database.
	ListByIDs(ctx context.Context, ids []int) ([]*domain.User, error)
	// Ban sets banned_at, banned_by, and ban_reason on the user record.
	// If the user is already banned the ban details are overwritten.
	// Returns NotFound for unknown or soft-deleted users.
	Ban(ctx context.Context, userID, adminID int, reason string) (*domain.User, error)
	// Unban clears the ban fields. Idempotent: unbanning an active user succeeds
	// silently. Returns NotFound for unknown or soft-deleted users.
	Unban(ctx context.Context, userID int) error
	// ListBanned returns all active users whose banned_at is not NULL.
	ListBanned(ctx context.Context) ([]*domain.User, error)
	// ListFiltered returns users matching the given filters with cursor-based
	// pagination. The second return value is an opaque cursor token for the next
	// page; it is empty when the caller has reached the last page.
	ListFiltered(ctx context.Context, f UserFilters, p CursorPage) ([]*domain.User, string, error)
	// GetStatusCounts returns user counts grouped by lifecycle status.
	// Used by the admin dashboard stats endpoint.
	GetStatusCounts(ctx context.Context) (UserStatusCounts, error)
	// GetBalance returns the current balance_cents and reserved_cents for
	// userID without fetching the full user row. Returns NotFound for unknown
	// or soft-deleted users.
	GetBalance(ctx context.Context, userID int) (balanceCents, reservedCents int, err error)
}

// MatchRepository defines the persistence operations for the Match entity.
//
// ListByStatus is provided as a first-class method rather than a filter
// parameter on List because it maps to an indexed query on the status column.
// Making it explicit encourages implementors to add the appropriate index
// and prevents callers from accidentally issuing a full-table scan by
// filtering in application code after retrieving all matches.
type MatchRepository interface {
	Create(ctx context.Context, match *domain.Match) error
	GetByID(ctx context.Context, id int) (*domain.Match, error)
	Update(ctx context.Context, match *domain.Match) error
	List(ctx context.Context) ([]*domain.Match, error)
	ListByPhase(ctx context.Context, phase domain.MatchPhase) ([]*domain.Match, error)
	ListByStatus(ctx context.Context, status domain.MatchStatus) ([]*domain.Match, error)
}

// PredictionRepository defines the persistence operations for the Prediction
// entity.
//
// GetByUserAndMatch enforces the uniqueness invariant that a user may submit
// at most one prediction per match. The service layer calls this method
// before creating a new prediction; the database should also enforce this
// via a unique index on (user_id, match_id) to prevent a race condition
// between the check and the insert.
type PredictionRepository interface {
	// Upsert inserts a new prediction or, when the (user_id, match_id) unique
	// constraint fires, performs a no-op UPDATE to obtain the existing row via
	// RETURNING. Returns created=true when a new row was inserted and
	// created=false when the existing row was returned unchanged. This makes
	// POST /predictions safe to retry without the client receiving a 409: the
	// second call returns the original resource with a 200 instead of an error.
	Upsert(ctx context.Context, prediction *domain.Prediction) (created bool, err error)
	Create(ctx context.Context, prediction *domain.Prediction) error
	GetByID(ctx context.Context, id int) (*domain.Prediction, error)
	Update(ctx context.Context, prediction *domain.Prediction) error
	// UpdateIfUnchanged applies the new scores only when prediction.updated_at
	// still equals expectedUpdatedAt. This optimistic-lock check prevents
	// concurrent editors from silently overwriting each other with a last-write-
	// wins outcome. Returns Conflict when another request has already updated
	// the row since the caller last read it.
	UpdateIfUnchanged(ctx context.Context, prediction *domain.Prediction, expectedUpdatedAt time.Time) error
	GetByUserAndMatch(ctx context.Context, userID, matchID int) (*domain.Prediction, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.Prediction, error)
	ListByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error)
	// UpdateManyPoints atomically sets the points column for every prediction ID
	// in the provided map inside a single database transaction. If any UPDATE
	// fails the transaction is rolled back and no scores are persisted, preventing
	// the partial-scoring state where some predictions on a finished match are
	// scored and others are not. An empty map is a no-op.
	UpdateManyPoints(ctx context.Context, points map[int]int) error
	// TotalPointsByQuiniela returns a map of userID -> total scored points for
	// every active, paid member of the given quiniela. It is used exclusively
	// by the ranking service to compute leaderboard standings in a single query,
	// avoiding N+1 database round-trips when the group is large.
	TotalPointsByQuiniela(ctx context.Context, quinielaID int) (map[int]int, error)
	// TotalPointsByQuinielaAndPhase is the phase-scoped variant of
	// TotalPointsByQuiniela. It restricts the point aggregation to predictions
	// on matches belonging to phase, enabling per-phase leaderboards (e.g. a
	// "group stage" standings table). Only active, paid members are included;
	// predictions with NULL points are excluded from the sum.
	TotalPointsByQuinielaAndPhase(ctx context.Context, quinielaID int, phase domain.MatchPhase) (map[int]int, error)
	// ListQuinielaIDsByMatch returns the distinct IDs of every quiniela that has
	// at least one active, paid member who submitted a prediction for matchID.
	// Called by the scoring worker after ScoreMatch to determine which quinielas
	// need a fresh leaderboard snapshot. Returns an empty slice (not an error)
	// when no active members have predictions for the match.
	ListQuinielaIDsByMatch(ctx context.Context, matchID int) ([]int, error)
	// ListByUserAndQuiniela returns all predictions for userID where the user is
	// an active member of the given quiniela. The EXISTS gate ensures that callers
	// who are not active members of quinielaID receive an empty slice, not an
	// error, keeping the response consistent whether the quiniela does not exist
	// or the user has simply made no predictions yet.
	ListByUserAndQuiniela(ctx context.Context, userID, quinielaID int) ([]*domain.Prediction, error)
	// PredictionStatsByQuiniela returns per-user prediction statistics for every
	// active, paid member of the given quiniela. The map is keyed by user ID;
	// members with no scored predictions appear with all counts at zero.
	// Used exclusively by the ranking service to resolve leaderboard ties when
	// two or more members share the same total points.
	PredictionStatsByQuiniela(ctx context.Context, quinielaID int) (map[int]*domain.UserPredictionStats, error)
	// GetUserPredictionCounts returns aggregated prediction counts and total
	// points for a single user across all quinielas. Used by UserStatsService
	// to build the user's performance profile in a single database round-trip.
	GetUserPredictionCounts(ctx context.Context, userID int) (*domain.UserPredictionCounts, error)
	// GetUserPointsByPhase returns a map of tournament phase to total scored
	// points for a single user. Only phases with at least one scored prediction
	// appear in the result. Used by UserStatsService to populate the
	// per-phase breakdown in UserStats.PointsByPhase.
	GetUserPointsByPhase(ctx context.Context, userID int) (map[domain.MatchPhase]int, error)
	// ListUserScoredPointsChronological returns the points values of all scored
	// predictions for a user, ordered ascending by their match's kickoff time.
	// Unscored predictions (points IS NULL) are excluded. The slice is consumed
	// by UserStatsService to derive CurrentStreak and LongestStreak.
	ListUserScoredPointsChronological(ctx context.Context, userID int) ([]int, error)
	// ListAdmin returns predictions matching the given admin filters with
	// pagination. Used exclusively by the admin panel; not exposed to players.
	ListAdmin(ctx context.Context, f PredictionAdminFilters, p Pagination) ([]*domain.Prediction, error)
	// GlobalLeaderboard returns the top `limit` users ranked by total scored
	// points across all quinielas. Used by the admin global leaderboard endpoint.
	GlobalLeaderboard(ctx context.Context, limit int) ([]*domain.GlobalLeaderboardEntry, error)
}

// QuinielaRepository defines the persistence operations for the Quiniela
// entity.
//
// The repository stores and retrieves a Quiniela's metadata only. Membership
// records are managed by GroupMembershipRepository. GetByInviteCode enables
// the join-by-code flow without exposing the internal ID in share links.
type QuinielaRepository interface {
	// CreateWithMembership atomically inserts the quiniela row and the owner's
	// initial membership in a single database transaction. Both writes succeed
	// or neither is committed, preventing orphaned quinielas that have no owner
	// membership. quiniela.ID and membership.ID are populated on success.
	CreateWithMembership(ctx context.Context, quiniela *domain.Quiniela, membership *domain.GroupMembership) error
	Create(ctx context.Context, quiniela *domain.Quiniela) error
	GetByID(ctx context.Context, id int) (*domain.Quiniela, error)
	// GetByInviteCode returns the quiniela matching code only when the code has
	// not expired (invite_code_expires_at IS NULL OR > NOW()). Returns nil, nil
	// for an unknown or expired code - callers should surface a 404 to the client
	// so that the difference between "wrong code" and "expired code" is not
	// exposed.
	GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error)
	// RotateInviteCode generates a new invite code and optional expiry for the
	// quiniela in a single atomic UPDATE. The old code is immediately invalidated.
	// expiresAt may be nil to create a non-expiring code.
	RotateInviteCode(ctx context.Context, id int, newCode string, expiresAt *time.Time) (*domain.Quiniela, error)
	// UpdateStatus sets the quiniela's active/inactive status in a single atomic
	// UPDATE. It is called exclusively by the membership service's syncGroupStatus
	// helper after every membership state transition. Returns NotFound when the
	// quiniela does not exist or has been soft-deleted.
	UpdateStatus(ctx context.Context, quinielaID int, status domain.QuinielaStatus) error
	Update(ctx context.Context, quiniela *domain.Quiniela) error
	Delete(ctx context.Context, id int) error
	ListByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error)
	// UpdateGroupSettings changes the entry_fee for a group. Returns the updated quiniela.
	UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee int) (*domain.Quiniela, error)
	// DeleteByAdmin soft-deletes a quiniela on behalf of an administrator.
	// The audit trail is the caller's responsibility via AuditLogRepository.
	DeleteByAdmin(ctx context.Context, quinielaID, adminID int) error
	// ListByIDs returns quinielas matching the given IDs in a single query.
	// Used by ConflictService to hydrate group details for conflict entries.
	// An empty ids slice returns nil, nil without hitting the database.
	ListByIDs(ctx context.Context, ids []int) ([]*domain.Quiniela, error)
	// GetStatusCounts returns quiniela counts grouped by lifecycle status.
	// Used by the admin dashboard stats endpoint.
	GetStatusCounts(ctx context.Context) (QuinielaStatusCounts, error)
	// BulkDeleteByAdmin soft-deletes multiple quinielas on behalf of an admin.
	// Returns the IDs that were successfully deleted. Already-deleted IDs are
	// silently skipped and do not appear in the result.
	BulkDeleteByAdmin(ctx context.Context, ids []int, adminID int) ([]int, error)
}

// GroupMembershipRepository defines the persistence operations for the
// GroupMembership entity.
//
// GetByQuinielaAndUser enforces the one-membership-per-user-per-quiniela
// invariant. ListByUser enables the "my groups" dashboard view.
// ListByQuiniela returns the full member roster for a group detail page.
// MarkPaid flips paid = true for the given member; it is called exclusively
// by the payment system after a transaction is confirmed.
type GroupMembershipRepository interface {
	Create(ctx context.Context, m *domain.GroupMembership) error
	// RequestJoinByInviteCode resolves inviteCode, serialises membership lookup
	// and mutation inside one transaction, and either inserts a new pending
	// membership or re-queues a left membership back to pending. The returned
	// quiniela is the locked group row used for the operation, allowing callers
	// to continue with side effects such as payment-record creation without a
	// second lookup. Returns NotFound for unknown or expired codes, and Conflict
	// when the user is already active, already pending, or the group is full.
	// maxMembers is the authoritative cap read from system_params by the caller;
	// it is applied inside the transaction after the quiniela FOR UPDATE lock.
	RequestJoinByInviteCode(ctx context.Context, inviteCode string, userID, maxMembers int) (*domain.Quiniela, *domain.GroupMembership, error)
	// GetByID returns a membership by its primary key. Returns nil, nil when no
	// matching row exists. Used by ApproveJoin to load the pending request.
	GetByID(ctx context.Context, membershipID int) (*domain.GroupMembership, error)
	GetByQuinielaAndUser(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	Update(ctx context.Context, m *domain.GroupMembership) error
	MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error)
	// ListActiveMemberIDsByGroup returns the user IDs of all active members of
	// quinielaID, excluding soft-deleted users. It is the narrow read used by
	// UserDispatcher to fan out broadcast events (e.g. EventGroupMemberJoined)
	// to every current group member without loading full membership rows.
	ListActiveMemberIDsByGroup(ctx context.Context, quinielaID int) ([]int, error)
	// CountActive returns the number of members with status=active in the given
	// quiniela. It is called exclusively by syncGroupStatus after every
	// membership transition to decide whether the quiniela should be set to
	// active or inactive.
	CountActive(ctx context.Context, quinielaID int) (int, error)
	// CountActivePaid returns the number of members with status=active AND
	// paid=true in the given quiniela. This is the authoritative count used by
	// the ranking service to determine prize eligibility and winner count via
	// domain.EligibleForPayments and domain.WinnerCount. It differs from
	// CountActive when some active members have not yet settled their entry fee.
	CountActivePaid(ctx context.Context, quinielaID int) (int, error)
	// OldestActiveMember returns the active membership with the earliest JoinedAt
	// in quinielaID, excluding excludeUserID. Returns nil, nil when no eligible
	// member exists. Used by the ownership-transfer logic to find the automatic
	// successor when the current owner leaves or is banned.
	OldestActiveMember(ctx context.Context, quinielaID, excludeUserID int) (*domain.GroupMembership, error)
	// SetRole updates the role field for a single membership. It is the only
	// path through which MembershipRole changes; the general Update method
	// deliberately does not touch role to prevent accidental privilege escalation.
	SetRole(ctx context.Context, membershipID int, role domain.MembershipRole) error
	// RemoveByAdmin soft-deletes a membership by setting its status to 'left'
	// on behalf of an administrator. Only active memberships can be removed;
	// returns NotFound for inactive or non-existent memberships.
	RemoveByAdmin(ctx context.Context, membershipID, adminID int) error
	// ListGroupIDsWithoutOwner returns quiniela IDs of active quinielas that
	// have no active MembershipRoleCreateOwner member. Used by ConflictService.
	ListGroupIDsWithoutOwner(ctx context.Context) ([]int, error)
	// ListStalePending returns pending memberships older than olderThan.
	// Used by ConflictService to surface unresolved join requests.
	ListStalePending(ctx context.Context, olderThan time.Time) ([]*domain.GroupMembership, error)
	// BulkRemoveByAdmin soft-deletes multiple memberships on behalf of an admin.
	// Only memberships that belong to quinielaID are affected; IDs from other
	// groups are silently ignored, preventing cross-group scope bypass.
	// Returns the IDs that were successfully removed. Already-inactive IDs are
	// silently skipped and do not appear in the result.
	BulkRemoveByAdmin(ctx context.Context, quinielaID int, ids []int, adminID int) ([]int, error)
	// DebitBalanceAndMarkPaid atomically deducts amountCents from the user's
	// available balance (balance_cents - reserved_cents), marks the membership
	// paid, and inserts a balance_ledger row in a single transaction.
	// Returns Conflict when the available balance is insufficient.
	DebitBalanceAndMarkPaid(ctx context.Context, quinielaID, userID, amountCents int) (*domain.GroupMembership, error)
	// TransferOwnershipRoles atomically demotes every current owner of quinielaID
	// to MembershipRoleMember and promotes newOwnerMembershipID to
	// MembershipRoleCreateOwner within a single database transaction. If either
	// UPDATE fails the transaction is rolled back and neither change persists,
	// preventing the ConflictGroupNoOwner state that arises from a partial write.
	// Returns NotFound when newOwnerMembershipID does not exist.
	TransferOwnershipRoles(ctx context.Context, quinielaID, newOwnerMembershipID int) error
	// ApproveMembership atomically promotes a pending membership to active and
	// recalculates the quiniela's status within a single database transaction.
	// minMembers is the minimum active-member count at or above which the
	// quiniela transitions to active. Returns Conflict when the row is no longer
	// pending - a concurrent approval committed between the caller's pre-flight
	// check and this call.
	// ApproveMembership atomically promotes a pending membership to active and
	// recalculates the quiniela's status in a single transaction.
	// maxMembers is the authoritative cap read from system_params by the caller;
	// it is enforced inside the transaction after a FOR UPDATE lock on the
	// quiniela row to prevent concurrent approvals from exceeding the limit.
	ApproveMembership(ctx context.Context, membershipID, quinielaID int, now time.Time, minMembers, maxMembers int) (*domain.GroupMembership, error)
	// LeaveMembership atomically marks a membership as left and recalculates
	// the quiniela's status within a single database transaction. Returns
	// Conflict when the membership is no longer active - e.g. an admin removed
	// the member concurrently between the caller's pre-flight check and this call.
	LeaveMembership(ctx context.Context, quinielaID, userID int, now time.Time, minMembers int) error
	// LeaveMembershipAndTransferOwnership atomically promotes successorMembershipID
	// to MembershipRoleCreateOwner, marks leavingUserID as left, and recalculates
	// the quiniela's status in one transaction. Used when the current owner leaves
	// so the group never commits a "no active owner" intermediate state.
	LeaveMembershipAndTransferOwnership(ctx context.Context, quinielaID, leavingUserID, successorMembershipID int, now time.Time, minMembers int) error
}

// TiebreakerRepository defines the persistence operations for the Tiebreaker
// entity.
//
// Each user may submit at most one prediction per TiebreakerConfig.
// TiebreakerConfigID scopes the prediction to the active question for the
// caller's group; the unique index on (user_id, tiebreaker_config_id)
// eliminates the check-then-act race condition in Submit.
type TiebreakerRepository interface {
	// Upsert inserts the tiebreaker or updates prediction when the (user_id)
	// unique constraint fires. Eliminates the read-then-write TOCTOU race in
	// TiebreakerService.Submit: concurrent identical requests both succeed and
	// converge to the same prediction value.
	Upsert(ctx context.Context, tb *domain.Tiebreaker) error
	// GetByUser returns the caller's prediction for the given configID.
	// Returns nil, nil when the user has not yet submitted for that config.
	GetByUser(ctx context.Context, userID, configID int) (*domain.Tiebreaker, error)
	Update(ctx context.Context, tb *domain.Tiebreaker) error
	// ListByUserIDs returns global tiebreaker predictions (configID = 1) for the
	// given user IDs. Kept for backward compatibility with the ranking service,
	// which still uses the platform-wide config. Use ListByUserIDsForConfig when
	// multiple configs are active. An empty ids slice returns nil, nil.
	ListByUserIDs(ctx context.Context, userIDs []int) ([]*domain.Tiebreaker, error)
	// ListByUserIDsForConfig returns predictions scoped to configID for the given
	// user IDs. Used when per-phase or per-group configs are active. An empty ids
	// slice returns nil, nil.
	ListByUserIDsForConfig(ctx context.Context, userIDs []int, configID int) ([]*domain.Tiebreaker, error)
	// ListAll returns all tiebreaker submissions with pagination.
	// Used by the admin panel to inspect all player predictions at once.
	ListAll(ctx context.Context, p Pagination) ([]*domain.Tiebreaker, error)
}

// TiebreakerConfigRepository manages tiebreaker question configurations.
//
// Configurations can be global (the original singleton at id=1), scoped to a
// tournament phase, scoped to a specific quiniela, or both. At most one config
// exists per scope combination; the database enforces this with partial unique
// indices. Upsert creates or replaces the global config; the scoped variants
// (UpsertForPhase, UpsertForQuiniela) create or replace their respective rows.
type TiebreakerConfigRepository interface {
	// Get returns the global tiebreaker configuration (phase IS NULL, quiniela_id IS NULL).
	// Returns nil, nil when no global question has been set yet.
	Get(ctx context.Context) (*domain.TiebreakerConfig, error)
	// GetByPhase returns the platform-wide config scoped to phase.
	// Returns nil, nil when no question has been set for that phase.
	GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.TiebreakerConfig, error)
	// GetByQuiniela returns the group-specific config for quinielaID.
	// Returns nil, nil when no group-specific question has been configured.
	GetByQuiniela(ctx context.Context, quinielaID int) (*domain.TiebreakerConfig, error)
	// Upsert sets or replaces the global tiebreaker question and returns the updated config.
	Upsert(ctx context.Context, question string) (*domain.TiebreakerConfig, error)
	// UpsertForPhase sets or replaces the phase-scoped question (quiniela_id IS NULL).
	UpsertForPhase(ctx context.Context, phase domain.MatchPhase, question string) (*domain.TiebreakerConfig, error)
	// UpsertForQuiniela sets or replaces the group-specific question (phase IS NULL).
	UpsertForQuiniela(ctx context.Context, quinielaID int, question string) (*domain.TiebreakerConfig, error)
	// SetResult records the confirmed numeric outcome for the global config.
	// Called once by the administrator after the tournament concludes.
	SetResult(ctx context.Context, result int) error
	// SetResultByID records the confirmed numeric outcome for any config by ID.
	// Returns NotFound when configID does not exist.
	SetResultByID(ctx context.Context, configID, result int) error
}

// TournamentRepository manages bracket position slots created and confirmed by
// the system administrator as teams advance through the tournament.
type TournamentRepository interface {
	// CreateSlot inserts a new named bracket slot. label must be unique.
	CreateSlot(ctx context.Context, label string) (*domain.TournamentSlot, error)
	// GetSlot returns a slot by ID. Returns nil, nil when not found.
	GetSlot(ctx context.Context, id int) (*domain.TournamentSlot, error)
	// ListSlots returns all slots ordered by id.
	ListSlots(ctx context.Context) ([]*domain.TournamentSlot, error)
	// ConfirmSlot sets the advancing team for the given slot.
	// Returns NotFound when the slot does not exist.
	ConfirmSlot(ctx context.Context, id, confirmedByUserID int, team string) (*domain.TournamentSlot, error)
}

// SystemParamRepository manages runtime-configurable key-value settings.
//
// Params are upserted - not inserted - so the table acts as a live
// configuration store. The type, category, and is_runtime columns are set at
// first creation (typically by a migration seed) and preserved on subsequent
// value updates via Set and BulkSet.
type SystemParamRepository interface {
	// Get returns the param for key. Returns nil, nil when the key is not
	// configured, so callers can fall back to a coded default without error.
	Get(ctx context.Context, key string) (*domain.SystemParam, error)
	// GetAll returns every param ordered by key.
	GetAll(ctx context.Context) ([]*domain.SystemParam, error)
	// GetByCategory returns all params whose category equals cat.
	GetByCategory(ctx context.Context, category string) ([]*domain.SystemParam, error)
	// Set upserts a single key-value pair, preserving type/category/is_runtime
	// on conflict. actorID is forwarded by the service layer for audit logging.
	Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error)
	// BulkSet upserts every entry in params atomically. A nil or empty map is a
	// no-op. actorID is forwarded by the service layer for audit logging.
	BulkSet(ctx context.Context, params map[string]string, actorID int) error
	// ResetToDefault restores value to default_value for the given key.
	// Returns the updated param, or nil if the key does not exist.
	ResetToDefault(ctx context.Context, key string) (*domain.SystemParam, error)
}

// AuditLogRepository provides append-only access to the audit_log table.
//
// No UPDATE or DELETE is ever issued; rows are immutable once written. The
// listing methods use cursor-based pagination: they return an opaque next-page
// token alongside the result slice. The token is empty on the final page.
// Keyset ordering is by id DESC (primary key), which is equivalent to
// creation-time ordering and guaranteed stable with no ties.
type AuditLogRepository interface {
	// Create inserts an immutable audit entry. entry.ID and entry.CreatedAt
	// are populated on success.
	Create(ctx context.Context, entry *domain.AuditLog) error
	// ListByEntity returns entries for a specific resource type and ID.
	ListByEntity(ctx context.Context, resourceType string, resourceID int, p CursorPage) ([]*domain.AuditLog, string, error)
	// ListByActor returns all entries attributed to actorID.
	ListByActor(ctx context.Context, actorID int, p CursorPage) ([]*domain.AuditLog, string, error)
	// ListByAction returns all entries whose action field matches exactly.
	ListByAction(ctx context.Context, action string, p CursorPage) ([]*domain.AuditLog, string, error)
	// List is the general query method; all non-nil filter fields are AND-ed.
	List(ctx context.Context, f AuditLogFilters, p CursorPage) ([]*domain.AuditLog, string, error)
}

// AdminNotificationLogCreator provides append-only write access to the
// admin_notification_log table.  Every admin email dispatch — whether
// successful or failed — produces one row.  Rows are never updated or deleted.
type AdminNotificationLogCreator interface {
	// Create inserts an immutable log entry.  entry.ID and entry.CreatedAt are
	// populated by the database on INSERT.
	Create(ctx context.Context, entry *domain.AdminNotificationLog) error
}

// NotificationDLQEntryCreator writes delivery failures to notification_dlq so
// they can be inspected and replayed by operators.
type NotificationDLQEntryCreator interface {
	// CreateEntry inserts a new DLQ record for a failed channel delivery.
	CreateEntry(ctx context.Context, entry *domain.NotificationDLQEntry) error
}

// NotificationDLQRepository is the full persistence contract for the
// notification dead-letter queue, including the replay-worker operations.
type NotificationDLQRepository interface {
	NotificationDLQEntryCreator

	// ClaimBatch returns up to limit unresolved entries whose attempts are
	// below maxAttempts and whose exponential back-off delay has elapsed.
	// The retry delay for attempt n is 2^n seconds.
	ClaimBatch(ctx context.Context, limit, maxAttempts int) ([]*domain.NotificationDLQEntry, error)

	// MarkResolved sets resolved_at = NOW() for the given entry, removing it
	// from future ClaimBatch results.
	MarkResolved(ctx context.Context, id int64) error

	// RecordFailure increments attempts, sets last_retry_at = NOW(), and
	// updates error_detail for the given entry.
	RecordFailure(ctx context.Context, id int64, errDetail string) error

	// CountUnresolved returns the number of entries where resolved_at IS NULL.
	CountUnresolved(ctx context.Context) (int64, error)
}

// UserNotificationRepository manages the per-user notification inbox.
type UserNotificationRepository interface {
	// Create inserts a new notification.  Uses ON CONFLICT (idempotency_key) DO
	// NOTHING so the outbox worker can safely retry.  entry.ID and entry.CreatedAt
	// are populated on a real insert; on conflict the struct is unchanged.
	// Returns (false, nil) when the idempotency key already exists.
	Create(ctx context.Context, n *domain.UserNotification) (inserted bool, err error)
	// List returns the notification inbox for a user, newest-first.
	// Pass unreadOnly=true to restrict to unread rows.
	// Results are paginated via limit+offset.
	List(ctx context.Context, userID, limit, offset int, unreadOnly bool) ([]*domain.UserNotification, error)
	// CountUnread returns the number of unread notifications for a user.
	CountUnread(ctx context.Context, userID int) (int, error)
	// MarkRead marks a single notification as read.
	// Returns apperrors.NotFound when the notification does not belong to the user.
	MarkRead(ctx context.Context, notificationID int64, userID int) error
	// MarkAllRead marks all unread notifications for a user as read.
	MarkAllRead(ctx context.Context, userID int) error
}

// NotificationPreferenceRepository manages per-user, per-event-type channel opt-in.
type NotificationPreferenceRepository interface {
	// Get returns the preference for a (userID, eventType) pair.
	// Returns apperrors.NotFound when no row exists (caller should apply defaults).
	Get(ctx context.Context, userID int, eventType string) (*domain.NotificationPreference, error)
	// ListByUser returns all preference rows for a user.
	ListByUser(ctx context.Context, userID int) ([]*domain.NotificationPreference, error)
	// Upsert inserts or updates a preference row (conflict on (user_id, event_type)).
	Upsert(ctx context.Context, pref *domain.NotificationPreference) error
	// DisableAllEmail records a global email opt-out for userID by upserting a
	// sentinel row (event_type = '*', channel_email = FALSE).  Idempotent.
	DisableAllEmail(ctx context.Context, userID int) error
	// GlobalEmailOptedOut reports whether userID has a global email opt-out
	// (sentinel row with event_type = '*' and channel_email = FALSE).
	// Returns (false, nil) when no sentinel row exists.
	GlobalEmailOptedOut(ctx context.Context, userID int) (bool, error)
}

// PushSubscriptionRepository manages Web Push (VAPID) subscriptions.
type PushSubscriptionRepository interface {
	// Create inserts a new subscription.  Uses ON CONFLICT (endpoint) DO UPDATE
	// to reactivate an existing subscription when the same browser re-registers.
	Create(ctx context.Context, sub *domain.PushSubscription) error
	// ListActiveByUser returns all active subscriptions for a user.
	ListActiveByUser(ctx context.Context, userID int) ([]*domain.PushSubscription, error)
	// DeleteByEndpoint removes the subscription matching the given endpoint.
	// Used when the user explicitly unsubscribes.
	DeleteByEndpoint(ctx context.Context, endpoint string) error
	// MarkInactive deactivates a subscription by ID.
	// Called when a push delivery returns HTTP 410 Gone.
	MarkInactive(ctx context.Context, id int64) error
	// UpdateLastUsed stamps last_used_at = NOW() for the given subscription.
	// Called after every successful push delivery so inactive-subscription
	// cleanup jobs can identify browsers that have not been reached recently.
	UpdateLastUsed(ctx context.Context, id int64) error
}

// PaymentRecordRepository manages entry-fee payment records.
//
// Records are created in pending state. Validate and Reject transition them
// to confirmed or rejected respectively; only pending records can transition.
// Refunded is a terminal state set outside this repository by the payment
// provider webhook handler.
type PaymentRecordRepository interface {
	// Create inserts a new payment record in pending state. record.ID is
	// populated on success.
	Create(ctx context.Context, record *domain.PaymentRecord) error
	// GetByID returns the record or nil, nil when not found.
	GetByID(ctx context.Context, id int) (*domain.PaymentRecord, error)
	// ListByQuiniela returns records for a group, optionally filtered by status.
	ListByQuiniela(ctx context.Context, quinielaID int, f PaymentFilters) ([]*domain.PaymentRecord, error)
	// ListByUser returns all records for a user across every quiniela.
	ListByUser(ctx context.Context, userID int) ([]*domain.PaymentRecord, error)
	// ListPending returns all records in pending state, oldest first.
	ListPending(ctx context.Context) ([]*domain.PaymentRecord, error)
	// Validate transitions a pending payment to confirmed. Returns NotFound
	// when the record does not exist or is not in pending state.
	Validate(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error)
	// ValidateAndMarkPaid atomically transitions a pending payment to confirmed
	// AND flips group_memberships.paid=true for the corresponding member.
	// Returns NotFound when the record does not exist or is not pending.
	ValidateAndMarkPaid(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error)
	// Reject transitions a pending payment to rejected. Returns NotFound when
	// the record does not exist or is not in pending state.
	Reject(ctx context.Context, id, adminID int, notes string) (*domain.PaymentRecord, error)
	// List returns payment records matching the given filters with pagination.
	// Used by the admin panel for the full payments list.
	List(ctx context.Context, f PaymentFilters, p Pagination) ([]*domain.PaymentRecord, error)
	// ListStale returns pending records older than olderThan.
	// Used by ConflictService to surface unreviewed payments.
	ListStale(ctx context.Context, olderThan time.Time) ([]*domain.PaymentRecord, error)
	// GetStatusCounts returns payment record counts grouped by lifecycle status.
	// Used by the admin dashboard stats endpoint.
	GetStatusCounts(ctx context.Context) (PaymentStatusCounts, error)
}

// LeaderboardSnapshotRepository persists point-in-time leaderboard copies.
//
// Snapshots are immutable after creation and are keyed by quiniela_id +
// taken_at. The most common access pattern is GetLatest, used by the API to
// serve the last confirmed rankings without re-computing them on every request.
type LeaderboardSnapshotRepository interface {
	// Create persists a new snapshot. snapshot.ID and snapshot.CreatedAt are
	// populated on success.
	Create(ctx context.Context, snapshot *domain.LeaderboardSnapshot) error
	// ListByQuiniela returns the most recent limit snapshots. A limit of 0
	// returns all snapshots for the quiniela.
	ListByQuiniela(ctx context.Context, quinielaID, limit int) ([]*domain.LeaderboardSnapshot, error)
	// GetLatest returns the most recently taken snapshot. Returns nil, nil when
	// no snapshot exists yet.
	GetLatest(ctx context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error)
}

// QuinielaStatusCounts groups quiniela counts by lifecycle status.
type QuinielaStatusCounts struct {
	Total    int
	Active   int
	Inactive int
	Deleted  int
}

// UserStatusCounts groups user counts by lifecycle status.
type UserStatusCounts struct {
	Total  int
	Active int
	Banned int
}

// PaymentStatusCounts groups payment record counts by lifecycle status.
// TotalCollected is the sum of confirmed payment amounts in minor currency units.
type PaymentStatusCounts struct {
	Pending        int
	Confirmed      int
	Rejected       int
	TotalCollected int
}

// Purger permanently removes soft-deleted rows that have aged beyond the
// configured retention window. The scoring worker calls it on a daily tick;
// the caller is responsible for computing olderThan from the configured
// retention duration.
//
// Only rows whose deleted_at is strictly before olderThan are removed.
// Foreign-key constraints may prevent deletion of rows that still have
// dependent records; such errors are returned without partial commits.
type Purger interface {
	// PurgeDeletedUsers permanently removes user rows soft-deleted before
	// olderThan. Returns the number of rows removed.
	PurgeDeletedUsers(ctx context.Context, olderThan time.Time) (int64, error)
	// PurgeDeletedQuinielas permanently removes quiniela rows soft-deleted
	// before olderThan. Returns the number of rows removed.
	PurgeDeletedQuinielas(ctx context.Context, olderThan time.Time) (int64, error)
	// PurgeOldSnapshots removes leaderboard_snapshots that fall outside the
	// most-recent keepLatestN window for each quiniela. The latest snapshot
	// per quiniela is always preserved regardless of keepLatestN. Returns the
	// number of rows deleted.
	PurgeOldSnapshots(ctx context.Context, keepLatestN int) (int64, error)
	// EraseUserPII anonymises or removes all personal data associated with
	// userID as required by the platform's data-retention policy. Must be
	// called before PurgeDeletedUsers so that the hard-delete does not fail
	// on ON DELETE RESTRICT foreign keys.
	//
	// The following operations are executed atomically in a single transaction:
	//   - audit_log.actor_id → NULL  (ImmutableAnonymise)
	//   - payment_records.user_id → NULL  (ImmutableAnonymise)
	//   - predictions rows deleted  (OperationalDelete)
	//   - tiebreakers rows deleted  (OperationalDelete)
	EraseUserPII(ctx context.Context, userID int) error
}

// ScoringRuleRepository defines persistence operations for per-phase scoring
// configuration. Implementations must guarantee that every valid MatchPhase
// has a corresponding row (seeded by migration 000063).
type ScoringRuleRepository interface {
	// List returns all scoring rules ordered by phase for admin display.
	List(ctx context.Context) ([]*domain.ScoringRule, error)
	// GetByPhase returns the rule for a specific phase, or nil if absent.
	GetByPhase(ctx context.Context, phase domain.MatchPhase) (*domain.ScoringRule, error)
	// Update persists new point values and the is_active flag for an existing
	// phase row. Returns NotFound when the phase has no seeded row.
	Update(ctx context.Context, rule *domain.ScoringRule) (*domain.ScoringRule, error)
}

// BalanceLedgerRepository owns all balance mutations for the users table and
// the corresponding immutable balance_ledger audit rows.  Every method
// executes atomically: it updates users.balance_cents / users.reserved_cents
// AND inserts a balance_ledger row inside a single database transaction.
//
// creatorID is the internal user ID of the admin or system actor responsible
// for the mutation; 0 is stored as NULL (system/webhook origin).
//
// Methods return apperrors.Conflict when a conditional UPDATE matches zero
// rows (e.g. insufficient available balance).
type BalanceLedgerRepository interface {
	// Credit adds deltaCents to balance_cents.  kind identifies the originating
	// operation (e.g. LedgerKindBankTransfer, LedgerKindWebhookRecurrente).
	Credit(ctx context.Context, userID, deltaCents int, kind domain.BalanceLedgerKind, refID int64, refType string, creatorID int) error
	// CreditIdempotent is like Credit but safe for webhook re-delivery: a
	// duplicate reference silently no-ops at the database level instead of
	// double-crediting the user.  Returns (true, nil) on a fresh credit,
	// (false, nil) for a duplicate reference, or (false, err) on failure.
	// reference must be non-empty.
	CreditIdempotent(ctx context.Context, userID, deltaCents int, kind domain.BalanceLedgerKind, reference string) (bool, error)
	// Debit subtracts deltaCents from available balance
	// (balance_cents - reserved_cents).  Returns Conflict when insufficient.
	Debit(ctx context.Context, userID, deltaCents int, kind domain.BalanceLedgerKind, refID int64, refType string, creatorID int) error
	// Reserve moves amountCents from available to reserved_cents.  Used when a
	// withdrawal request is created.  Returns Conflict when insufficient.
	Reserve(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error
	// ReleaseReservation decrements reserved_cents.  Used when a withdrawal is
	// rejected.  Returns Conflict when reserved_cents < amountCents.
	ReleaseReservation(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error
	// CommitReservation decrements both balance_cents and reserved_cents.  Used
	// when a withdrawal is processed.  Returns Conflict when reserved_cents < amountCents.
	CommitReservation(ctx context.Context, userID, amountCents int, refID int64, refType string, creatorID int) error
	// ListByUser returns ledger entries for userID ordered by created_at DESC.
	ListByUser(ctx context.Context, userID int, p Pagination) ([]*domain.BalanceLedger, error)
}

// BankTransferProofRepository manages bank transfer proof records and their
// admin review lifecycle.  Approve atomically credits the user's balance so
// there is no window in which the proof is approved but the funds are absent.
type BankTransferProofRepository interface {
	// Create inserts a new proof in pending status.  proof.ID and proof.CreatedAt
	// are populated on success.
	Create(ctx context.Context, proof *domain.BankTransferProof) error
	// GetByID returns the proof or nil, nil when not found.
	GetByID(ctx context.Context, id int) (*domain.BankTransferProof, error)
	// ListByUser returns all proofs for a user ordered by created_at DESC.
	ListByUser(ctx context.Context, userID int) ([]*domain.BankTransferProof, error)
	// ListPending returns all pending proofs ordered by created_at ASC.
	ListPending(ctx context.Context) ([]*domain.BankTransferProof, error)
	// ApproveAndCredit atomically transitions a pending proof to approved AND
	// credits proof.AmountCents to the user's balance AND inserts a ledger row.
	// Returns NotFound when the proof does not exist or is not pending.
	ApproveAndCredit(ctx context.Context, id, reviewerID int, notes string) (*domain.BankTransferProof, error)
	// Reject transitions a pending proof to rejected.  Returns NotFound when
	// the proof does not exist or is not in pending status.
	Reject(ctx context.Context, id, reviewerID int, notes string) (*domain.BankTransferProof, error)
}

// WithdrawalRequestRepository manages withdrawal lifecycle.  Operations that
// change the user's balance reservation are atomic: they update both the
// withdrawal_requests status and the users.reserved_cents column in one tx,
// then insert a balance_ledger row, eliminating partial-state windows.
type WithdrawalRequestRepository interface {
	// CreateAndReserve atomically inserts a new withdrawal request AND moves
	// amountCents from available balance to reserved_cents AND inserts a
	// ledger row.  Returns Conflict when available balance is insufficient.
	CreateAndReserve(ctx context.Context, req *domain.WithdrawalRequest) error
	// GetByID returns the request or nil, nil when not found.
	GetByID(ctx context.Context, id int) (*domain.WithdrawalRequest, error)
	// ListByUser returns all requests for a user ordered by created_at DESC.
	ListByUser(ctx context.Context, userID int) ([]*domain.WithdrawalRequest, error)
	// ListPending returns all pending requests ordered by created_at ASC.
	ListPending(ctx context.Context) ([]*domain.WithdrawalRequest, error)
	// Approve transitions a pending request to approved (status change only —
	// no balance mutation at this step).  Returns NotFound when not pending.
	Approve(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error)
	// RejectAndRelease atomically transitions a pending request to rejected AND
	// releases reserved_cents back to available AND inserts a ledger row.
	// Returns NotFound when the request does not exist or is not pending.
	RejectAndRelease(ctx context.Context, id, reviewerID int, notes string) (*domain.WithdrawalRequest, error)
	// MarkProcessedAndCommit atomically transitions an approved request to
	// processed AND commits the reserved amount (decrements both balance_cents
	// and reserved_cents) AND inserts a ledger row.  Returns NotFound when the
	// request does not exist or is not in approved status.
	MarkProcessedAndCommit(ctx context.Context, id int) (*domain.WithdrawalRequest, error)
}

// PaymentIntentRepository manages server-generated payment intent records used
// as opaque PayPal custom_id values.  The single-use token prevents a user
// from substituting another user's ID in the PayPal order metadata.
type PaymentIntentRepository interface {
	// Create persists a new pending intent. intent.ID and intent.CreatedAt are
	// populated on success.
	Create(ctx context.Context, intent *domain.PaymentIntent) error
	// CaptureAndCredit atomically transitions a pending, non-expired intent to
	// captured, stores captureID, and credits the intent's amount_cents to the
	// user's balance in a single database transaction.
	//
	// Returns ErrPaymentIntentAlreadyCaptured when the same captureID has
	// already been applied to this intent — the caller should treat this as a
	// successful no-op (idempotent webhook re-delivery).
	// Returns apperrors.Conflict when the intent is captured by a different
	// captureID (duplicate capture from a different PayPal transaction).
	// Returns apperrors.NotFound when no pending, non-expired intent matches token.
	CaptureAndCredit(ctx context.Context, token, captureID string) (*domain.PaymentIntent, error)
}

// NotificationTemplateRepository manages operator-editable notification content.
// Each row overrides the compiled Go default for a (event_type, locale) pair.
// The implementation must cache reads aggressively: Get is called on every
// notification dispatch and must not block the outbox worker loop with a DB
// round-trip on every call.
type NotificationTemplateRepository interface {
	// Get returns the stored template for (eventType, locale), or nil when no
	// override exists (caller should fall back to the compiled default).
	Get(ctx context.Context, eventType, locale string) (*domain.NotificationTemplate, error)
	// List returns all stored template overrides, ordered by event_type, locale.
	List(ctx context.Context) ([]*domain.NotificationTemplate, error)
	// Upsert creates or replaces a template override.  UpdatedAt is set to
	// now() server-side; the caller populates all other fields.
	Upsert(ctx context.Context, t *domain.NotificationTemplate) error
	// Delete removes the override for (eventType, locale), reverting to the
	// compiled Go default.  Returns nil when the row does not exist.
	Delete(ctx context.Context, eventType, locale string) error
}
