package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// BulkBanError records a single ban failure within a BulkBan call.
type BulkBanError struct {
	UserID  int
	Message string
}

// BulkBanResult is the outcome of a BulkBan call. Banned holds the IDs of
// every user that was successfully banned; Failed holds the IDs and reasons
// for every user whose ban could not be completed. The caller is responsible
// for deciding the appropriate HTTP status (200 vs 207 Multi-Status).
type BulkBanResult struct {
	Banned []int
	Failed []BulkBanError
}

// AdminUserProfile aggregates the data needed by the admin user-detail endpoint:
// the user row, all group memberships, and all payment records.
type AdminUserProfile struct {
	User        *domain.User
	Memberships []*domain.GroupMembership
	Payments    []*domain.PaymentRecord
}

// AdminUserService exposes administrative operations on User accounts.
//
// BanUser and BulkBan automatically transfer group ownership when the banned
// user holds MembershipRoleCreateOwner in any quiniela - see
// GroupMembershipService for the transfer algorithm. The admin role gate is
// enforced at the HTTP layer; this service does not re-check it.
type AdminUserService interface {
	BanUser(ctx context.Context, targetUserID, adminID int, reason string) (*domain.User, error)
	UnbanUser(ctx context.Context, targetUserID, adminID int) (*domain.User, error)
	ListUsers(ctx context.Context) ([]*domain.User, error)
	// BulkBan bans every user in userIDs with the same reason. It processes
	// bans sequentially so that a failure on one user does not block the
	// remaining bans. Per-user failures are reported in BulkBanResult.Failed;
	// the outer error is reserved for unexpected, request-level failures.
	BulkBan(ctx context.Context, userIDs []int, adminID int, reason string) (BulkBanResult, error)
	// ListFiltered returns users matching the given filters with cursor-based
	// pagination. The second return value is an opaque next-page cursor; empty
	// on the last page.
	ListFiltered(ctx context.Context, f repository.UserFilters, p repository.CursorPage) ([]*domain.User, string, error)
	// GetProfile returns the full admin view of a user: base profile, active
	// group memberships, and payment records.
	GetProfile(ctx context.Context, userID int) (*AdminUserProfile, error)
}

// adminUserService is the concrete implementation of AdminUserService.
type adminUserService struct {
	userRepo    repository.UserRepository
	memberRepo  repository.GroupMembershipRepository
	paymentRepo repository.PaymentRecordRepository
	audit       AuditLogger
	log         *zap.Logger
}

// NewAdminUserService constructs an adminUserService.
func NewAdminUserService(
	userRepo repository.UserRepository,
	memberRepo repository.GroupMembershipRepository,
	paymentRepo repository.PaymentRecordRepository,
	audit AuditLogger,
	log *zap.Logger,
) AdminUserService {
	return &adminUserService{
		userRepo:    userRepo,
		memberRepo:  memberRepo,
		paymentRepo: paymentRepo,
		audit:       audit,
		log:         log,
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

// BulkBan bans every user in userIDs sequentially. Per-user failures are
// collected in BulkBanResult.Failed so the caller can report partial success;
// the remaining bans always proceed regardless of individual failures.
func (s *adminUserService) BulkBan(ctx context.Context, userIDs []int, adminID int, reason string) (BulkBanResult, error) {
	result := BulkBanResult{
		Banned: make([]int, 0, len(userIDs)),
		Failed: make([]BulkBanError, 0),
	}
	for _, uid := range userIDs {
		if _, err := s.BanUser(ctx, uid, adminID, reason); err != nil {
			s.log.Warn("admin_user: bulk ban failed for user",
				zap.Int("user_id", uid), zap.Error(err))
			result.Failed = append(result.Failed, BulkBanError{UserID: uid, Message: err.Error()})
		} else {
			result.Banned = append(result.Banned, uid)
		}
	}
	return result, nil
}

// transferOwnedGroups finds all quinielas where userID is the CreateOwner and
// transfers each to the oldest active member. Groups with no other active
// member are left without a CreateOwner - the group itself is unchanged.
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

// ListFiltered returns users matching the given filters with cursor-based pagination.
func (s *adminUserService) ListFiltered(ctx context.Context, f repository.UserFilters, p repository.CursorPage) ([]*domain.User, string, error) {
	return s.userRepo.ListFiltered(ctx, f, p)
}

// GetProfile returns the full admin view: user row, memberships, and payment records.
func (s *adminUserService) GetProfile(ctx context.Context, userID int) (*AdminUserProfile, error) {
	user, err := s.userRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	memberships, err := s.memberRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	payments, err := s.paymentRepo.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}

	return &AdminUserProfile{
		User:        user,
		Memberships: memberships,
		Payments:    payments,
	}, nil
}

var _ AdminUserService = (*adminUserService)(nil)
