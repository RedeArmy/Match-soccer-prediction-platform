package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

const (
	adminUserUnexpectedErr = "unexpected error: %v"
	adminUserNotFoundErr   = "not found"
	adminUserDBError       = "db error"
	adminUserBanReason     = "cheating"
)

func newAdminUserSvc(ur *stubUserRepo, mr *stubMemberRepo) AdminUserService {
	return NewAdminUserService(ur, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())
}

// ── BanUser ───────────────────────────────────────────────────────────────────

func TestAdminUserService_BanUser_HappyPath_ReturnsBannedUser(t *testing.T) {
	banned := &domain.User{ID: 5}
	svc := newAdminUserSvc(&stubUserRepo{user: banned}, &stubMemberRepo{memberships: nil})

	got, err := svc.BanUser(context.Background(), 5, 99, adminUserBanReason)
	if err != nil || got == nil {
		t.Fatalf("expected user, got %v err=%v", got, err)
	}
}

func TestAdminUserService_BanUser_RepoError_Propagates(t *testing.T) {
	svc := newAdminUserSvc(&stubUserRepo{err: errors.New(adminUserNotFoundErr)}, &stubMemberRepo{})

	_, err := svc.BanUser(context.Background(), 5, 99, "")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAdminUserService_BanUser_TransfersOwnedGroups(t *testing.T) {
	// The banned user is CreateOwner of quiniela 1; there is a successor member.
	ownerMembership := &domain.GroupMembership{
		ID: 10, QuinielaID: 1, UserID: 5,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleCreateOwner,
	}
	successor := &domain.GroupMembership{
		ID: 20, QuinielaID: 1, UserID: 6,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleMember,
	}
	mr := &transferTestMemberRepo{
		memberships: []*domain.GroupMembership{ownerMembership},
		oldest:      successor,
	}
	svc := NewAdminUserService(&stubUserRepo{user: &domain.User{ID: 5}}, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.BanUser(context.Background(), 5, 99, adminUserBanReason)
	if err != nil {
		t.Fatalf(adminUserUnexpectedErr, err)
	}
	if mr.setRoleCalled != successor.ID {
		t.Errorf("expected SetRole on membership %d, got %d", successor.ID, mr.setRoleCalled)
	}
}

func TestAdminUserService_BanUser_NoOwnedGroups_TransferIsNoop(t *testing.T) {
	// User has no CreateOwner memberships — transfer should be a no-op.
	regularMembership := &domain.GroupMembership{
		ID: 10, QuinielaID: 1, UserID: 5,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleMember,
	}
	mr := &transferTestMemberRepo{memberships: []*domain.GroupMembership{regularMembership}}
	svc := NewAdminUserService(&stubUserRepo{user: &domain.User{ID: 5}}, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.BanUser(context.Background(), 5, 99, "")
	if err != nil {
		t.Fatalf(adminUserUnexpectedErr, err)
	}
	if mr.setRoleCalled != 0 {
		t.Errorf("expected no SetRole call, but got membership ID %d", mr.setRoleCalled)
	}
}

func TestAdminUserService_BanUser_NoSuccessor_TransferIsNoop(t *testing.T) {
	ownerMembership := &domain.GroupMembership{
		ID: 10, QuinielaID: 1, UserID: 5,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleCreateOwner,
	}
	mr := &transferTestMemberRepo{
		memberships: []*domain.GroupMembership{ownerMembership},
		oldest:      nil, // no successor
	}
	svc := NewAdminUserService(&stubUserRepo{user: &domain.User{ID: 5}}, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.BanUser(context.Background(), 5, 99, "")
	if err != nil {
		t.Fatalf(adminUserUnexpectedErr, err)
	}
	if mr.setRoleCalled != 0 {
		t.Errorf("expected no SetRole call when no successor, got %d", mr.setRoleCalled)
	}
}

// ── UnbanUser ─────────────────────────────────────────────────────────────────

func TestAdminUserService_UnbanUser_HappyPath_ReturnsUser(t *testing.T) {
	u := &domain.User{ID: 5}
	svc := newAdminUserSvc(&stubUserRepo{user: u}, &stubMemberRepo{})

	got, err := svc.UnbanUser(context.Background(), 5, 99)
	if err != nil || got == nil {
		t.Fatalf("expected user, got %v err=%v", got, err)
	}
}

func TestAdminUserService_UnbanUser_RepoError_Propagates(t *testing.T) {
	svc := newAdminUserSvc(&stubUserRepo{err: errors.New(adminUserNotFoundErr)}, &stubMemberRepo{})

	_, err := svc.UnbanUser(context.Background(), 5, 99)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── ListUsers ─────────────────────────────────────────────────────────────────

func TestAdminUserService_ListUsers_ReturnsAllUsers(t *testing.T) {
	users := []*domain.User{{ID: 1}, {ID: 2}}
	svc := newAdminUserSvc(&stubUserRepo{users: users}, &stubMemberRepo{})

	got, err := svc.ListUsers(context.Background())
	if err != nil || len(got) != 2 {
		t.Errorf("expected 2 users, got %v err=%v", got, err)
	}
}

func TestAdminUserService_UnbanUser_GetByIDError_Propagates(t *testing.T) {
	svc := NewAdminUserService(&unbanGetByIDErrRepo{}, &stubMemberRepo{}, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.UnbanUser(context.Background(), 5, 99)
	if err == nil {
		t.Fatal("expected error from GetByID after Unban, got nil")
	}
}

func TestAdminUserService_BanUser_TransferOwnershipListUserError_BanStillSucceeds(t *testing.T) {
	// ListByUser returns an error; transferOwnedGroups error is swallowed by BanUser.
	mr := &listByUserErrMemberRepo{}
	svc := NewAdminUserService(&stubUserRepo{user: &domain.User{ID: 5}}, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.BanUser(context.Background(), 5, 99, "")
	if err != nil {
		t.Fatalf("BanUser must succeed even when transferOwnedGroups fails: %v", err)
	}
}

func TestAdminUserService_BanUser_DoTransferSetRoleError_BanStillSucceeds(t *testing.T) {
	ownerMembership := &domain.GroupMembership{
		ID: 10, QuinielaID: 1, UserID: 5,
		Status: domain.MembershipActive, Role: domain.MembershipRoleCreateOwner,
	}
	successor := &domain.GroupMembership{ID: 20, QuinielaID: 1, UserID: 6, Status: domain.MembershipActive}
	mr := &setRoleErrMemberRepo{
		memberships: []*domain.GroupMembership{ownerMembership},
		oldest:      successor,
	}
	svc := NewAdminUserService(&stubUserRepo{user: &domain.User{ID: 5}}, mr, &stubPaymentRepo{}, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.BanUser(context.Background(), 5, 99, "")
	if err != nil {
		t.Fatalf("BanUser must succeed even when SetRole fails: %v", err)
	}
}

// unbanGetByIDErrRepo succeeds on Unban but fails on GetByID.
type unbanGetByIDErrRepo struct {
	stubUserRepo
}

func (r *unbanGetByIDErrRepo) Unban(_ context.Context, _ int) error { return nil }
func (r *unbanGetByIDErrRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, errors.New(adminUserDBError)
}

// listByUserErrMemberRepo fails on ListByUser so transferOwnedGroups returns an error.
type listByUserErrMemberRepo struct {
	stubMemberRepo
}

func (r *listByUserErrMemberRepo) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return nil, errors.New(adminUserDBError)
}

// setRoleErrMemberRepo returns an error from SetRole to test the doTransfer error path.
type setRoleErrMemberRepo struct {
	stubMemberRepo
	memberships []*domain.GroupMembership
	oldest      *domain.GroupMembership
}

func (r *setRoleErrMemberRepo) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return r.memberships, nil
}
func (r *setRoleErrMemberRepo) OldestActiveMember(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.oldest, nil
}
func (r *setRoleErrMemberRepo) SetRole(_ context.Context, _ int, _ domain.MembershipRole) error {
	return errors.New(adminUserDBError)
}

// ── BulkBan ───────────────────────────────────────────────────────────────────

func TestAdminUserService_BulkBan_AllSucceed_ReturnsNil(t *testing.T) {
	u := &domain.User{ID: 1}
	svc := newAdminUserSvc(&stubUserRepo{user: u}, &stubMemberRepo{})

	err := svc.BulkBan(context.Background(), []int{1, 2, 3}, 99, adminUserBanReason)
	if err != nil {
		t.Errorf("expected nil error, got %v", err)
	}
}

func TestAdminUserService_BulkBan_PartialFailure_ReturnsFirstError(t *testing.T) {
	svc := newAdminUserSvc(&stubUserRepo{err: errors.New(adminUserNotFoundErr)}, &stubMemberRepo{})

	err := svc.BulkBan(context.Background(), []int{1, 2}, 99, "")
	if err == nil {
		t.Error("expected error for all-fail bulk ban, got nil")
	}
}

// ── transferTestMemberRepo ────────────────────────────────────────────────────

// transferTestMemberRepo captures SetRole calls and controls OldestActiveMember.
type transferTestMemberRepo struct {
	stubMemberRepo
	memberships   []*domain.GroupMembership
	oldest        *domain.GroupMembership
	setRoleCalled int
}

func (r *transferTestMemberRepo) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return r.memberships, nil
}
func (r *transferTestMemberRepo) OldestActiveMember(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.oldest, nil
}
func (r *transferTestMemberRepo) SetRole(_ context.Context, membershipID int, _ domain.MembershipRole) error {
	r.setRoleCalled = membershipID
	return nil
}
