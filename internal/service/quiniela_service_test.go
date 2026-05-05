package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/codegen"
)

const (
	quinielaTestName = "Test Quiniela"
	quinielaNewName  = "Renamed Quiniela"
	quinielaPool     = "Pool Quiniela"
)

// stubQuinielaRepo implements repository.QuinielaRepository with configurable returns.
// updateErr, if set, is returned exclusively by Update so that RenameGroup
// conflict paths can be tested independently of GetByID.
// updateStatusErr, if set, is returned exclusively by UpdateStatus so that
// syncGroupStatus error paths can be tested without affecting earlier calls.
type stubQuinielaRepo struct {
	quiniela        *domain.Quiniela
	quinielas       []*domain.Quiniela
	err             error
	updateErr       error
	updateStatusErr error
}

func (r *stubQuinielaRepo) CreateWithMembership(_ context.Context, _ *domain.Quiniela, _ *domain.GroupMembership) error {
	return r.err
}
func (r *stubQuinielaRepo) Create(_ context.Context, _ *domain.Quiniela) error { return r.err }
func (r *stubQuinielaRepo) GetByID(_ context.Context, _ int) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) Update(_ context.Context, _ *domain.Quiniela) error {
	if r.updateErr != nil {
		return r.updateErr
	}
	return r.err
}
func (r *stubQuinielaRepo) Delete(_ context.Context, _ int) error { return r.err }
func (r *stubQuinielaRepo) ListByOwner(_ context.Context, _ int) ([]*domain.Quiniela, error) {
	return r.quinielas, r.err
}
func (r *stubQuinielaRepo) RotateInviteCode(_ context.Context, _ int, _ string, _ *time.Time) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) UpdateStatus(_ context.Context, _ int, _ domain.QuinielaStatus) error {
	return r.updateStatusErr
}
func (r *stubQuinielaRepo) UpdateGroupSettings(_ context.Context, _ int, _ *int, _ int) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) DeleteByAdmin(_ context.Context, _, _ int) error { return r.err }
func (r *stubQuinielaRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.Quiniela, error) {
	return r.quinielas, r.err
}
func (r *stubQuinielaRepo) GetStatusCounts(_ context.Context) (repository.QuinielaStatusCounts, error) {
	return repository.QuinielaStatusCounts{}, r.err
}
func (r *stubQuinielaRepo) BulkDeleteByAdmin(_ context.Context, ids []int, _ int) ([]int, error) {
	if r.err != nil {
		return nil, r.err
	}
	return ids, nil
}

// stubMemberRepo implements repository.GroupMembershipRepository for service tests.
// membershipByID is returned by GetByID (used in ApproveJoin to load the pending
// request). membership is returned by GetByQuinielaAndUser (used to look up the
// approver and in Leave). activeCount is returned by CountActive (checkCapacity).
// approveErr, if set, is returned by ApproveMembership. leaveErr, if set, is
// returned by LeaveMembership. countActiveErr, if set, is returned exclusively
// by CountActive to test checkCapacity error paths without affecting other calls.
type stubMemberRepo struct {
	membership       *domain.GroupMembership
	membershipByID   *domain.GroupMembership
	memberships      []*domain.GroupMembership
	joinQuiniela     *domain.Quiniela
	joinMembership   *domain.GroupMembership
	joinErr          error
	activeCount      int
	err              error
	countActiveErr   error
	approveErr       error
	leaveErr         error
	leaveTransferErr error
}

