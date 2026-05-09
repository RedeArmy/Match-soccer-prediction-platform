package service

import (
	"context"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

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
