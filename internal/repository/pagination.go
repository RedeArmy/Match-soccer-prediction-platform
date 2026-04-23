package repository

import "github.com/rede/world-cup-quiniela/internal/domain"

// Pagination carries LIMIT / OFFSET parameters for listing operations.
// A zero Limit means no upper bound; callers should always supply a positive
// value in production to prevent unbounded result sets.
type Pagination struct {
	Limit  int // 0 = no limit
	Offset int
}

// AuditLogFilters narrows an audit_log query. All non-nil fields are combined
// with AND. A zero-value AuditLogFilters returns every row.
type AuditLogFilters struct {
	ActorID      *int
	Action       *string
	ResourceType *string
	ResourceID   *int
}

// PaymentFilters narrows a payment_records query.
type PaymentFilters struct {
	Status     *domain.PaymentStatus
	QuinielaID *int
	UserID     *int
}

// UserFilters narrows a users query for admin listing.
type UserFilters struct {
	Banned *bool // nil = all, true = banned only, false = active only
	Role   *domain.UserRole
	Search *string // substring match on name or email
}

// PredictionAdminFilters narrows a predictions query for admin listing.
type PredictionAdminFilters struct {
	UserID     *int
	MatchID    *int
	QuinielaID *int
}
