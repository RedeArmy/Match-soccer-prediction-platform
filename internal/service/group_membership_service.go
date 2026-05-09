package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

// groupMembershipService is the concrete implementation of GroupMembershipService.
type groupMembershipService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
	authz        GroupAuthz
	params       SystemParamService
	audit        AuditLogger
	paymentSvc   PaymentService
	clock        clock.Nower
	log          *zap.Logger
}

// NewGroupMembershipService constructs a groupMembershipService.
func NewGroupMembershipService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	authz GroupAuthz,
	params SystemParamService,
	audit AuditLogger,
	paymentSvc PaymentService,
	clk clock.Nower,
	log *zap.Logger,
) GroupMembershipService {
	return &groupMembershipService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
		authz:        authz,
		params:       params,
		audit:        audit,
		paymentSvc:   paymentSvc,
		clock:        clk,
		log:          log,
	}
}

// Join resolves invite_code to a Quiniela and creates a pending join request.
// The user is NOT active until any existing active member calls ApproveJoin.
// If the user was previously a member but left, they are re-queued as pending
// for a new approval round.
func (s *groupMembershipService) Join(ctx context.Context, inviteCode string, userID int) (*domain.GroupMembership, error) {
	maxMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMaxSize, domain.MaxMembersPerGroup)
	quiniela, m, err := s.memberRepo.RequestJoinByInviteCode(ctx, inviteCode, userID, maxMembers)
	if err != nil {
		return nil, err
	}
	if quiniela.EntryFee > 0 {
		s.createPendingPayment(ctx, quiniela, userID)
	}
	return m, nil
}

// createPendingPayment creates a payment_record with status=pending for the
// joining user. Errors are logged and swallowed: the membership is already
// persisted and a missing payment record is a recoverable admin concern, not
// a reason to roll back the join request.
func (s *groupMembershipService) createPendingPayment(ctx context.Context, quiniela *domain.Quiniela, userID int) {
	if _, err := s.paymentSvc.CreateRecord(ctx, quiniela.ID, userID, quiniela.EntryFee, quiniela.Currency, ""); err != nil {
		s.log.Warn("membership: failed to create pending payment record on join",
			zap.Int("quiniela_id", quiniela.ID),
			zap.Int("user_id", userID),
			zap.Error(err),
		)
	}
}

// ApproveJoin promotes a pending membership to active. The approverUserID must
// be an active member of the same quiniela - any member may approve; there is
// no admin-only gate. The membership update and group status recalculation are
// committed atomically via ApproveMembership.
func (s *groupMembershipService) ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error) {
	if err := s.authz.RequireActiveMember(ctx, quinielaID, approverUserID); err != nil {
		return nil, err
	}

	pending, err := s.memberRepo.GetByID(ctx, membershipID)
	if err != nil {
		return nil, err
	}
	if pending == nil || pending.QuinielaID != quinielaID {
		return nil, apperrors.NotFound("join request not found")
	}
	if pending.Status != domain.MembershipPending {
		return nil, apperrors.Conflict("this join request is no longer pending")
	}

	minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
	maxMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMaxSize, domain.MaxMembersPerGroup)
	m, err := s.memberRepo.ApproveMembership(ctx, membershipID, quinielaID, s.clock.Now(), minMembers, maxMembers)
	if err != nil {
		return nil, err
	}

	resType := "membership"
	s.audit.Log(ctx, &approverUserID, nil, domain.AuditActionJoinApproved, &resType, &m.ID, map[string]any{
		"quiniela_id":  quinielaID,
		"approved_uid": m.UserID,
	})
	return m, nil
}

// Leave sets the caller's own membership to left. Only the member themselves
// may call this - no admin or owner can remove another user. If the leaving
// user holds MembershipRoleCreateOwner, ownership is transferred to the oldest
// remaining active member within the same transaction as the leave operation.
func (s *groupMembershipService) Leave(ctx context.Context, quinielaID, callerUserID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerUserID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Validation("you are not an active member of this group")
	}

	minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
	if m.Role == domain.MembershipRoleCreateOwner {
		successor, err := s.memberRepo.OldestActiveMember(ctx, quinielaID, callerUserID)
		if err != nil {
			return err
		}
		if successor != nil {
			return s.memberRepo.LeaveMembershipAndTransferOwnership(
				ctx,
				quinielaID,
				callerUserID,
				successor.ID,
				s.clock.Now(),
				minMembers,
			)
		}
	}

	return s.memberRepo.LeaveMembership(ctx, quinielaID, callerUserID, s.clock.Now(), minMembers)
}

// MarkPaid flips the paid flag to true for the given membership. It is
// intended to be called by the payment system after a successful transaction
// - never from an HTTP handler directly.
func (s *groupMembershipService) MarkPaid(ctx context.Context, quinielaID, userID int) (*domain.GroupMembership, error) {
	m, err := s.memberRepo.MarkPaid(ctx, quinielaID, userID)
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *groupMembershipService) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.GroupMembership, error) {
	members, err := s.memberRepo.ListByQuiniela(ctx, quinielaID)
	if err != nil {
		return nil, err
	}
	return members, nil
}

func (s *groupMembershipService) ListByUser(ctx context.Context, userID int) ([]*domain.GroupMembership, error) {
	return s.memberRepo.ListByUser(ctx, userID)
}

// enforce compile-time interface satisfaction.
var _ GroupMembershipService = (*groupMembershipService)(nil)
