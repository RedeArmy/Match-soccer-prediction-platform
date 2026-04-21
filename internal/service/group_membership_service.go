package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// groupMembershipService is the concrete implementation of GroupMembershipService.
type groupMembershipService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
	log          *zap.Logger
}

// NewGroupMembershipService constructs a groupMembershipService.
func NewGroupMembershipService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
	log *zap.Logger,
) GroupMembershipService {
	return &groupMembershipService{
		quinielaRepo: quinielaRepo,
		memberRepo:   memberRepo,
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
		return s.requestRejoin(ctx, existing, autoPaid)
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
	return m, nil
}

// checkCapacity returns a Conflict error when the quiniela has a max_members
// cap and the number of active members has reached it.
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
		m.JoinedAt = nil // reset; will be set when approved
		if err := s.memberRepo.Update(ctx, m); err != nil {
			return nil, err
		}
		return m, nil
	}
}

// ApproveJoin promotes a pending membership to active. The approverUserID must
// be an active member of the same quiniela — any member may approve; there is
// no admin-only gate. After approval, group status is synchronised via
// syncGroupStatus.
func (s *groupMembershipService) ApproveJoin(ctx context.Context, quinielaID, membershipID, approverUserID int) (*domain.GroupMembership, error) {
	// Verify the approver is an active member of this quiniela.
	approver, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, approverUserID)
	if err != nil {
		return nil, err
	}
	if approver == nil || approver.Status != domain.MembershipActive {
		return nil, apperrors.Forbidden("only active members of this group may approve join requests")
	}

	// Load and validate the pending request.
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

	now := time.Now().UTC()
	pending.Status = domain.MembershipActive
	pending.JoinedAt = &now
	if err := s.memberRepo.Update(ctx, pending); err != nil {
		return nil, err
	}

	s.syncGroupStatus(ctx, quinielaID)
	return pending, nil
}

// Leave sets the caller's own membership to left. Only the member themselves
// may call this — no admin or owner can remove another user. After leaving,
// group status is synchronised via syncGroupStatus.
func (s *groupMembershipService) Leave(ctx context.Context, quinielaID, callerUserID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerUserID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Validation("you are not an active member of this group")
	}

	m.Status = domain.MembershipLeft
	m.JoinedAt = nil
	if err := s.memberRepo.Update(ctx, m); err != nil {
		return err
	}

	s.syncGroupStatus(ctx, quinielaID)
	return nil
}

// syncGroupStatus recomputes whether the quiniela should be active or inactive
// based on the current active member count. It is called after every membership
// state transition. Errors are logged but not propagated — the membership change
// already succeeded, and the status will self-correct on the next transition.
func (s *groupMembershipService) syncGroupStatus(ctx context.Context, quinielaID int) {
	count, err := s.memberRepo.CountActive(ctx, quinielaID)
	if err != nil {
		s.log.Warn("group status sync: failed to count active members",
			zap.Int("quiniela_id", quinielaID), zap.Error(err))
		return
	}

	status := domain.QuinielaStatusInactive
	if count >= domain.MinMembersForActive {
		status = domain.QuinielaStatusActive
	}

	if err := s.quinielaRepo.UpdateStatus(ctx, quinielaID, status); err != nil {
		s.log.Warn("group status sync: failed to update quiniela status",
			zap.Int("quiniela_id", quinielaID),
			zap.String("status", string(status)),
			zap.Error(err))
	}
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
