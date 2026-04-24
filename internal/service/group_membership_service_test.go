package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	membershipCode    = "CODE"
	membershipDBError = "db error"
)

// ── GroupMembershipService tests ──────────────────────────────────────────────

// recordingPaymentService records calls to CreateRecord for assertion in tests.
type recordingPaymentService struct {
	noopPaymentService
	created []*domain.PaymentRecord
}

func (s *recordingPaymentService) CreateRecord(_ context.Context, quinielaID, userID, amount int, currency, _ string) (*domain.PaymentRecord, error) {
	r := &domain.PaymentRecord{QuinielaID: quinielaID, UserID: userID, Amount: amount, Currency: currency}
	s.created = append(s.created, r)
	return r, nil
}

// noopPaymentService is a no-op implementation of PaymentService for tests
// that do not care about payment side-effects.
type noopPaymentService struct{}

func (noopPaymentService) CreateRecord(_ context.Context, _, _, _ int, _, _ string) (*domain.PaymentRecord, error) {
	return &domain.PaymentRecord{}, nil
}
func (noopPaymentService) ValidateDeposit(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return nil, nil
}
func (noopPaymentService) RejectDeposit(_ context.Context, _, _ int, _ string) (*domain.PaymentRecord, error) {
	return nil, nil
}
func (noopPaymentService) ListPending(_ context.Context) ([]*domain.PaymentRecord, error) {
	return nil, nil
}
func (noopPaymentService) ListByQuiniela(_ context.Context, _ int) ([]*domain.PaymentRecord, error) {
	return nil, nil
}
func (noopPaymentService) List(_ context.Context, _ repository.PaymentFilters, _ repository.Pagination) ([]*domain.PaymentRecord, error) {
	return nil, nil
}

func newMemberSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) GroupMembershipService {
	return NewGroupMembershipService(qr, mr, &noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{}, zap.NewNop())
}

func quinielaWithCode(id int, code string) *domain.Quiniela {
	return &domain.Quiniela{ID: id, Name: "Test", OwnerID: 1, InviteCode: code}
}

// activeMembership returns an active membership for use as an approver stub.
func activeMembership(quinielaID, userID int) *domain.GroupMembership {
	now := time.Now()
	return &domain.GroupMembership{
		ID:         1,
		QuinielaID: quinielaID,
		UserID:     userID,
		Status:     domain.MembershipActive,
		JoinedAt:   &now,
	}
}

// pendingMembership returns a pending membership for use as the join-request stub.
func pendingMembership(id, quinielaID, userID int) *domain.GroupMembership {
	return &domain.GroupMembership{
		ID:         id,
		QuinielaID: quinielaID,
		UserID:     userID,
		Status:     domain.MembershipPending,
	}
}

// ── Join ──────────────────────────────────────────────────────────────────────

func TestGroupMembershipService_Join_NewMember_ReturnsPending(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, "VALIDCODE")},
		&stubMemberRepo{membership: nil},
	)

	m, err := svc.Join(context.Background(), "VALIDCODE", 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if m.Status != domain.MembershipPending {
		t.Errorf("expected pending status, got %s", m.Status)
	}
	if m.JoinedAt != nil {
		t.Error("JoinedAt must be nil for a pending request (set on approval)")
	}
}

func TestGroupMembershipService_Join_CodeNotFound_ReturnsNotFound(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: nil},
		&stubMemberRepo{},
	)

	_, err := svc.Join(context.Background(), "BADCODE", 42)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestGroupMembershipService_Join_AlreadyActive_ReturnsConflict(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, membershipCode)},
		&stubMemberRepo{membership: activeMembership(1, 42)},
	)

	_, err := svc.Join(context.Background(), membershipCode, 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestGroupMembershipService_Join_AlreadyPending_ReturnsConflict(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, membershipCode)},
		&stubMemberRepo{membership: pendingMembership(1, 1, 42)},
	)

	_, err := svc.Join(context.Background(), membershipCode, 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for duplicate pending request, got %v", err)
	}
}

