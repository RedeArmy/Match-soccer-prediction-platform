package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

func newAdminGroupSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) AdminGroupService {
	return NewAdminGroupService(qr, mr, &noopAuditLogger{}, zap.NewNop())
}

// ── DeleteGroup ───────────────────────────────────────────────────────────────

func TestAdminGroupService_DeleteGroup_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAdminGroupService_DeleteGroup_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err == nil {
		t.Error("expected error, got nil")
	}
}

// ── RemoveMember ──────────────────────────────────────────────────────────────

func TestAdminGroupService_RemoveMember_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.RemoveMember(context.Background(), 10, 99); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAdminGroupService_RemoveMember_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New("inactive")})

	if err := svc.RemoveMember(context.Background(), 10, 99); err == nil {
		t.Error("expected error, got nil")
	}
}

// ── UpdateGroupSettings ───────────────────────────────────────────────────────

func TestAdminGroupService_UpdateGroupSettings_WithMaxMembers_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test"}
	svc := newAdminGroupSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	cap := 10
	got, err := svc.UpdateGroupSettings(context.Background(), 1, &cap, 0, 99)
	if err != nil || got == nil {
		t.Fatalf("expected quiniela, got %v err=%v", got, err)
	}
}

func TestAdminGroupService_UpdateGroupSettings_NilMaxMembers_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1}
	svc := newAdminGroupSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.UpdateGroupSettings(context.Background(), 1, nil, 500, 99)
	if err != nil || got == nil {
		t.Fatalf("expected quiniela, got %v err=%v", got, err)
	}
}

func TestAdminGroupService_UpdateGroupSettings_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	_, err := svc.UpdateGroupSettings(context.Background(), 1, nil, 0, 99)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// ── TransferOwnership ─────────────────────────────────────────────────────────

func TestAdminGroupService_TransferOwnership_HappyPath_DemotesAndPromotes(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive, Role: domain.MembershipRoleMember}
	currentOwner := &domain.GroupMembership{ID: 10, UserID: 1, Status: domain.MembershipActive, Role: domain.MembershipRoleCreateOwner}

	mr := &stubMemberRepo{
		membership:  newOwner,
		memberships: []*domain.GroupMembership{currentOwner, newOwner},
	}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	if err := svc.TransferOwnership(context.Background(), 1, 2, 99); err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAdminGroupService_TransferOwnership_NewOwnerNotMember_ReturnsNotFound(t *testing.T) {
	mr := &stubMemberRepo{membership: nil}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdminGroupService_TransferOwnership_NewOwnerInactive_ReturnsNotFound(t *testing.T) {
	inactive := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipLeft}
	mr := &stubMemberRepo{membership: inactive}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestAdminGroupService_TransferOwnership_ListError_Propagates(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive}
	svc := NewAdminGroupService(&stubQuinielaRepo{}, &errOnListMemberRepo{newOwner: newOwner}, &noopAuditLogger{}, zap.NewNop())

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if err == nil {
		t.Error("expected error from ListByQuiniela, got nil")
	}
}

// errOnListMemberRepo returns the newOwner from GetByQuinielaAndUser but fails ListByQuiniela.
type errOnListMemberRepo struct {
	stubMemberRepo
	newOwner *domain.GroupMembership
}

func (r *errOnListMemberRepo) GetByQuinielaAndUser(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.newOwner, nil
}
func (r *errOnListMemberRepo) ListByQuiniela(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return nil, errors.New("db error")
}
