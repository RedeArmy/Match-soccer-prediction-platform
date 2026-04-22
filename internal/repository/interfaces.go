// Package repository defines the persistence contracts for the application's
// domain entities.
//
// Each interface here represents the complete set of data operations that the
// service layer requires. Defining interfaces in this package — rather than
// alongside the concrete implementations — is the Dependency Inversion
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
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// UserRepository defines the persistence operations for the User entity.
//
// All methods accept context.Context as the first argument. This is not
// optional or ceremonial: the context carries cancellation signals from
// the HTTP layer (client disconnects, request timeouts). Propagating it
// to every database call ensures that a cancelled request releases its
// database connection promptly rather than holding it until the query
// completes naturally — a critical property under sustained load.
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
	Create(ctx context.Context, prediction *domain.Prediction) error
	GetByID(ctx context.Context, id int) (*domain.Prediction, error)
	Update(ctx context.Context, prediction *domain.Prediction) error
	GetByUserAndMatch(ctx context.Context, userID, matchID int) (*domain.Prediction, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.Prediction, error)
	ListByMatch(ctx context.Context, matchID int) ([]*domain.Prediction, error)
	// UpdateManyPoints atomically sets the points column for every prediction ID
	// in the provided map inside a single database transaction. If any UPDATE
	// fails the transaction is rolled back and no scores are persisted, preventing
	// the partial-scoring state where some predictions on a finished match are
	// scored and others are not. An empty map is a no-op.
	UpdateManyPoints(ctx context.Context, points map[int]int) error
	// TotalPointsByQuiniela returns a map of userID → total scored points for
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
	// for an unknown or expired code — callers should surface a 404 to the client
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
	// GetByID returns a membership by its primary key. Returns nil, nil when no
	// matching row exists. Used by ApproveJoin to load the pending request.
	GetByID(ctx context.Context, membershipID int) (*domain.GroupMembership, error)
	GetByQuinielaAndUser(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	Update(ctx context.Context, m *domain.GroupMembership) error
	MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error)
	// CountActive returns the number of active members in the given quiniela. It
	// is called exclusively by syncGroupStatus after every membership transition
	// to decide whether the quiniela should be set to active or inactive.
	CountActive(ctx context.Context, quinielaID int) (int, error)
	// OldestActiveMember returns the active membership with the earliest JoinedAt
	// in quinielaID, excluding excludeUserID. Returns nil, nil when no eligible
	// member exists. Used by the ownership-transfer logic to find the automatic
	// successor when the current owner leaves or is banned.
	OldestActiveMember(ctx context.Context, quinielaID, excludeUserID int) (*domain.GroupMembership, error)
	// SetRole updates the role field for a single membership. It is the only
	// path through which MembershipRole changes; the general Update method
	// deliberately does not touch role to prevent accidental privilege escalation.
	SetRole(ctx context.Context, membershipID int, role domain.MembershipRole) error
}

// TiebreakerRepository defines the persistence operations for the Tiebreaker
// entity.
//
// Tiebreaker predictions are global: each user may submit exactly one estimate
// for the administrator-defined question, and that estimate applies uniformly
// to every group the user belongs to. The unique index on user_id eliminates
// the check-then-act race condition in Submit.
type TiebreakerRepository interface {
	Create(ctx context.Context, tb *domain.Tiebreaker) error
	// GetByUser returns the caller's global tiebreaker prediction.
	// Returns nil, nil when the user has not yet submitted.
	GetByUser(ctx context.Context, userID int) (*domain.Tiebreaker, error)
	Update(ctx context.Context, tb *domain.Tiebreaker) error
	// ListByUserIDs returns predictions for the given user IDs in a single
	// query. Used by the ranking service to load all relevant entries for a
	// group without N+1 round-trips. An empty slice is returned when no user
	// in userIDs has submitted. An empty ids slice returns nil, nil.
	ListByUserIDs(ctx context.Context, userIDs []int) ([]*domain.Tiebreaker, error)
}

// TiebreakerConfigRepository manages the singleton global tiebreaker
// configuration set by the system administrator.
//
// There is at most one row in the database (id=1). Upsert creates the row
// on first call and updates question on subsequent calls. SetResult may only
// be called after a question has been set; it is the service layer's
// responsibility to enforce this precondition before calling SetResult.
type TiebreakerConfigRepository interface {
	// Get returns the current global configuration.
	// Returns nil, nil when no question has been set yet.
	Get(ctx context.Context) (*domain.TiebreakerConfig, error)
	// Upsert sets or replaces the tiebreaker question and returns the updated config.
	Upsert(ctx context.Context, question string) (*domain.TiebreakerConfig, error)
	// SetResult records the confirmed numeric outcome. Called once by the
	// administrator after the tournament concludes.
	SetResult(ctx context.Context, result int) error
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
