package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// conflictService is the concrete implementation of ConflictService.
type conflictService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
	paymentRepo  repository.PaymentRecordRepository
	audit        AuditLogger
	log          *zap.Logger
}

// NewConflictService constructs a conflictService.
func NewConflictService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	paymentRepo repository.PaymentRecordRepository,
	audit AuditLogger,
	log *zap.Logger,
) ConflictService {
	return &conflictService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		paymentRepo:  paymentRepo,
		audit:        audit,
		log:          log,
	}
}

// ListConflicts returns all detected operational conflicts across all categories.
func (s *conflictService) ListConflicts(ctx context.Context) ([]domain.Conflict, error) {
	now := time.Now().UTC()
	staleThreshold := now.Add(-time.Duration(domain.ConflictStaleDays) * 24 * time.Hour)

	var conflicts []domain.Conflict
	conflicts = s.appendGroupOwnerConflicts(ctx, now, conflicts)
	conflicts = s.appendStalePaymentConflicts(ctx, now, staleThreshold, conflicts)
	conflicts = s.appendStaleMembershipConflicts(ctx, now, staleThreshold, conflicts)

	if conflicts == nil {
		return []domain.Conflict{}, nil
	}
	return conflicts, nil
}

func (s *conflictService) appendGroupOwnerConflicts(ctx context.Context, now time.Time, out []domain.Conflict) []domain.Conflict {
	groupIDs, err := s.memberRepo.ListGroupIDsWithoutOwner(ctx)
	if err != nil {
		s.log.Warn("conflict: failed to list groups without owner", zap.Error(err))
		return out
	}
	quinielas, err := s.quinielaRepo.ListByIDs(ctx, groupIDs)
	if err != nil {
		s.log.Warn("conflict: failed to load quiniela details for ownerless groups", zap.Error(err))
	}
	nameByID := make(map[int]string, len(quinielas))
	for _, q := range quinielas {
		nameByID[q.ID] = q.Name
	}
	for _, id := range groupIDs {
		out = append(out, domain.Conflict{
			Type:       domain.ConflictGroupNoOwner,
			EntityID:   id,
			EntityType: "quiniela",
			Details:    map[string]any{"name": nameByID[id]},
			DetectedAt: now,
		})
	}
	return out
}

func (s *conflictService) appendStalePaymentConflicts(ctx context.Context, now time.Time, threshold time.Time, out []domain.Conflict) []domain.Conflict {
	stalePayments, err := s.paymentRepo.ListStale(ctx, threshold)
	if err != nil {
		s.log.Warn("conflict: failed to list stale payments", zap.Error(err))
		return out
	}
	for _, p := range stalePayments {
		age := int(now.Sub(p.CreatedAt).Hours() / 24)
		out = append(out, domain.Conflict{
			Type:       domain.ConflictPaymentStale,
			EntityID:   p.ID,
			EntityType: "payment_record",
			Details: map[string]any{
				"quiniela_id": p.QuinielaID,
				"user_id":     p.UserID,
				"amount":      p.Amount,
				"age_days":    age,
			},
			DetectedAt: now,
		})
	}
	return out
}

func (s *conflictService) appendStaleMembershipConflicts(ctx context.Context, now time.Time, threshold time.Time, out []domain.Conflict) []domain.Conflict {
	staleMemberships, err := s.memberRepo.ListStalePending(ctx, threshold)
	if err != nil {
		s.log.Warn("conflict: failed to list stale memberships", zap.Error(err))
		return out
	}
	for _, m := range staleMemberships {
		age := int(now.Sub(m.CreatedAt).Hours() / 24)
		out = append(out, domain.Conflict{
			Type:       domain.ConflictMembershipStale,
			EntityID:   m.ID,
			EntityType: "group_membership",
			Details: map[string]any{
				"quiniela_id": m.QuinielaID,
				"user_id":     m.UserID,
				"age_days":    age,
			},
			DetectedAt: now,
		})
	}
	return out
}

// ResolveConflict records an admin acknowledgement of a conflict in the audit log.
// It does not automatically fix the underlying issue.
func (s *conflictService) ResolveConflict(ctx context.Context, conflictType string, entityID, adminID int, note string) error {
	resType := "conflict"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, "conflict.resolved", &resType, &entityID, map[string]any{
		"conflict_type": conflictType,
		"note":          note,
	})
	return nil
}

var _ ConflictService = (*conflictService)(nil)
