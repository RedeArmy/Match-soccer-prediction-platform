package service

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// GroupAuthz is a focused permission service for group-scoped membership checks.
// It centralises the two recurring authorisation patterns shared across
// QuinielaService, GroupMembershipService, and TiebreakerService so that the
// rule is defined once and the error message never drifts between callers.
type GroupAuthz interface {
	// RequireOwner returns Forbidden when callerID does not hold
	// MembershipRoleCreateOwner in quinielaID. Returns the repository error
	// unchanged when the membership lookup itself fails.
	RequireOwner(ctx context.Context, quinielaID, callerID int) error
	// RequireActiveMember returns Forbidden when callerID is not an active
	// member of quinielaID. Returns the repository error unchanged when the
	// membership lookup itself fails.
	RequireActiveMember(ctx context.Context, quinielaID, callerID int) error
}

// groupAuthzService implements GroupAuthz backed by GroupMembershipRepository.
type groupAuthzService struct {
	memberRepo repository.GroupMembershipRepository
}

// NewGroupAuthzService constructs a GroupAuthz backed by memberRepo.
func NewGroupAuthzService(memberRepo repository.GroupMembershipRepository) GroupAuthz {
	return &groupAuthzService{memberRepo: memberRepo}
}

func (s *groupAuthzService) RequireOwner(ctx context.Context, quinielaID, callerID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerID)
	if err != nil {
		return err
	}
	if m == nil || m.Role != domain.MembershipRoleCreateOwner {
		return apperrors.Forbidden("only the group owner can perform this action")
	}
	return nil
}

func (s *groupAuthzService) RequireActiveMember(ctx context.Context, quinielaID, callerID int) error {
	m, err := s.memberRepo.GetByQuinielaAndUser(ctx, quinielaID, callerID)
	if err != nil {
		return err
	}
	if m == nil || m.Status != domain.MembershipActive {
		return apperrors.Forbidden("caller is not an active member of this group")
	}
	return nil
}

var _ GroupAuthz = (*groupAuthzService)(nil)
