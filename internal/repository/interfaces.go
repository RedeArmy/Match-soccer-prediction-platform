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
}

// QuinielaRepository defines the persistence operations for the Quiniela
// entity.
//
// The repository stores and retrieves a Quiniela's metadata only; the
// embedded Predictions slice is hydrated separately by the service layer
// using PredictionRepository.ListByUser when the full detail view is
// required. This avoids loading the entire prediction set on every read
// (e.g. a list of quinielas for a user's dashboard).
type QuinielaRepository interface {
	Create(ctx context.Context, quiniela *domain.Quiniela) error
	GetByID(ctx context.Context, id int) (*domain.Quiniela, error)
	Update(ctx context.Context, quiniela *domain.Quiniela) error
	Delete(ctx context.Context, id int) error
	ListByOwner(ctx context.Context, ownerID int) ([]*domain.Quiniela, error)
}

// TiebreakerRepository defines the persistence operations for the Tiebreaker
// entity.
//
// GetByUserAndQuiniela enforces the invariant that each player may submit at
// most one tiebreaker forecast per quiniela. As with predictions, the service
// layer checks for an existing entry before creating a new one; the database
// must enforce a unique index on (user_id, quiniela_id) to eliminate the
// check-then-act race condition.
type TiebreakerRepository interface {
	Create(ctx context.Context, tb *domain.Tiebreaker) error
	GetByUserAndQuiniela(ctx context.Context, userID, quinielaID int) (*domain.Tiebreaker, error)
	Update(ctx context.Context, tb *domain.Tiebreaker) error
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.Tiebreaker, error)
}
