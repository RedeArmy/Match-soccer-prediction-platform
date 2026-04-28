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

func (s *adminGroupService) UpdateGroupSettings(ctx context.Context, quinielaID int, maxMembers *int, entryFee, adminID int) (*domain.Quiniela, error) {
	q, err := s.quinielaRepo.UpdateGroupSettings(ctx, quinielaID, maxMembers, entryFee)
	if err != nil {
		return nil, err
	}

	resType := "quiniela"
	role := domain.RoleAdmin
	meta := map[string]any{"entry_fee": entryFee}
	if maxMembers != nil {
		meta["max_members"] = *maxMembers
	}
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupSettingsUpdated, &resType, &quinielaID, meta)
	return q, nil
}

// TransferOwnership assigns MembershipRoleCreateOwner to newOwnerUserID and
// demotes the current owner to MembershipRoleMember. Both SetRole calls must
// succeed or the function returns an error — the caller should treat partial
// failure as a transient issue and retry.
func (s *adminGroupService) TransferOwnership(ctx context.Context, quinielaID, newOwnerUserID, adminID int) error {
	newMembership, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, newOwnerUserID)
	if err != nil {
		return err
	}
	if newMembership == nil || newMembership.Status != domain.MembershipActive {
		return apperrors.NotFound("new owner must be an active member of this group")
	}

	// Find and demote the current owner before promoting the new one so the
	// group is never left with two concurrent owners even on a partial failure.
	members, err := s.memberRepo.ListByQuiniela(ctx, quinielaID)
	if err != nil {
		return err
	}
	for _, m := range members {
		if m.Role == domain.MembershipRoleCreateOwner && m.UserID != newOwnerUserID {
			if err := s.memberRepo.SetRole(ctx, m.ID, domain.MembershipRoleMember); err != nil {
				return err
			}
			break
		}
	}

	if err := s.memberRepo.SetRole(ctx, newMembership.ID, domain.MembershipRoleCreateOwner); err != nil {
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
	result := BulkOperationResult{Succeeded: succeeded, Failed: failed}
	if len(succeeded) > 0 {
		resType := "quiniela"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionGroupBulkDeleted, &resType, nil, map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
		})
	}
	return result, nil
}

// BulkRemoveMembers sets multiple memberships to 'left' and records a single
// audit entry listing all succeeded and failed IDs.
func (s *adminGroupService) BulkRemoveMembers(ctx context.Context, ids []int, adminID int) (BulkOperationResult, error) {
	succeeded, err := s.memberRepo.BulkRemoveByAdmin(ctx, ids, adminID)
	if err != nil {
		return BulkOperationResult{}, err
	}
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
	result := BulkOperationResult{Succeeded: succeeded, Failed: failed}
	if len(succeeded) > 0 {
		resType := "group_membership"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &adminID, &role, domain.AuditActionMemberBulkRemoved, &resType, nil, map[string]any{
			"succeeded": succeeded,
			"failed":    failed,
		})
	}
	return result, nil
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