func TestGroupMembershipService_Join_PreviouslyLeft_ReturnsPending(t *testing.T) {
	existing := &domain.GroupMembership{
		ID:         1,
		QuinielaID: 1,
		UserID:     42,
		Status:     domain.MembershipLeft,
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, membershipCode)},
		&stubMemberRepo{membership: existing},
	)

	m, err := svc.Join(context.Background(), membershipCode, 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if m.Status != domain.MembershipPending {
		t.Errorf("expected pending status for re-join, got %s", m.Status)
	}
}

func TestGroupMembershipService_Join_MaxMembersReached_ReturnsConflict(t *testing.T) {
	maxMembers := 1
	q := &domain.Quiniela{ID: 1, Name: "Full", OwnerID: 1, InviteCode: membershipCode, MaxMembers: &maxMembers}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{activeCount: 1}, // CountActive returns 1, which equals maxMembers
	)

	_, err := svc.Join(context.Background(), membershipCode, 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict (full group) error, got %v", err)
	}
}

func TestGroupMembershipService_Join_FreeGroup_AutoPaid(t *testing.T) {
	q := quinielaWithCode(1, "FREECODE")
	q.EntryFee = 0
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{membership: nil},
	)

	m, err := svc.Join(context.Background(), "FREECODE", 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if !m.Paid {
		t.Error("expected Paid = true for free group even while pending")
	}
}

func TestGroupMembershipService_Join_PaidGroup_NotAutoPaid(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 200
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{membership: nil},
	)

	m, err := svc.Join(context.Background(), "PAIDCODE", 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if m.Paid {
		t.Error("expected Paid = false for paid group until payment confirmed")
	}
}

func TestGroupMembershipService_Join_PaidGroup_CreatesPendingPaymentRecord(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 200
	q.Currency = "GTQ"

	recorder := &recordingPaymentService{}
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{membership: nil},
		&noopSystemParamService{},
		&noopAuditLogger{},
		recorder,
		zap.NewNop(),
	)

	if _, err := svc.Join(context.Background(), "PAIDCODE", 42); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(recorder.created) != 1 {
		t.Fatalf("expected 1 payment record created, got %d", len(recorder.created))
	}
	rec := recorder.created[0]
	if rec.Amount != 200 || rec.Currency != "GTQ" || rec.QuinielaID != 1 || rec.UserID != 42 {
		t.Errorf("payment record mismatch: %+v", rec)
	}
}

// ── ApproveJoin ───────────────────────────────────────────────────────────────

func TestGroupMembershipService_ApproveJoin_Success_ReturnsActive(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    3,
		},
	)

	got, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf("expected active status after approval, got %s", got.Status)
	}
	if got.JoinedAt == nil {
		t.Error("expected JoinedAt to be set after approval")
	}
}

func TestGroupMembershipService_ApproveJoin_ApproverNotMember_ReturnsForbidden(t *testing.T) {
	// approver has no membership (nil returned by GetByQuinielaAndUser)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: nil},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected forbidden for non-member approver, got %v", err)
	}
}

func TestGroupMembershipService_ApproveJoin_ApproverPending_ReturnsForbidden(t *testing.T) {
	// approver is pending (not yet active) — must not be able to approve
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: pendingMembership(1, 1, 10)},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected forbidden for pending approver, got %v", err)
	}
}

func TestGroupMembershipService_ApproveJoin_MembershipNotFound_ReturnsNotFound(t *testing.T) {
	// approver is valid but the membership being approved does not exist
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     activeMembership(1, 10),
			membershipByID: nil, // pending not found
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found when pending membership absent, got %v", err)
	}
}

func TestGroupMembershipService_ApproveJoin_WrongQuiniela_ReturnsNotFound(t *testing.T) {
	approver := activeMembership(1, 10)
	// pending belongs to a different quiniela
	pending := pendingMembership(99, 2, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10) // path quinielaID = 1
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found for cross-quiniela approval attempt, got %v", err)
	}
}

func TestGroupMembershipService_ApproveJoin_NotPending_ReturnsConflict(t *testing.T) {
	approver := activeMembership(1, 10)
	// The membership belongs to quinielaID=1 but is already active (not pending).
	alreadyActive := activeMembership(1, 42)
	alreadyActive.ID = 99
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: alreadyActive,
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for non-pending membership, got %v", err)
	}
}

// ── Leave ─────────────────────────────────────────────────────────────────────

