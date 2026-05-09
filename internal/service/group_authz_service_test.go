package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── GroupAuthzService ─────────────────────────────────────────────────────────

func TestGroupAuthzService_RequireOwner_OwnerSucceeds(t *testing.T) {
	m := &domain.GroupMembership{Role: domain.MembershipRoleCreateOwner}
	svc := NewGroupAuthzService(&stubMemberRepo{membership: m})

	if err := svc.RequireOwner(context.Background(), 1, 10); err != nil {
		t.Errorf("expected nil for owner caller, got %v", err)
	}
}

func TestGroupAuthzService_RequireOwner_NonOwnerReturnsForbidden(t *testing.T) {
	m := &domain.GroupMembership{Role: domain.MembershipRoleMember, Status: domain.MembershipActive}
	svc := NewGroupAuthzService(&stubMemberRepo{membership: m})

	err := svc.RequireOwner(context.Background(), 1, 99)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-owner, got %v", err)
	}
}

func TestGroupAuthzService_RequireOwner_NoMembershipReturnsForbidden(t *testing.T) {
	svc := NewGroupAuthzService(&stubMemberRepo{membership: nil})

	err := svc.RequireOwner(context.Background(), 1, 99)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for absent membership, got %v", err)
	}
}

func TestGroupAuthzService_RequireOwner_RepoErrorPropagates(t *testing.T) {
	dbErr := errors.New("connection reset")
	svc := NewGroupAuthzService(&stubMemberRepo{err: dbErr})

	if err := svc.RequireOwner(context.Background(), 1, 10); !errors.Is(err, dbErr) {
		t.Errorf("expected repo error to propagate, got %v", err)
	}
}

func TestGroupAuthzService_RequireActiveMember_ActiveMemberSucceeds(t *testing.T) {
	m := &domain.GroupMembership{Status: domain.MembershipActive}
	svc := NewGroupAuthzService(&stubMemberRepo{membership: m})

	if err := svc.RequireActiveMember(context.Background(), 1, 42); err != nil {
		t.Errorf("expected nil for active member, got %v", err)
	}
}

func TestGroupAuthzService_RequireActiveMember_PendingReturnsForbidden(t *testing.T) {
	m := &domain.GroupMembership{Status: domain.MembershipPending}
	svc := NewGroupAuthzService(&stubMemberRepo{membership: m})

	err := svc.RequireActiveMember(context.Background(), 1, 42)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for pending member, got %v", err)
	}
}

func TestGroupAuthzService_RequireActiveMember_NoMembershipReturnsForbidden(t *testing.T) {
	svc := NewGroupAuthzService(&stubMemberRepo{membership: nil})

	err := svc.RequireActiveMember(context.Background(), 1, 99)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for absent membership, got %v", err)
	}
}

func TestGroupAuthzService_RequireActiveMember_RepoErrorPropagates(t *testing.T) {
	dbErr := errors.New("connection reset")
	svc := NewGroupAuthzService(&stubMemberRepo{err: dbErr})

	if err := svc.RequireActiveMember(context.Background(), 1, 42); !errors.Is(err, dbErr) {
		t.Errorf("expected repo error to propagate, got %v", err)
	}
}
