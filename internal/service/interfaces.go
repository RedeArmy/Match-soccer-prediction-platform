// Package service contains the application's business logic.
//
// Each service orchestrates one domain concern: it reads from and writes to
// repositories, enforces business rules, and emits domain events. Services
// must not be aware of HTTP or database implementation details — they operate
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

	"github.com/rede/world-cup-quiniela/internal/domain"
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
// Submit enforces the prediction deadline: it delegates to the domain
// validator which rejects submissions after kick-off. It also rejects
// duplicate predictions (one per user per match) by checking for an existing
// record before creating a new one. Update follows the same deadline rules and
// requires the caller to own the prediction being modified.
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
// of updated predictions — the caller's context is asynchronous.
type MatchScorer interface {
	ScoreMatch(ctx context.Context, matchID int) error
}

// Ranker computes leaderboard standings for a given Quiniela.
//
// GetLeaderboard returns the overall standings sorted descending by TotalPoints.
// GetPhaseLeaderboard returns standings restricted to a single tournament phase.
// Only active, paid members are included in both variants. Unscored predictions
// (nil points) are excluded from the aggregation. PrizeWinner is set to true on
// entries that rank within the prize positions computed from PrizeThreshold.
type Ranker interface {
	GetLeaderboard(ctx context.Context, quinielaID int) ([]*domain.LeaderboardEntry, error)
	GetPhaseLeaderboard(ctx context.Context, quinielaID int, phase domain.MatchPhase) ([]*domain.LeaderboardEntry, error)
}

// QuinielaService defines operations on the Quiniela entity.
//
// Create generates a unique invite code and records the owner as the first
// active member. GetByInviteCode enables the join flow: the caller obtains
// the quiniela from a short code before creating the membership.
type QuinielaService interface {
	Create(ctx context.Context, quiniela *domain.Quiniela) error
	GetByID(ctx context.Context, id int) (*domain.Quiniela, error)
	GetByInviteCode(ctx context.Context, code string) (*domain.Quiniela, error)
	GetByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error)
	// RotateInviteCode generates a new cryptographically random invite code for
	// the given quiniela and immediately invalidates the old one. ownerID is
	// checked: only the quiniela owner may rotate the code. The updated quiniela
	// (with the new code and reset expiry) is returned.
	RotateInviteCode(ctx context.Context, quinielaID, ownerID int) (*domain.Quiniela, error)
}

// GroupMembershipService manages user membership in Quinielas.
//
// Join resolves the invite code to a Quiniela and creates a pending join
// request — the user is NOT active until any existing active member calls
// ApproveJoin. ListByQuiniela returns the full roster. ListByUser returns all
// groups a user belongs to, regardless of status.
//
// ApproveJoin promotes a pending request to active. Any active member of the
// quiniela may approve — there is no admin-only gate. After approval the group
// status is synchronised: if active member count reaches MinMembersForActive
// the quiniela transitions from inactive to active.
//
// Leave lets a user remove themselves from a quiniela. Only the user themselves
// may call this; no admin or owner can remove another member. After leaving,
// the group status is re-evaluated and may become inactive.
//
// MarkPaid is called exclusively by the payment system after a transaction is
// confirmed. It must never be exposed as a direct API action — callers cannot
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
// predictions — not scoped to any single quiniela.
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
//     Predictions are global — one per user, applied to every group they
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
