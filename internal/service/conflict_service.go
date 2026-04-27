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
	params       SystemParamService
	audit        AuditLogger
	log          *zap.Logger
}

// NewConflictService constructs a conflictService.
func NewConflictService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	paymentRepo repository.PaymentRecordRepository,
	params SystemParamService,
	audit AuditLogger,
	log *zap.Logger,
) ConflictService {
	return &conflictService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		paymentRepo:  paymentRepo,
		params:       params,
		audit:        audit,
		log:          log,
	}
}

// ListConflicts returns all detected operational conflicts across all categories.
func (s *conflictService) ListConflicts(ctx context.Context) ([]domain.Conflict, error) {
	now := time.Now().UTC()
	staleDays := s.params.GetInt(ctx, domain.ParamKeyConflictStaleDays, domain.ConflictStaleDays)
	staleThreshold := now.Add(-time.Duration(staleDays) * 24 * time.Hour)

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
			AgeDays:    &age,
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
			AgeDays:    &age,
		})
	}
	return out
}

// ConflictSummary aggregates the live conflict list into per-type counts and
// average ages, suitable for a lightweight dashboard alert widget.
func (s *conflictService) ConflictSummary(ctx context.Context) (*ConflictSummaryResult, error) {
	conflicts, err := s.ListConflicts(ctx)
	if err != nil {
		return nil, err
	}

	type agg struct {
		count    int
		totalAge int
		hasAge   int // number of conflicts with an AgeDays value
	}
	byType := make(map[domain.ConflictType]*agg)
	for _, c := range conflicts {
		a := byType[c.Type]
		if a == nil {
			a = &agg{}
			byType[c.Type] = a
		}
		a.count++
		if c.AgeDays != nil {
			a.totalAge += *c.AgeDays
			a.hasAge++
		}
	}

	summaries := make([]ConflictTypeSummary, 0, len(byType))
	for ct, a := range byType {
		s := ConflictTypeSummary{Type: ct, Count: a.count}
		if a.hasAge > 0 {
			avg := float64(a.totalAge) / float64(a.hasAge)
			s.AvgAgeDays = &avg
		}
		summaries = append(summaries, s)
	}

	return &ConflictSummaryResult{
		TotalUnresolved: len(conflicts),
		ByType:          summaries,
	}, nil
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
