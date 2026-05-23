package domain

import "time"

// ── Audit log ─────────────────────────────────────────────────────────────────

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

// ── Operational conflicts ─────────────────────────────────────────────────────

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

// ── Dashboard statistics ──────────────────────────────────────────────────────

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
