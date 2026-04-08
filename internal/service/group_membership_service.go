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

	// Check if max_members cap would be exceeded.
	if quiniela.MaxMembers != nil {
		existing, err := s.memberRepo.ListByQuiniela(ctx, quiniela.ID)
		if err != nil {
			return nil, err
		}
		active := 0
		for _, m := range existing {
			if m.Status == domain.MembershipActive {
				active++
			}
		}
		if active >= *quiniela.MaxMembers {
			return nil, apperrors.Conflict("this group has reached its maximum number of members")
		}
	}

	existing, err := s.memberRepo.GetByQuinielaAndUser(ctx, quiniela.ID, userID)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()

	// paid is true for free groups; for paid groups the payment system
	// will call MarkPaid after the transaction is confirmed.
	autoPayd := quiniela.EntryFee == 0

	if existing != nil {
		switch existing.Status {
		case domain.MembershipActive:
			return nil, apperrors.Conflict("you are already a member of this group")
		case domain.MembershipLeft, domain.MembershipPending:
			existing.Status = domain.MembershipActive
			existing.Paid = autoPayd
			existing.JoinedAt = &now
			if err := s.memberRepo.Update(ctx, existing); err != nil {
				return nil, err
			}
			return existing, nil
		}
	}

	m := &domain.GroupMembership{
		QuinielaID: quiniela.ID,
		UserID:     userID,
		Status:     domain.MembershipActive,
		Paid:       autoPayd,
		JoinedAt:   &now,
	}
	if err := s.memberRepo.Create(ctx, m); err != nil {
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
