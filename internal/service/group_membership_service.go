package service

import (
	"context"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// groupMembershipService is the concrete implementation of GroupMembershipService.
type groupMembershipService struct {
	quinielaRepo repository.QuinielaRepository
	memberRepo   repository.GroupMembershipRepository
}

// NewGroupMembershipService constructs a groupMembershipService.
func NewGroupMembershipService(
	quinielaRepo repository.QuinielaRepository,
	memberRepo repository.GroupMembershipRepository,
) GroupMembershipService {
	return &groupMembershipService{quinielaRepo: quinielaRepo, memberRepo: memberRepo}
}

// Join resolves invite_code to a Quiniela and adds the user as an active
// member. If the user previously left the group, their membership is
// re-activated. Joining a group the user is already active in returns a
// Conflict error so the caller can surface a meaningful message.
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

	now := time.Now().UTC()
	// paid is true for free groups; for paid groups the payment system
	// will call MarkPaid after the transaction is confirmed.
	autoPaid := quiniela.EntryFee == 0

	if existing != nil {
		return s.reactivate(ctx, existing, autoPaid, now)
	}

	m := &domain.GroupMembership{
		QuinielaID: quiniela.ID,
		UserID:     userID,
		Status:     domain.MembershipActive,
		Paid:       autoPaid,
		JoinedAt:   &now,
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
	members, err := s.memberRepo.ListByQuiniela(ctx, quiniela.ID)
	if err != nil {
		return err
	}
	active := 0
	for _, m := range members {
		if m.Status == domain.MembershipActive {
			active++
		}
	}
	if active >= *quiniela.MaxMembers {
		return apperrors.Conflict("this group has reached its maximum number of members")
	}
	return nil
}

// reactivate handles the case where a membership row already exists.
// Active memberships return Conflict; left or pending memberships are
// promoted to active.
func (s *groupMembershipService) reactivate(ctx context.Context, m *domain.GroupMembership, autoPaid bool, now time.Time) (*domain.GroupMembership, error) {
	if m.Status == domain.MembershipActive {
		return nil, apperrors.Conflict("you are already a member of this group")
	}
	m.Status = domain.MembershipActive
	m.Paid = autoPaid
	m.JoinedAt = &now
	if err := s.memberRepo.Update(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
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