func TestGroupMembershipService_Leave_ActiveMember_ReturnsNil(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: activeMembership(1, 42), activeCount: 1},
	)

	if err := svc.Leave(context.Background(), 1, 42); err != nil {
		t.Errorf("expected nil for valid self-leave, got %v", err)
	}
}

func TestGroupMembershipService_Leave_NotMember_ReturnsValidation(t *testing.T) {
	// GetByQuinielaAndUser returns nil — user has no membership
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: nil},
	)

	if err := svc.Leave(context.Background(), 1, 42); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for non-member leave, got %v", err)
	}
}

func TestGroupMembershipService_Leave_AlreadyLeft_ReturnsValidation(t *testing.T) {
	left := &domain.GroupMembership{ID: 1, QuinielaID: 1, UserID: 42, Status: domain.MembershipLeft}
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: left},
	)

	if err := svc.Leave(context.Background(), 1, 42); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for already-left member, got %v", err)
	}
}

// ── ListByQuiniela / ListByUser ───────────────────────────────────────────────

func TestGroupMembershipService_ListByQuiniela_ReturnsMemberships(t *testing.T) {
	memberships := []*domain.GroupMembership{
		{ID: 1, QuinielaID: 1, UserID: 10, Status: domain.MembershipActive},
		{ID: 2, QuinielaID: 1, UserID: 11, Status: domain.MembershipActive},
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{memberships: memberships},
	)

	got, err := svc.ListByQuiniela(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 memberships, got %d", len(got))
	}
}

func TestGroupMembershipService_ListByUser_ReturnsMemberships(t *testing.T) {
	memberships := []*domain.GroupMembership{
		{ID: 1, QuinielaID: 1, UserID: 10, Status: domain.MembershipActive},
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{memberships: memberships},
	)

	got, err := svc.ListByUser(context.Background(), 10)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 membership, got %d", len(got))
	}
}

// ── MarkPaid ──────────────────────────────────────────────────────────────────

// ── syncGroupStatus error paths ───────────────────────────────────────────────

// syncGroupStatus errors are logged and swallowed, so the parent operation
// (ApproveJoin / Leave) must still succeed when CountActive or UpdateStatus fail.

func TestGroupMembershipService_ApproveJoin_SyncCountActiveError_StillSucceeds(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			countActiveErr: errors.New(membershipDBError),
		},
	)

	got, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if err != nil {
		t.Fatalf("expected ApproveJoin to succeed despite sync error, got %v", err)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf("expected active status, got %s", got.Status)
	}
}

func TestGroupMembershipService_Leave_SyncUpdateStatusError_StillSucceeds(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{updateStatusErr: errors.New(membershipDBError)},
		&stubMemberRepo{membership: activeMembership(1, 42), activeCount: 1},
	)

	if err := svc.Leave(context.Background(), 1, 42); err != nil {
		t.Errorf("expected Leave to succeed despite sync error, got %v", err)
	}
}

// ── Leave — ownership transfer ────────────────────────────────────────────────

func TestGroupMembershipService_Leave_CreateOwner_TransfersOwnership(t *testing.T) {
	// Leaving user is the CreateOwner; there is an eligible successor.
	ownerMembership := &domain.GroupMembership{
		ID:         1,
		QuinielaID: 1,
		UserID:     10,
		Status:     domain.MembershipActive,
		Role:       domain.MembershipRoleCreateOwner,
	}
	successor := &domain.GroupMembership{
		ID:         2,
		QuinielaID: 1,
		UserID:     20,
		Status:     domain.MembershipActive,
		Role:       domain.MembershipRoleMember,
	}
	mr := &leaveOwnerMemberRepo{
		ownerMembership: ownerMembership,
		successor:       successor,
	}
	svc := NewGroupMembershipService(&stubQuinielaRepo{}, mr, &noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{}, zap.NewNop())

	if err := svc.Leave(context.Background(), 1, 10); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if mr.setRoleMembershipID != successor.ID {
		t.Errorf("expected SetRole on successor membership %d, got %d", successor.ID, mr.setRoleMembershipID)
	}
}

