package repository

import (
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

const (
	// unboundedLimit is the internal sentinel value for Pagination.Limit that
	// indicates "no upper bound". Using -1 instead of 0 prevents accidental
	// unbounded queries from zero-value Pagination{} structs. Code that needs
	// unlimited result sets must explicitly call Pagination.Unbounded().
	unboundedLimit = -1
)

// Pagination carries LIMIT / OFFSET parameters for listing operations.
//
// Production code must always use explicit pagination:
//   - Bounded queries: Pagination{Limit: 50, Offset: 0}
//   - Unbounded queries: Pagination.Unbounded() (use sparingly, only in controlled contexts)
//
// Zero-value Pagination{} (Limit=0) is NOT supported and will be rejected by
// repository methods. This prevents accidental full-table scans from zero-value
// structs passed between service layers.
type Pagination struct {
	Limit  int // positive = bounded, -1 = unbounded (via Unbounded()), 0 = invalid
	Offset int
}

// Unbounded returns a Pagination configured for unlimited result sets.
//
// Use this ONLY when:
//   - You need the complete dataset for an aggregation or batch operation
//   - The result set is known to be small (e.g., system config tables)
//   - You're in test code verifying full-dataset behavior
//
// For all other cases, use explicit positive Limit values to prevent memory
// exhaustion and unbounded query execution times.
func Unbounded() Pagination {
	return Pagination{Limit: unboundedLimit, Offset: 0}
}

// IsUnbounded returns true if this Pagination was created via Unbounded() and
// therefore has no upper limit on result set size.
func (p Pagination) IsUnbounded() bool {
	return p.Limit == unboundedLimit
}

// AuditLogFilters narrows an audit_log query. All non-nil fields are combined
// with AND. A zero-value AuditLogFilters returns every row.
type AuditLogFilters struct {
	ActorID      *int
	Action       *string
	ResourceType *string
	ResourceID   *int
	// CreatedAfter restricts results to entries created at or after this time.
	CreatedAfter *time.Time
	// CreatedBefore restricts results to entries created at or before this time.
	CreatedBefore *time.Time
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

// KYCProfileFilters narrows a kyc_profiles query for admin listing.
type KYCProfileFilters struct {
	Status *domain.KYCStatus // nil = all active review states
	Tier   *domain.KYCTier
}

// CursorPage carries parameters for keyset-based (cursor) listing.
//
// Keyset pagination eliminates the O(OFFSET) full-table scan of LIMIT/OFFSET
// by seeking directly to the row after the previous page's last ID. This makes
// large-page traversal O(1) in the seek and O(page_size) in the fetch,
// regardless of total table size.
//
// The Cursor field is opaque: clients receive it from the previous response's
// next_cursor field and pass it back unchanged. An empty Cursor means "start
// from the first page". Limit must be positive; zero panics (same invariant as
// Pagination.Limit), preventing accidental full-table scans from zero-value
// structs.
type CursorPage struct {
	Limit  int    // must be > 0
	Cursor string // opaque token from prior response; empty = first page
}
