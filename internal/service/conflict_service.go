package service

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
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

// ListConflicts returns detected operational conflicts across all categories,
// sliced according to p. A zero Pagination (Limit == 0) returns the full list.
func (s *conflictService) ListConflicts(ctx context.Context, p repository.Pagination) ([]domain.Conflict, error) {
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
	return paginate(conflicts, p), nil
}

// paginate applies offset and limit to a slice. A zero Limit means no upper bound.
func paginate[T any](s []T, p repository.Pagination) []T {
	if p.Offset >= len(s) {
		return []T{}
	}
	s = s[p.Offset:]
	if p.Limit > 0 && len(s) > p.Limit {
		s = s[:p.Limit]
	}
	return s
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
	conflicts, err := s.ListConflicts(ctx, repository.Pagination{})
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

// ResolveConflict records an admin action on a conflict. action must be "ack"
// (acknowledgement only) or "auto_fix" (attempt automatic remediation). Any
// other value is rejected with a validation error so misconfigured clients
// cannot fall through to the ack branch silently.
// Auto-fix failures are logged at WARN level but do not fail the call —
// the audit entry is always written so the attempt is traceable.
func (s *conflictService) ResolveConflict(ctx context.Context, conflictType string, entityID, adminID int, action, note string) error {
	if action != "ack" && action != "auto_fix" {
		return apperrors.Validation(fmt.Sprintf("action %q is not valid; accepted: ack, auto_fix", action))
	}

	resType := "conflict"
	role := domain.RoleAdmin

	if action == "auto_fix" {
		if err := s.autoFix(ctx, conflictType, entityID, adminID); err != nil {
			s.log.Warn("conflict: auto_fix failed",
				zap.String("type", conflictType),
				zap.Int("entity_id", entityID),
				zap.Error(err),
			)
		}
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionConflictAutoResolved, &resType, &entityID, map[string]any{
			"conflict_type": conflictType,
			"note":          note,
		})
		return nil
	}

	s.audit.Log(ctx, &adminID, &role, domain.AuditActionConflictAcknowledged, &resType, &entityID, map[string]any{
		"conflict_type": conflictType,
		"note":          note,
	})
	return nil
}

// autoFix applies automatic remediation for the given conflict type.
// The remediation is best-effort: callers must handle a non-nil error by
// logging it rather than treating it as a hard failure.
func (s *conflictService) autoFix(ctx context.Context, conflictType string, entityID, adminID int) error {
	switch domain.ConflictType(conflictType) {
	case domain.ConflictGroupNoOwner:
		successor, err := s.memberRepo.OldestActiveMember(ctx, entityID, 0)
		if err != nil {
			return err
		}
		if successor == nil {
			return nil // no eligible successor; nothing to promote
		}
		return s.memberRepo.SetRole(ctx, successor.ID, domain.MembershipRoleCreateOwner)

	case domain.ConflictPaymentStale:
		_, err := s.paymentRepo.Reject(ctx, entityID, adminID, "Auto-rejected by conflict resolution")
		return err

	case domain.ConflictMembershipStale:
		return s.memberRepo.RemoveByAdmin(ctx, entityID, adminID)
	}
	return nil
}

var _ ConflictService = (*conflictService)(nil)