func (r *stubMemberRepo) Create(_ context.Context, _ *domain.GroupMembership) error { return r.err }
func (r *stubMemberRepo) RequestJoinByInviteCode(_ context.Context, _ string, _ int) (*domain.Quiniela, *domain.GroupMembership, error) {
	if r.joinErr != nil {
		return nil, nil, r.joinErr
	}
	if r.joinQuiniela == nil {
		return nil, nil, apperrors.NotFound("group not found for the given invite code")
	}
	if r.joinMembership == nil {
		if r.membership != nil {
			switch r.membership.Status {
			case domain.MembershipActive:
				return nil, nil, apperrors.Conflict("you are already a member of this group")
			case domain.MembershipPending:
				return nil, nil, apperrors.Conflict("you already have a pending join request for this group")
			default:
				m := *r.membership
				m.Status = domain.MembershipPending
				m.Paid = r.joinQuiniela.EntryFee == 0
				m.JoinedAt = nil
				m.RemovedAt = nil
				m.RemovedBy = nil
				r.joinMembership = &m
			}
		} else {
			r.joinMembership = &domain.GroupMembership{
				QuinielaID: r.joinQuiniela.ID,
				Status:     domain.MembershipPending,
				Paid:       r.joinQuiniela.EntryFee == 0,
			}
		}
	}
	return r.joinQuiniela, r.joinMembership, nil
}
func (r *stubMemberRepo) GetByID(_ context.Context, _ int) (*domain.GroupMembership, error) {
	return r.membershipByID, r.err
}
func (r *stubMemberRepo) GetByQuinielaAndUser(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.membership, r.err
}
func (r *stubMemberRepo) Update(_ context.Context, _ *domain.GroupMembership) error { return r.err }
func (r *stubMemberRepo) MarkPaid(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.membership, r.err
}
func (r *stubMemberRepo) ListByQuiniela(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return r.memberships, r.err
}
func (r *stubMemberRepo) ListByUser(_ context.Context, _ int) ([]*domain.GroupMembership, error) {
	return r.memberships, r.err
}
func (r *stubMemberRepo) CountActive(_ context.Context, _ int) (int, error) {
	return r.activeCount, r.countActiveErr
}
func (r *stubMemberRepo) OldestActiveMember(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.membership, r.err
}
func (r *stubMemberRepo) SetRole(_ context.Context, _ int, _ domain.MembershipRole) error {
	return r.err
}
func (r *stubMemberRepo) RemoveByAdmin(_ context.Context, _, _ int) error { return r.err }
func (r *stubMemberRepo) ListGroupIDsWithoutOwner(_ context.Context) ([]int, error) {
	return nil, r.err
}
func (r *stubMemberRepo) ListStalePending(_ context.Context, _ time.Time) ([]*domain.GroupMembership, error) {
	return r.memberships, r.err
}
func (r *stubMemberRepo) BulkRemoveByAdmin(_ context.Context, _ int, ids []int, _ int) ([]int, error) {
	if r.err != nil {
		return nil, r.err
	}
	return ids, nil
}
func (r *stubMemberRepo) TransferOwnershipRoles(_ context.Context, _, _ int) error { return r.err }
func (r *stubMemberRepo) ApproveMembership(_ context.Context, _, _ int, now time.Time, _ int) (*domain.GroupMembership, error) {
	if r.approveErr != nil {
		return nil, r.approveErr
	}
	if r.membershipByID == nil {
		return nil, apperrors.Conflict("this join request is no longer pending")
	}
	m := *r.membershipByID
	m.Status = domain.MembershipActive
	m.JoinedAt = &now
	return &m, nil
}
func (r *stubMemberRepo) LeaveMembership(_ context.Context, _, _ int, _ time.Time, _ int) error {
	return r.leaveErr
}
func (r *stubMemberRepo) LeaveMembershipAndTransferOwnership(_ context.Context, _, _, _ int, _ time.Time, _ int) error {
	return r.leaveTransferErr
}

// ── QuinielaService tests ─────────────────────────────────────────────────────

func newQuinielaSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) QuinielaService {
	return NewQuinielaService(qr, mr, &noopSystemParamService{}, &noopAuditLogger{}, codegen.Fixed{Code: "AAAAAAAAAA"})
}

func TestQuinielaService_Create_ValidQuiniela_ReturnsNil(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaTestName, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestQuinielaService_Create_SetsInviteCode(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaTestName, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.InviteCode == "" {
		t.Error("expected InviteCode to be set after Create")
	}
	if len(q.InviteCode) != domain.DefaultGroupInviteCodeLength {
		t.Errorf("expected invite code length %d, got %d", domain.DefaultGroupInviteCodeLength, len(q.InviteCode))
	}
}

func TestQuinielaService_Create_InviteCodeNeverExpires(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaTestName, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.InviteCodeExpiresAt != nil {
		t.Errorf("expected InviteCodeExpiresAt to be nil (no expiry), got %v", q.InviteCodeExpiresAt)
	}
}

func TestQuinielaService_Create_InitialStatusIsInactive(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaTestName, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.Status != domain.QuinielaStatusInactive {
		t.Errorf("expected initial status %q, got %q", domain.QuinielaStatusInactive, q.Status)
	}
}

func TestQuinielaService_Create_EmptyName_ReturnsValidation(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{OwnerID: 1}

	if err := svc.Create(context.Background(), q); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for empty name, got %v", err)
	}
}

func TestQuinielaService_GetByID_Found_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test Pool", OwnerID: 2}
	svc := newQuinielaSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID 1, got %d", got.ID)
	}
}

