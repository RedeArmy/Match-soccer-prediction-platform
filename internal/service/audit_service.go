package service

import (
	"context"
	"sync"
	"sync/atomic"
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
// Delivery model:
//
// Each Log call spawns a goroutine that attempts to write the entry up to
// auditMaxAttempts times before giving up. The goroutine is tracked by both a
// sync.WaitGroup (for Drain) and an atomic counter (for InFlight).
//
// A deferred recover() guards against repository panics: a panicking goroutine
// would otherwise crash the process while leaving wg.Done uncalled.
//
// On permanent write failure the entry is irrecoverably lost; the service
// emits a structured log event at Error level with audit_lost=true so that
// log-aggregation alert rules can page on-call when audit integrity is at risk.

const (
	auditMaxAttempts = 2
	auditRetryDelay  = 250 * time.Millisecond
)

// auditService is the concrete implementation of AuditLogger.
type auditService struct {
	repo         repository.AuditLogRepository
	writeTimeout time.Duration
	log          *zap.Logger
	// wg tracks fire-and-forget goroutines launched by Log. Drain blocks until
	// all in-flight audit writes complete, preventing data loss during graceful shutdown.
	wg sync.WaitGroup
	// inFlight counts goroutines currently executing. Exposed via InFlight()
	// for health checks and metrics exporters.
	inFlight atomic.Int64
}

// NewAuditService constructs an auditService backed by the given repository.
// writeTimeout caps the time each write attempt waits to persist an entry;
// pass 5*time.Second when no override is available.
func NewAuditService(repo repository.AuditLogRepository, writeTimeout time.Duration, log *zap.Logger) AuditService {
	return &auditService{repo: repo, writeTimeout: writeTimeout, log: log}
}

// Log persists an audit entry in a detached goroutine (fire-and-forget).
//
// The HTTP request context is intentionally NOT forwarded: a cancelled request
// context would abort the INSERT even though the primary operation already
// succeeded. Instead, a fresh background context with writeTimeout guarantees
// the write completes promptly without blocking the caller.
//
// Resilience guarantees:
//
//   - Panic recovery: a repository panic is caught by a deferred recover so
//     the process does not crash and wg.Done is always called.
//   - Retry: the write is attempted up to auditMaxAttempts times with a short
//     backoff between attempts, covering transient network and DB blips.
//   - Structured loss event: when all retries are exhausted the entry is lost
//     and a structured log event with audit_lost=true is emitted so monitoring
//     alert rules can detect audit-trail gaps before they become compliance issues.
//
// The goroutine is tracked via s.wg so that Drain() blocks during graceful
// shutdown and via s.inFlight for health checks.
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
	s.inFlight.Add(1)
	s.wg.Add(1)
	go func() {
		// recover() must be the innermost deferred call (i.e. last registered,
		// first to run) so it catches panics before the other defers execute.
		defer s.wg.Done()
		defer s.inFlight.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				s.log.Error("audit: goroutine panic recovered — entry permanently lost",
					zap.String("action", action),
					zap.Any("panic", r),
					zap.Bool("audit_lost", true),
				)
			}
		}()
		s.writeWithRetry(entry, action)
	}()
}

// writeWithRetry attempts to persist entry up to auditMaxAttempts times.
// Between attempts it sleeps auditRetryDelay to allow transient failures to
// clear. On permanent failure it emits a structured audit_lost event.
func (s *auditService) writeWithRetry(entry *domain.AuditLog, action string) {
	for attempt := 1; attempt <= auditMaxAttempts; attempt++ {
		ctx, cancel := context.WithTimeout(context.Background(), s.writeTimeout)
		err := s.repo.Create(ctx, entry)
		cancel()
		if err == nil {
			return
		}
		s.log.Warn("audit log: failed to persist entry",
			zap.String("action", action),
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", auditMaxAttempts),
			zap.Error(err),
		)
		if attempt < auditMaxAttempts {
			time.Sleep(auditRetryDelay)
		}
	}
	// All retries exhausted: the entry is permanently lost. Emit a structured
	// error that log-aggregation alert rules can match on audit_lost=true.
	s.log.Error("audit log: entry permanently lost after all retry attempts",
		zap.String("action", action),
		zap.Int64("in_flight", s.inFlight.Load()),
		zap.Bool("audit_lost", true),
	)
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
// can block, so Drain returns within (writeTimeout * auditMaxAttempts) in
// the worst case. With concurrent writes the effective wait is much shorter.
func (s *auditService) Drain() {
	s.wg.Wait()
}

// InFlight returns the number of audit goroutines currently executing.
func (s *auditService) InFlight() int64 {
	return s.inFlight.Load()
}

var _ AuditService = (*auditService)(nil)
