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
// Join resolves the invite code to a Quiniela and creates or re-activates a
// membership for the caller. ListByQuiniela returns the full roster. ListByUser
// returns all groups a user belongs to, regardless of status.
//
// MarkPaid is called exclusively by the payment system after a transaction is
// confirmed. It must never be exposed as a direct API action — callers cannot
// mark themselves as paid. For free groups (entry_fee = 0), paid is set to
// true automatically at join time and this method is never invoked.
type GroupMembershipService interface {
	Join(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error)
	MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error)
	ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error)
}

// Notifier dispatches notifications in response to domain events.
//
// Notify is a fire-and-forget operation: failures are logged but not returned
// to the caller because notification delivery is best-effort and must not
// block or fail the primary operation that triggered the event.
type Notifier interface {
	Notify(ctx context.Context, userID int, message string) error
}
