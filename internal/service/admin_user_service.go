package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// adminUserService is the concrete implementation of AdminUserService.
type adminUserService struct {
	userRepo   repository.UserRepository
	memberRepo repository.GroupMembershipRepository
	audit      AuditLogger
	log        *zap.Logger
}

// NewAdminUserService constructs an adminUserService.
func NewAdminUserService(
	userRepo repository.UserRepository,
	memberRepo repository.GroupMembershipRepository,
	audit AuditLogger,
	log *zap.Logger,
) AdminUserService {
	return &adminUserService{
		userRepo:   userRepo,
		memberRepo: memberRepo,
		audit:      audit,
		log:        log,
	}
}

// BanUser bans the target user and automatically transfers ownership of any
// quiniela where the banned user holds MembershipRoleCreateOwner. Ownership
// transfer is best-effort: a failure is logged but does not roll back the ban.
func (s *adminUserService) BanUser(ctx context.Context, targetUserID, adminID int, reason string) (*domain.User, error) {
	banned, err := s.userRepo.Ban(ctx, targetUserID, adminID, reason)
	if err != nil {
		return nil, err
	}

	if err := s.transferOwnedGroups(ctx, targetUserID); err != nil {
		s.log.Warn("admin_user: ownership transfer after ban failed",
			zap.Int("banned_user_id", targetUserID),
			zap.Error(err))
	}

	resType := "user"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionUserBanned, &resType, &targetUserID, map[string]any{
		"reason": reason,
	})
	return banned, nil
}

// UnbanUser clears ban fields on the target user and records the action.
func (s *adminUserService) UnbanUser(ctx context.Context, targetUserID, adminID int) (*domain.User, error) {
	if err := s.userRepo.Unban(ctx, targetUserID); err != nil {
		return nil, err
	}
	user, err := s.userRepo.GetByID(ctx, targetUserID)
	if err != nil {
		return nil, err
	}

	resType := "user"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionUserUnbanned, &resType, &targetUserID, nil)
	return user, nil
}

func (s *adminUserService) ListUsers(ctx context.Context) ([]*domain.User, error) {
	return s.userRepo.List(ctx)
}

// BulkBan bans every user in userIDs sequentially. A per-user failure is
// logged and skipped so the remaining bans proceed. Returns the first error
// encountered, or nil when all bans succeeded.
func (s *adminUserService) BulkBan(ctx context.Context, userIDs []int, adminID int, reason string) error {
	var firstErr error
	for _, uid := range userIDs {
		if _, err := s.BanUser(ctx, uid, adminID, reason); err != nil {
			s.log.Warn("admin_user: bulk ban failed for user",
				zap.Int("user_id", uid), zap.Error(err))
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// transferOwnedGroups finds all quinielas where userID is the CreateOwner and
// transfers each to the oldest active member. Groups with no other active
// member are left without a CreateOwner — the group itself is unchanged.
func (s *adminUserService) transferOwnedGroups(ctx context.Context, userID int) error {
	memberships, err := s.memberRepo.ListByUser(ctx, userID)
	if err != nil {
		return err
	}
	for _, m := range memberships {
		if m.Role != domain.MembershipRoleCreateOwner || m.Status != domain.MembershipActive {
			continue
		}
		if err := s.doTransfer(ctx, m.QuinielaID, userID); err != nil {
			s.log.Warn("admin_user: ownership transfer failed for quiniela",
				zap.Int("quiniela_id", m.QuinielaID),
				zap.Int("outgoing_owner", userID),
				zap.Error(err))
		}
	}
	return nil
}

func (s *adminUserService) doTransfer(ctx context.Context, quinielaID, excludeUserID int) error {
	successor, err := s.memberRepo.OldestActiveMember(ctx, quinielaID, excludeUserID)
	if err != nil {
		return err
	}
	if successor == nil {
		return nil // no eligible member; group stays without a CreateOwner
	}
	return s.memberRepo.SetRole(ctx, successor.ID, domain.MembershipRoleCreateOwner)
}

var _ AdminUserService = (*adminUserService)(nil)
