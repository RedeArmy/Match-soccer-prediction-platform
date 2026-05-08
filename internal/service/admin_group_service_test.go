package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	adminGroupNilErrFmt    = "expected nil error, got %v"
	adminGroupExpectErrMsg = "expected error, got nil"
)

// noopSnapshotter implements Snapshotter for tests where snapshot content is irrelevant.
type noopSnapshotter struct{}

func (s *noopSnapshotter) Snapshot(_ context.Context, quinielaID int) (*domain.LeaderboardSnapshot, error) {
	return &domain.LeaderboardSnapshot{QuinielaID: quinielaID}, nil
}

func newAdminGroupSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) AdminGroupService {
	return NewAdminGroupService(qr, mr, &noopSnapshotter{}, &noopAuditLogger{}, zap.NewNop())
}

// ── DeleteGroup ───────────────────────────────────────────────────────────────

func TestAdminGroupService_DeleteGroup_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
	}
}

func TestAdminGroupService_DeleteGroup_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	if err := svc.DeleteGroup(context.Background(), 1, 99); err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── RemoveMember ──────────────────────────────────────────────────────────────

func TestAdminGroupService_RemoveMember_HappyPath_ReturnsNil(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if err := svc.RemoveMember(context.Background(), 10, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
	}
}

func TestAdminGroupService_RemoveMember_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New("inactive")})

	if err := svc.RemoveMember(context.Background(), 10, 99); err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── UpdateGroupSettings ───────────────────────────────────────────────────────

func TestAdminGroupService_UpdateGroupSettings_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test", EntryFee: 500}
	svc := newAdminGroupSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.UpdateGroupSettings(context.Background(), 1, 500, 99)
	if err != nil || got == nil {
		t.Fatalf("expected quiniela, got %v err=%v", got, err)
	}
}

func TestAdminGroupService_UpdateGroupSettings_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("not found")}, &stubMemberRepo{})

	_, err := svc.UpdateGroupSettings(context.Background(), 1, 0, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── TransferOwnership ─────────────────────────────────────────────────────────

func TestAdminGroupService_TransferOwnership_HappyPath_DemotesAndPromotes(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive, Role: domain.MembershipRoleMember}
	mr := &stubMemberRepo{membership: newOwner}
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, mr)

	if err := svc.TransferOwnership(context.Background(), 1, 2, 99); err != nil {
		t.Errorf(adminGroupNilErrFmt, err)
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

func TestAdminGroupService_TransferOwnership_TransferRolesError_Propagates(t *testing.T) {
	newOwner := &domain.GroupMembership{ID: 20, UserID: 2, Status: domain.MembershipActive}
	svc := NewAdminGroupService(&stubQuinielaRepo{}, &errOnTransferOwnershipRepo{newOwner: newOwner}, &noopSnapshotter{}, &noopAuditLogger{}, zap.NewNop())

	err := svc.TransferOwnership(context.Background(), 1, 2, 99)
	if err == nil {
		t.Error("expected error from TransferOwnershipRoles, got nil")
	}
}

// ── BulkDeleteGroups ──────────────────────────────────────────────────────────

func TestAdminGroupService_BulkDeleteGroups_AllSucceed_ReturnsAllInSucceeded(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	result, err := svc.BulkDeleteGroups(context.Background(), []int{1, 2, 3}, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if len(result.Succeeded) != 3 {
		t.Errorf("expected 3 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestAdminGroupService_BulkDeleteGroups_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{err: errors.New("db error")}, &stubMemberRepo{})

	_, err := svc.BulkDeleteGroups(context.Background(), []int{1, 2}, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── BulkRemoveMembers ─────────────────────────────────────────────────────────

func TestAdminGroupService_BulkRemoveMembers_AllSucceed_ReturnsAllInSucceeded(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	result, err := svc.BulkRemoveMembers(context.Background(), 1, []int{10, 11}, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if len(result.Succeeded) != 2 {
		t.Errorf("expected 2 succeeded, got %d", len(result.Succeeded))
	}
	if len(result.Failed) != 0 {
		t.Errorf("expected 0 failed, got %d", len(result.Failed))
	}
}

func TestAdminGroupService_BulkRemoveMembers_RepoError_Propagates(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New("db error")})

	_, err := svc.BulkRemoveMembers(context.Background(), 1, []int{10}, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// ── RecalculateLeaderboard ────────────────────────────────────────────────────

func TestAdminGroupService_RecalculateLeaderboard_HappyPath_ReturnsSnapshot(t *testing.T) {
	svc := newAdminGroupSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	snap, err := svc.RecalculateLeaderboard(context.Background(), 5, 99)
	if err != nil {
		t.Fatalf(adminGroupNilErrFmt, err)
	}
	if snap == nil || snap.QuinielaID != 5 {
		t.Errorf("expected snapshot with QuinielaID=5, got %v", snap)
	}
}

func TestAdminGroupService_RecalculateLeaderboard_SnapshotterError_Propagates(t *testing.T) {
	errSnap := &errSnapshotter{}
	svc := NewAdminGroupService(&stubQuinielaRepo{}, &stubMemberRepo{}, errSnap, &noopAuditLogger{}, zap.NewNop())

	_, err := svc.RecalculateLeaderboard(context.Background(), 5, 99)
	if err == nil {
		t.Error(adminGroupExpectErrMsg)
	}
}

// errSnapshotter always fails Snapshot.
type errSnapshotter struct{}

func (*errSnapshotter) Snapshot(_ context.Context, _ int) (*domain.LeaderboardSnapshot, error) {
	return nil, errors.New("snapshotter error")
}

// errOnTransferOwnershipRepo returns the newOwner from GetByQuinielaAndUser but
// fails TransferOwnershipRoles, simulating a transaction error mid-transfer.
type errOnTransferOwnershipRepo struct {
	stubMemberRepo
	newOwner *domain.GroupMembership
}

func (r *errOnTransferOwnershipRepo) GetByQuinielaAndUser(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.newOwner, nil
}
func (r *errOnTransferOwnershipRepo) TransferOwnershipRoles(_ context.Context, _, _ int) error {
	return errors.New("db error")
}
