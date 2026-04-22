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
	repo repository.AuditLogRepository
	log  *zap.Logger
}

// NewAuditService constructs an auditService backed by the given repository.
func NewAuditService(repo repository.AuditLogRepository, log *zap.Logger) AuditLogger {
	return &auditService{repo: repo, log: log}
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.repo.Create(ctx, entry); err != nil {
			s.log.Warn("audit log: failed to persist entry",
				zap.String("action", action),
				zap.Error(err),
			)
		}
	}()
}

var _ AuditLogger = (*auditService)(nil)
