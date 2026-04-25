package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// auditService is the concrete implementation of AuditLogger.
type auditService struct {
	repo         repository.AuditLogRepository
	writeTimeout time.Duration
	log          *zap.Logger
}

// NewAuditService constructs an auditService backed by the given repository.
// writeTimeout caps the time each fire-and-forget goroutine waits to persist
// an entry; pass defaultAuditWriteTimeout (5s) when no override is available.
// The return type is *auditService so callers can use it as both AuditLogger
// and AuditReader without a second constructor.
func NewAuditService(repo repository.AuditLogRepository, writeTimeout time.Duration, log *zap.Logger) *auditService {
	return &auditService{repo: repo, writeTimeout: writeTimeout, log: log}
}

// Log persists an audit entry in a detached goroutine (fire-and-forget).
//
// The HTTP request context is intentionally NOT forwarded: a cancelled request
// context would abort the INSERT even though the primary operation already
// succeeded. Instead, a fresh background context with a 5-second timeout
// guarantees the write completes promptly without blocking the caller.
//
// Failures are logged at WARN level and silently swallowed — audit logging
// is a best-effort observability concern and must never propagate errors that
// would roll back an already-committed business operation.
func (s *auditService) Log(
	_ context.Context,
	actorID *int,
	actorRole *domain.UserRole,
	action string,
	resourceType *string,
	resourceID *int,
	metadata map[string]any,
) {
	entry := &domain.AuditLog{
		ActorID:      actorID,
		ActorRole:    actorRole,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     metadata,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), s.writeTimeout)
		defer cancel()
		if err := s.repo.Create(ctx, entry); err != nil {
			s.log.Warn("audit log: failed to persist entry",
				zap.String("action", action),
				zap.Error(err),
			)
		}
	}()
}

// ListAuditLogs returns audit log entries matching the given filters.
func (s *auditService) ListAuditLogs(ctx context.Context, f repository.AuditLogFilters, p repository.Pagination) ([]*domain.AuditLog, error) {
	return s.repo.List(ctx, f, p)
}

// ListAuditLogsByEntity returns audit log entries for a specific resource.
func (s *auditService) ListAuditLogsByEntity(ctx context.Context, resourceType string, resourceID int, p repository.Pagination) ([]*domain.AuditLog, error) {
	return s.repo.ListByEntity(ctx, resourceType, resourceID, p)
}

var _ AuditLogger = (*auditService)(nil)
var _ AuditReader = (*auditService)(nil)