func TestGroupMembershipService_Leave_CreateOwner_NoSuccessor_StillLeaves(t *testing.T) {
	ownerMembership := &domain.GroupMembership{
		ID: 1, QuinielaID: 1, UserID: 10,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleCreateOwner,
	}
	mr := &leaveOwnerMemberRepo{ownerMembership: ownerMembership, successor: nil}
	svc := NewGroupMembershipService(&stubQuinielaRepo{}, mr, &noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{}, zap.NewNop())

	if err := svc.Leave(context.Background(), 1, 10); err != nil {
		t.Fatalf("expected Leave to succeed even without a successor, got %v", err)
	}
}

func TestGroupMembershipService_Leave_CreateOwner_TransferError_StillLeaves(t *testing.T) {
	ownerMembership := &domain.GroupMembership{
		ID: 1, QuinielaID: 1, UserID: 10,
		Status: domain.MembershipActive,
		Role:   domain.MembershipRoleCreateOwner,
	}
	mr := &leaveOwnerMemberRepo{
		ownerMembership: ownerMembership,
		transferErr:     errors.New(membershipDBError),
	}
	svc := NewGroupMembershipService(&stubQuinielaRepo{}, mr, &noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{}, zap.NewNop())

	// Transfer failure must be logged-and-swallowed; Leave itself must succeed.
	if err := svc.Leave(context.Background(), 1, 10); err != nil {
		t.Fatalf("expected Leave to succeed despite transfer error, got %v", err)
	}
}

// leaveOwnerMemberRepo lets GetByQuinielaAndUser return the owner membership
// and OldestActiveMember return a configurable successor.
type leaveOwnerMemberRepo struct {
	stubMemberRepo
	ownerMembership     *domain.GroupMembership
	successor           *domain.GroupMembership
	transferErr         error
	setRoleMembershipID int
}

func (r *leaveOwnerMemberRepo) GetByQuinielaAndUser(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	return r.ownerMembership, nil
}
func (r *leaveOwnerMemberRepo) OldestActiveMember(_ context.Context, _, _ int) (*domain.GroupMembership, error) {
	if r.transferErr != nil {
		return nil, r.transferErr
	}
	return r.successor, nil
}
func (r *leaveOwnerMemberRepo) SetRole(_ context.Context, membershipID int, _ domain.MembershipRole) error {
	r.setRoleMembershipID = membershipID
	return nil
}
func (r *leaveOwnerMemberRepo) Update(_ context.Context, _ *domain.GroupMembership) error {
	return nil
}
func (r *leaveOwnerMemberRepo) CountActive(_ context.Context, _ int) (int, error) {
	return 0, nil
}

// ── checkCapacity error path ───────────────────────────────────────────────────

func TestGroupMembershipService_Join_ListByQuinielaError_ReturnsError(t *testing.T) {
	maxMembers := 5
	q := &domain.Quiniela{ID: 1, Name: "Pool", OwnerID: 1, InviteCode: membershipCode, MaxMembers: &maxMembers}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{err: errors.New(membershipDBError)},
	)

	if _, err := svc.Join(context.Background(), membershipCode, 42); err == nil {
		t.Error("expected error when ListByQuiniela fails in checkCapacity, got nil")
	}
}

// ── MarkPaid ──────────────────────────────────────────────────────────────────

func TestGroupMembershipService_MarkPaid_ReturnsMembership(t *testing.T) {
	now := time.Now()
	m := &domain.GroupMembership{
		ID: 1, QuinielaID: 1, UserID: 42,
		Status: domain.MembershipActive, Paid: true, JoinedAt: &now,
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: m},
	)

	got, err := svc.MarkPaid(context.Background(), 1, 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if !got.Paid {
		t.Error("expected Paid = true after MarkPaid")
	}
}

func TestGroupMembershipService_MarkPaid_RepoError_Propagates(t *testing.T) {
	svc := newMemberSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New(membershipDBError)})

	_, err := svc.MarkPaid(context.Background(), 1, 42)
	if err == nil {
		t.Error("expected error from MarkPaid, got nil")
	}
}

func TestGroupMembershipService_ListByQuiniela_RepoError_Propagates(t *testing.T) {
	svc := newMemberSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New(membershipDBError)})

	_, err := svc.ListByQuiniela(context.Background(), 1)
	if err == nil {
		t.Error("expected error from ListByQuiniela, got nil")
	}
}
