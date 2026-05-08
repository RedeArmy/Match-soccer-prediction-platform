package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// adminGroupService is the concrete implementation of AdminGroupService.
type adminGroupService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
	snapshotter  Snapshotter
	audit        AuditLogger
	log          *zap.Logger
}

// NewAdminGroupService constructs an adminGroupService.
func NewAdminGroupService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	snapshotter Snapshotter,
	audit AuditLogger,
	log *zap.Logger,
) AdminGroupService {
	return &adminGroupService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		snapshotter:  snapshotter,
		audit:        audit,
		log:          log,
	}
}

func (s *adminGroupService) DeleteGroup(ctx context.Context, quinielaID, adminID int) error {
	if err := s.quinielaRepo.DeleteByAdmin(ctx, quinielaID, adminID); err != nil {
		return err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupDeleted, &resType, &quinielaID, nil)
	return nil
}

func (s *adminGroupService) RemoveMember(ctx context.Context, membershipID, adminID int) error {
	if err := s.memberRepo.RemoveByAdmin(ctx, membershipID, adminID); err != nil {
		return err
	}

	resType := "group_membership"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionMemberRemoved, &resType, &membershipID, nil)
	return nil
}

func (s *adminGroupService) UpdateGroupSettings(ctx context.Context, quinielaID int, entryFee, adminID int) (*domain.Quiniela, error) {
	q, err := s.quinielaRepo.UpdateGroupSettings(ctx, quinielaID, entryFee)
	if err != nil {
		return nil, err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupSettingsUpdated, &resType, &quinielaID, map[string]any{"entry_fee": entryFee})
	return q, nil
}

// TransferOwnership assigns MembershipRoleCreateOwner to newOwnerUserID and
// demotes the current owner to MembershipRoleMember atomically. Both role
// changes occur in a single database transaction via TransferOwnershipRoles,
// so a partial failure cannot leave the group without an owner.
func (s *adminGroupService) TransferOwnership(ctx context.Context, quinielaID, newOwnerUserID, adminID int) error {
	newMembership, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, newOwnerUserID)
	if err != nil {
		return err
	}
	if newMembership == nil || newMembership.Status != domain.MembershipActive {
		return apperrors.NotFound("new owner must be an active member of this group")
	}

	if err := s.memberRepo.TransferOwnershipRoles(ctx, quinielaID, newMembership.ID); err != nil {
		return err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionOwnershipTransferred, &resType, &quinielaID, map[string]any{
		"new_owner_user_id": newOwnerUserID,
	})
	return nil
}

// BulkDeleteGroups soft-deletes multiple quinielas and records a single audit
// entry listing all succeeded and failed IDs.
func (s *adminGroupService) BulkDeleteGroups(ctx context.Context, ids []int, adminID int) (BulkOperationResult, error) {
	succeeded, err := s.quinielaRepo.BulkDeleteByAdmin(ctx, ids, adminID)
	if err != nil {
		return BulkOperationResult{}, err
	}
	result := diffBulkResult(ids, succeeded)
	if len(succeeded) > 0 {
		resType := "quiniela"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupBulkDeleted, &resType, nil, map[string]any{
			"succeeded": result.Succeeded,
			"failed":    result.Failed,
		})
	}
	return result, nil
}

// BulkRemoveMembers sets multiple memberships to 'left' and records a single
// audit entry listing all succeeded and failed IDs. Only memberships that
// belong to quinielaID are affected; the repo filters by quiniela_id to
// prevent cross-group scope bypass.
func (s *adminGroupService) BulkRemoveMembers(ctx context.Context, quinielaID int, ids []int, adminID int) (BulkOperationResult, error) {
	succeeded, err := s.memberRepo.BulkRemoveByAdmin(ctx, quinielaID, ids, adminID)
	if err != nil {
		return BulkOperationResult{}, err
	}
	result := diffBulkResult(ids, succeeded)
	if len(succeeded) > 0 {
		resType := "group_membership"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionMemberBulkRemoved, &resType, nil, map[string]any{
			"succeeded": result.Succeeded,
			"failed":    result.Failed,
		})
	}
	return result, nil
}

// diffBulkResult computes which of the requested ids were not returned in
// succeeded and returns a BulkOperationResult with both slices populated.
func diffBulkResult(ids, succeeded []int) BulkOperationResult {
	succeededSet := make(map[int]bool, len(succeeded))
	for _, id := range succeeded {
		succeededSet[id] = true
	}
	var failed []int
	for _, id := range ids {
		if !succeededSet[id] {
			failed = append(failed, id)
		}
	}
	return BulkOperationResult{Succeeded: succeeded, Failed: failed}
}

// RecalculateLeaderboard triggers an immediate snapshot and records an audit entry.
func (s *adminGroupService) RecalculateLeaderboard(ctx context.Context, quinielaID, adminID int) (*domain.LeaderboardSnapshot, error) {
	snap, err := s.snapshotter.Snapshot(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	resType := "quiniela"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionLeaderboardRefreshed, &resType, &quinielaID, nil)
	return snap, nil
}

var _ AdminGroupService = (*adminGroupService)(nil)