func TestQuinielaService_GetByID_NotFound_ReturnsNotFound(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if _, err := svc.GetByID(context.Background(), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestQuinielaService_GetByOwner_ReturnsSlice(t *testing.T) {
	qs := []*domain.Quiniela{{ID: 1, Name: "Pool A", OwnerID: 1}}
	svc := newQuinielaSvc(&stubQuinielaRepo{quinielas: qs}, &stubMemberRepo{})

	got, err := svc.GetByOwner(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 quiniela, got %d", len(got))
	}
}

func TestQuinielaService_Create_DefaultsCurrencyToMXN(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaPool, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.Currency != "MXN" {
		t.Errorf("expected default currency MXN, got %q", q.Currency)
	}
}

func TestQuinielaService_Create_DefaultsPrizeThresholdWhenZero(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaPool, OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.PrizeThreshold != domain.DefaultPrizeThreshold {
		t.Errorf("expected default prize_threshold %d, got %d", domain.DefaultPrizeThreshold, q.PrizeThreshold)
	}
}

func TestQuinielaService_Create_ExplicitPrizeThreshold_Preserved(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: quinielaPool, OwnerID: 1, PrizeThreshold: 5}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.PrizeThreshold != 5 {
		t.Errorf("explicit prize_threshold should be preserved, got %d", q.PrizeThreshold)
	}
}

func TestQuinielaService_Create_RepoConflict_ReturnsConflict(t *testing.T) {
	svc := newQuinielaSvc(
		&stubQuinielaRepo{err: apperrors.Conflict("a group with this name already exists")},
		&stubMemberRepo{},
	)
	q := &domain.Quiniela{Name: "Duplicate", OwnerID: 1}

	if err := svc.Create(context.Background(), q); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestQuinielaService_GetByInviteCode_Found(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: quinielaPool, InviteCode: "ABC123"}
	svc := newQuinielaSvc(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.GetByInviteCode(context.Background(), "ABC123")
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID 1, got %d", got.ID)
	}
}

func TestQuinielaService_GetByInviteCode_NotFound_ReturnsNotFound(t *testing.T) {
	svc := newQuinielaSvc(&stubQuinielaRepo{}, &stubMemberRepo{})

	if _, err := svc.GetByInviteCode(context.Background(), "BADCODE"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── RenameGroup ───────────────────────────────────────────────────────────────

func TestQuinielaService_RenameGroup_Success(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Old Name", OwnerID: 10}
	ownerMembership := &domain.GroupMembership{Role: domain.MembershipRoleCreateOwner, Status: domain.MembershipActive}
	svc := newQuinielaSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{membership: ownerMembership},
	)

	got, err := svc.RenameGroup(context.Background(), 1, 10, quinielaNewName)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.Name != quinielaNewName {
		t.Errorf("expected name %q, got %q", quinielaNewName, got.Name)
	}
}

func TestQuinielaService_RenameGroup_NotOwner_ReturnsForbidden(t *testing.T) {
	memberMembership := &domain.GroupMembership{Role: domain.MembershipRoleMember, Status: domain.MembershipActive}
	svc := newQuinielaSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: memberMembership},
	)

	if _, err := svc.RenameGroup(context.Background(), 1, 99, quinielaNewName); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-owner caller, got %v", err)
	}
}

func TestQuinielaService_RenameGroup_NoMembership_ReturnsForbidden(t *testing.T) {
	svc := newQuinielaSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: nil},
	)

	if _, err := svc.RenameGroup(context.Background(), 1, 99, quinielaNewName); !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for missing membership, got %v", err)
	}
}

func TestQuinielaService_RenameGroup_EmptyName_ReturnsValidation(t *testing.T) {
	ownerMembership := &domain.GroupMembership{Role: domain.MembershipRoleCreateOwner, Status: domain.MembershipActive}
	svc := newQuinielaSvc(
		&stubQuinielaRepo{quiniela: &domain.Quiniela{ID: 1}},
		&stubMemberRepo{membership: ownerMembership},
	)

	if _, err := svc.RenameGroup(context.Background(), 1, 10, "   "); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected ErrValidation for blank name, got %v", err)
	}
}

func TestQuinielaService_RenameGroup_QuinielaNotFound_ReturnsNotFound(t *testing.T) {
	ownerMembership := &domain.GroupMembership{Role: domain.MembershipRoleCreateOwner, Status: domain.MembershipActive}
	svc := newQuinielaSvc(
		&stubQuinielaRepo{quiniela: nil},
		&stubMemberRepo{membership: ownerMembership},
	)

	if _, err := svc.RenameGroup(context.Background(), 99, 10, quinielaNewName); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing quiniela, got %v", err)
	}
}

func TestQuinielaService_RenameGroup_NameConflict_ReturnsConflict(t *testing.T) {
	ownerMembership := &domain.GroupMembership{Role: domain.MembershipRoleCreateOwner, Status: domain.MembershipActive}
	svc := newQuinielaSvc(
		&stubQuinielaRepo{
			quiniela:  &domain.Quiniela{ID: 1, Name: "Old Name"},
			updateErr: apperrors.Conflict("a group with this name already exists"),
		},
		&stubMemberRepo{membership: ownerMembership},
	)

	if _, err := svc.RenameGroup(context.Background(), 1, 10, "Taken Name"); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected ErrConflict for duplicate name, got %v", err)
	}
}

func TestQuinielaService_RenameGroup_MemberRepoError_ReturnsError(t *testing.T) {
	dbErr := errors.New("db down")
	svc := newQuinielaSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{err: dbErr},
	)

	if _, err := svc.RenameGroup(context.Background(), 1, 10, quinielaNewName); err == nil {
		t.Error("expected an error from memberRepo, got nil")
	}
}
