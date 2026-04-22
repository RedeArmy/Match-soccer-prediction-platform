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
	Status *domain.PaymentStatus
}
