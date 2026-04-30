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
	params SystemParamService,
	audit AuditLogger,
	paymentSvc PaymentService,
	clk clock.Nower,
	log *zap.Logger,
) GroupMembershipService {
	return &groupMembershipService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
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
	quiniela, err := s.quinielaRepo.GetByInviteCode(ctx, inviteCode)
	if err != nil {
		return nil, err
	}
	if quiniela == nil {
		return nil, apperrors.NotFound("group not found for the given invite code")
	}

	if err := s.checkCapacity(ctx, quiniela); err != nil {
		return nil, err
	}

	existing, err := s.memberRepo.GetByQuinielaAndUser(ctx, quiniela.ID, userID)
	if err != nil {
		return nil, err
	}

	// paid is set at request time: free groups auto-pay immediately;
	// paid groups wait for the payment system to call MarkPaid.
	autoPaid := quiniela.EntryFee == 0

	if existing != nil {
		m, err := s.requestRejoin(ctx, existing, autoPaid)
		if err != nil {
			return nil, err
		}
		if quiniela.EntryFee > 0 {
			s.createPendingPayment(ctx, quiniela, userID)
		}
		return m, nil
	}

	m := &domain.GroupMembership{
		QuinielaID: quiniela.ID,
		UserID:     userID,
		Status:     domain.MembershipPending,
		Paid:       autoPaid,
		JoinedAt:   nil, // populated when the request is approved
	}
	if err := s.memberRepo.Create(ctx, m); err != nil {
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

// checkCapacity returns a Conflict error when the quiniela has a max_members
// cap and the number of active members has reached it.
//
// This is a pre-flight optimisation only: the gap between this read and the
// subsequent INSERT is not serialised, so two concurrent joins can both pass
// the check before either write commits. The database trigger
// trg_enforce_max_members (SQLSTATE P0001 / max_members_exceeded) is the
// authoritative enforcement point and is translated into a Conflict error by
// the repository layer on violation.
func (s *groupMembershipService) checkCapacity(ctx context.Context, quiniela *domain.Quiniela) error {
	if quiniela.MaxMembers == nil {
		return nil
	}
	count, err := s.memberRepo.CountActive(ctx, quiniela.ID)
	if err != nil {
		return err
	}
	if count >= *quiniela.MaxMembers {
		return apperrors.Conflict("this group has reached its maximum number of members")
	}
	return nil
}

// requestRejoin handles the case where a membership row already exists.
// Active and Pending states return Conflict; a Left membership is re-queued
// as Pending so the user must be approved again before becoming active.
func (s *groupMembershipService) requestRejoin(ctx context.Context, m *domain.GroupMembership, autoPaid bool) (*domain.GroupMembership, error) {
	switch m.Status {
	case domain.MembershipActive:
		return nil, apperrors.Conflict("you are already a member of this group")
	case domain.MembershipPending:
		return nil, apperrors.Conflict("you already have a pending join request for this group")
	default: // MembershipLeft
		m.Status = domain.MembershipPending
		m.Paid = autoPaid
		m.JoinedAt = nil  // reset; will be set when approved
		m.RemovedAt = nil // clear audit fields from the previous exit
		m.RemovedBy = nil
		if err := s.memberRepo.Update(ctx, m); err != nil {
			return nil, err
		}
		return m, nil
	}
}

// ApproveJoin promotes a pending membership to active. The approverUserID must
// be an active member of the same quiniela — any member may approve; there is
// no admin-only gate. The membership update and group status recalculation are
// committed atomically via ApproveMembership.
func (s *groupMembershipService) ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error) {
	approver, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, approverUserID)
	if err != nil {
		return nil, err
	}
	if approver == nil || approver.Status != domain.MembershipActive {
		return nil, apperrors.Forbidden("only active members of this group may approve join requests")
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
	m, err := s.memberRepo.ApproveMembership(ctx, membershipID, quinielaID, s.clock.Now(), minMembers)
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
// may call this — no admin or owner can remove another user. If the leaving
// user holds MembershipRoleCreateOwner, ownership is automatically transferred
// to the oldest remaining active member before the status is updated. The
// membership update and group status recalculation are committed atomically
// via LeaveMembership.
func (s *groupMembershipService) Leave(ctx context.Context, quinielaID, callerUserID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerUserID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Validation("you are not an active member of this group")
	}

	if m.Role == domain.MembershipRoleCreateOwner {
		if terr := s.transferOwnership(ctx, quinielaID, callerUserID); terr != nil {
			s.log.Warn("membership: ownership transfer on leave failed",
				zap.Int("quiniela_id", quinielaID),
				zap.Int("leaving_user", callerUserID),
				zap.Error(terr))
		}
	}

	minMembers := s.params.GetInt(ctx, domain.ParamKeyGroupMinMembers, domain.MinMembersForActive)
	return s.memberRepo.LeaveMembership(ctx, quinielaID, callerUserID, s.clock.Now(), minMembers)
}

// transferOwnership assigns MembershipRoleCreateOwner to the oldest active
// member of quinielaID, excluding excludeUserID. A no-op when no eligible
// successor exists.
func (s *groupMembershipService) transferOwnership(ctx context.Context, quinielaID, excludeUserID int) error {
	successor, err := s.memberRepo.OldestActiveMember(ctx, quinielaID, excludeUserID)
	if err != nil {
		return err
	}
	if successor == nil {
		return nil
	}
	return s.memberRepo.SetRole(ctx, successor.ID, domain.MembershipRoleCreateOwner)
}

// MarkPaid flips the paid flag to true for the given membership. It is
// intended to be called by the payment system after a successful transaction
// — never from an HTTP handler directly.
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
