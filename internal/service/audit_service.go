package service

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// Audit policy
//
// Every operation that mutates shared state on behalf of an actor is recorded.
// Read-only operations, background scoring, and Clerk webhook sync are not audited
// (no actor-driven intent). The complete set of audited actions is defined as
// AuditAction* constants in internal/domain/constants.go.
//
// Categories and their audited operations:
//
//   match      - match.created, match.started, match.result_set
//   tiebreaker - tiebreaker.question_set, tiebreaker.result_confirmed
//   tournament - tournament.slot_confirmed
//   group      - group.join_approved, group.renamed
//   admin_group- admin_group.deleted, admin_group.member_removed,
//                admin_group.settings_updated, admin_group.ownership_transferred,
//                admin_group.member_bulk_removed, admin_group.bulk_deleted,
//                admin_group.leaderboard_refreshed
//   admin_user - admin_user.banned, admin_user.unbanned
//   payment    - payment.created, payment.validated, payment.rejected
//   param      - param.updated
//   conflict   - conflict.acknowledged, conflict.auto_resolved
//
// The fire-and-forget model (goroutine + 5 s timeout) ensures audit writes never
// block or roll back a committed business operation. Audit failures are logged at
// WARN level only.

// auditService is the concrete implementation of AuditLogger.
type auditService struct {
	repo         repository.AuditLogRepository
	writeTimeout time.Duration
	log          *zap.Logger
	// wg tracks fire-and-forget goroutines launched by Log. Drain blocks until
	// all in-flight audit writes complete, preventing data loss during graceful shutdown.
	wg sync.WaitGroup
}

// NewAuditService constructs an auditService backed by the given repository.
// writeTimeout caps the time each fire-and-forget goroutine waits to persist
// an entry; pass defaultAuditWriteTimeout (5s) when no override is available.
func NewAuditService(repo repository.AuditLogRepository, writeTimeout time.Duration, log *zap.Logger) AuditService {
	return &auditService{repo: repo, writeTimeout: writeTimeout, log: log}
}

// Log persists an audit entry in a detached goroutine (fire-and-forget).
//
// The HTTP request context is intentionally NOT forwarded: a cancelled request
// context would abort the INSERT even though the primary operation already
// succeeded. Instead, a fresh background context with a 5-second timeout
// guarantees the write completes promptly without blocking the caller.
//
// Failures are logged at WARN level and silently swallowed - audit logging
// is a best-effort observability concern and must never propagate errors that
// would roll back an already-committed business operation.
//
// The goroutine is tracked via s.wg so that Drain() can block until all
// in-flight audit writes complete during graceful shutdown, preventing data loss.
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
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
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

// Drain blocks until all in-flight audit goroutines complete. Must be called
// during graceful shutdown before closing the database connection pool to
// prevent losing audit entries that were queued but not yet persisted.
//
// Drain is safe to call multiple times; subsequent calls are no-ops.
// The writeTimeout (default 5 s) caps the maximum time any single goroutine
// can block, so Drain returns within (writeTimeout * concurrency) in the
// worst case. With a 5 s timeout and 10 concurrent audit writes, Drain blocks
// for at most 5 s (all writes run concurrently, not sequentially).
func (s *auditService) Drain() {
	s.wg.Wait()
}

var _ AuditService = (*auditService)(nil)
