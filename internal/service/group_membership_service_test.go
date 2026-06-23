package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	membershipCode    = "CODE"
	membershipDBError = "db error"
)

// ── GroupMembershipService tests ──────────────────────────────────────────────

// errPaymentService always returns an error from CreateRecord.
// Used to verify that createPendingPayment swallows errors and does not
// propagate them to the caller.
type errPaymentService struct {
	noopPaymentService
}

func (errPaymentService) CreateRecord(_ context.Context, _, _, _ int, _, _ string) (*domain.PaymentRecord, error) {
	return nil, fmt.Errorf("payment store unavailable")
}

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
	if mr.joinQuiniela == nil && qr.quiniela != nil {
		mr.joinQuiniela = qr.quiniela
	}
	return NewGroupMembershipService(qr, mr, &noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{}, zap.NewNop())
}

func quinielaWithCode(id int, code string) *domain.Quiniela {
	return &domain.Quiniela{ID: id, Name: "Test", OwnerID: 1, InviteCode: code, RequireApproval: true}
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
	q := &domain.Quiniela{ID: 1, Name: "Full", OwnerID: 1, InviteCode: membershipCode, RequireApproval: true}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{joinErr: apperrors.Conflict("this group has reached its maximum number of members")},
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

	mr := &stubMemberRepo{joinQuiniela: q}
	recorder := &recordingPaymentService{}
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
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

func TestGroupMembershipService_Join_PaidGroup_PaymentError_JoinStillSucceeds(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 100
	mr := &stubMemberRepo{joinQuiniela: q}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		errPaymentService{},
		zap.NewNop(),
	)

	m, err := svc.Join(context.Background(), "PAIDCODE", 7)
	if err != nil {
		t.Fatalf("expected join to succeed despite payment error, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected membership, got nil")
	}
}

// failOnUpdateMemberRepo wraps stubMemberRepo but forces the atomic join
// operation to fail, allowing Join error propagation to be tested directly.
type failOnUpdateMemberRepo struct {
	stubMemberRepo
	updateErr error
}

func (r *failOnUpdateMemberRepo) RequestJoinByInviteCode(_ context.Context, _ string, _, _, _ int) (*domain.Quiniela, *domain.GroupMembership, error) {
	return nil, nil, r.updateErr
}
func (r *failOnUpdateMemberRepo) BulkDebitAndMarkPaid(_ context.Context, _, _ int) ([]int, error) {
	return nil, r.updateErr
}

func TestGroupMembershipService_Join_PaidGroup_Rejoin_CreatesPendingPayment(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 150
	q.Currency = "GTQ"

	existing := &domain.GroupMembership{ID: 5, QuinielaID: 1, UserID: 42, Status: domain.MembershipLeft}
	mr2 := &stubMemberRepo{membership: existing, joinQuiniela: q}
	recorder := &recordingPaymentService{}
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr2,
		&noopSystemParamService{},
		&noopAuditLogger{},
		recorder,
		zap.NewNop(),
	)

	if _, err := svc.Join(context.Background(), "PAIDCODE", 42); err != nil {
		t.Fatalf("expected rejoin to succeed, got: %v", err)
	}
	if len(recorder.created) != 1 {
		t.Fatalf("expected 1 pending payment on rejoin, got %d", len(recorder.created))
	}
}

func TestGroupMembershipService_Join_RejoinUpdateError_ReturnsError(t *testing.T) {
	q := quinielaWithCode(1, "CODE")
	existing := &domain.GroupMembership{ID: 5, QuinielaID: 1, UserID: 42, Status: domain.MembershipLeft}
	mr := &failOnUpdateMemberRepo{
		stubMemberRepo: stubMemberRepo{membership: existing, joinQuiniela: q},
		updateErr:      errors.New("db write failed"),
	}
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
	)

	if _, err := svc.Join(context.Background(), "CODE", 42); err == nil {
		t.Fatal("expected error from rejoin Update failure, got nil")
	}
}

// ── ApproveJoin ───────────────────────────────────────────────────────────────

func TestGroupMembershipService_ApproveJoin_Success_ReturnsActive(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
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
	// approver is pending (not yet active) - must not be able to approve
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

func TestGroupMembershipService_ApproveJoin_FreeGroupAtLimit_ReturnsConflict(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	// freeMax defaults to 5; activeCount=5 means we're at the limit
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    5,
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict when free group is at member limit, got %v", err)
	}
}

func TestGroupMembershipService_ApproveJoin_FreeGroupBelowLimit_Succeeds(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    4, // below default freeMax of 5
		},
	)

	got, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if err != nil {
		t.Fatalf("expected success for free group below limit, got %v", err)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf("expected active status, got %s", got.Status)
	}
}

func TestGroupMembershipService_ApproveJoin_PremiumGroupUnpaidMember_ChargesEntryFee(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	pending.Paid = false
	q := quinielaWithCode(1, "CODE")
	q.IsPremium = true
	q.EntryFee = 500
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
		},
	)

	got, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if err != nil {
		t.Fatalf("expected success for premium group approval with entry fee, got %v", err)
	}
	if got.Status != domain.MembershipActive {
		t.Errorf("expected active status, got %s", got.Status)
	}
}

func TestGroupMembershipService_ApproveJoin_PremiumGroupDebitError_PropagatesError(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	pending.Paid = false
	q := quinielaWithCode(1, "CODE")
	q.IsPremium = true
	q.EntryFee = 500
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			err:            apperrors.Conflict("insufficient balance"),
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if err == nil {
		t.Fatal("expected error when debit fails, got nil")
	}
}

func TestGroupMembershipService_ApproveJoin_GroupNotFound_ReturnsNotFound(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: nil}, // GetByID returns nil
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
		},
	)

	_, err := svc.ApproveJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found when quiniela missing, got %v", err)
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
	// GetByQuinielaAndUser returns nil - user has no membership
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

// ── RejectJoin ────────────────────────────────────────────────────────────────

func TestGroupMembershipService_RejectJoin_Success(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
		},
	)

	if err := svc.RejectJoin(context.Background(), 1, 99, 10); err != nil {
		t.Fatalf("expected nil error on successful reject, got %v", err)
	}
}

func TestGroupMembershipService_RejectJoin_RejectorNotMember_ReturnsForbidden(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{membership: nil},
	)

	err := svc.RejectJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected forbidden for non-member rejector, got %v", err)
	}
}

func TestGroupMembershipService_RejectJoin_MembershipNotFound_ReturnsNotFound(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     activeMembership(1, 10),
			membershipByID: nil,
		},
	)

	err := svc.RejectJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found when pending membership absent, got %v", err)
	}
}

func TestGroupMembershipService_RejectJoin_WrongQuiniela_ReturnsNotFound(t *testing.T) {
	approver := activeMembership(1, 10)
	wrongGroup := pendingMembership(99, 2, 42) // belongs to quinielaID=2
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: wrongGroup,
		},
	)

	err := svc.RejectJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found for cross-quiniela reject, got %v", err)
	}
}

func TestGroupMembershipService_RejectJoin_NotPending_ReturnsConflict(t *testing.T) {
	approver := activeMembership(1, 10)
	alreadyActive := activeMembership(1, 42)
	alreadyActive.ID = 99
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: alreadyActive,
		},
	)

	err := svc.RejectJoin(context.Background(), 1, 99, 10)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for non-pending membership, got %v", err)
	}
}

func TestGroupMembershipService_RejectJoin_RemoveByAdminError_Propagates(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			removeErr:      errors.New(membershipDBError),
		},
	)

	if err := svc.RejectJoin(context.Background(), 1, 99, 10); err == nil {
		t.Fatal("expected error when RemoveByAdmin fails, got nil")
	}
}

// ── ListByUser ────────────────────────────────────────────────────────────────

func TestGroupMembershipService_ListByUser_FiltersOutLeftMemberships(t *testing.T) {
	memberships := []*domain.GroupMembership{
		{ID: 1, QuinielaID: 1, UserID: 10, Status: domain.MembershipActive},
		{ID: 2, QuinielaID: 2, UserID: 10, Status: domain.MembershipLeft},
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
		t.Errorf("expected left memberships to be filtered out, got %d memberships", len(got))
	}
	if got[0].ID != 1 {
		t.Errorf("expected only the active membership to remain, got ID %d", got[0].ID)
	}
}

func TestGroupMembershipService_ListByUser_RepoError_Propagates(t *testing.T) {
	svc := newMemberSvc(&stubQuinielaRepo{}, &stubMemberRepo{err: errors.New(membershipDBError)})

	_, err := svc.ListByUser(context.Background(), 10)
	if err == nil {
		t.Error("expected error from ListByUser, got nil")
	}
}

// ── MarkPaid ──────────────────────────────────────────────────────────────────

// ── ApproveMembership / LeaveMembership error propagation ─────────────────────

// The membership write and group-status recalculation are now committed
// atomically. Any error from ApproveMembership or LeaveMembership must
// propagate to the caller so the API can surface the failure correctly.

func TestGroupMembershipService_ApproveJoin_ApproveMembershipError_PropagatesError(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			approveErr:     errors.New(membershipDBError),
		},
	)

	if _, err := svc.ApproveJoin(context.Background(), 1, 99, 10); err == nil {
		t.Fatal("expected ApproveJoin to fail when ApproveMembership returns error")
	}
}

func TestGroupMembershipService_Leave_LeaveMembershipError_PropagatesError(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{},
		&stubMemberRepo{
			membership: activeMembership(1, 42),
			leaveErr:   errors.New(membershipDBError),
		},
	)

	if err := svc.Leave(context.Background(), 1, 42); err == nil {
		t.Error("expected Leave to fail when LeaveMembership returns error")
	}
}

// ── Leave - ownership transfer ────────────────────────────────────────────────

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
	if mr.leaveTransferSuccessorID != successor.ID {
		t.Errorf("expected atomic transfer to successor membership %d, got %d", successor.ID, mr.leaveTransferSuccessorID)
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

func TestGroupMembershipService_Leave_CreateOwner_TransferError_ReturnsError(t *testing.T) {
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

	if err := svc.Leave(context.Background(), 1, 10); err == nil {
		t.Fatal("expected Leave to fail when ownership transfer cannot be completed atomically")
	}
}

// leaveOwnerMemberRepo lets GetByQuinielaAndUser return the owner membership,
// OldestActiveMember return a configurable successor, and
// LeaveMembershipAndTransferOwnership record the chosen successor.
type leaveOwnerMemberRepo struct {
	stubMemberRepo
	ownerMembership          *domain.GroupMembership
	successor                *domain.GroupMembership
	transferErr              error
	leaveTransferSuccessorID int
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
func (r *leaveOwnerMemberRepo) LeaveMembershipAndTransferOwnership(_ context.Context, _, _, successorMembershipID int, _ time.Time, _ int) error {
	r.leaveTransferSuccessorID = successorMembershipID
	return r.leaveErr
}

// ── checkCapacity error path ───────────────────────────────────────────────────

func TestGroupMembershipService_Join_AtomicJoinError_ReturnsError(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Pool", OwnerID: 1, InviteCode: membershipCode, RequireApproval: true}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{joinErr: errors.New(membershipDBError)},
	)

	if _, err := svc.Join(context.Background(), membershipCode, 42); err == nil {
		t.Error("expected error when atomic join operation fails, got nil")
	}
}

// ── require_approval=false auto-approve path ──────────────────────────────────

func TestGroupMembershipService_Join_AutoApprove_ReturnsActiveMembership(t *testing.T) {
	q := quinielaWithCode(1, membershipCode)
	q.RequireApproval = false
	pending := &domain.GroupMembership{ID: 7, QuinielaID: 1, UserID: 42, Status: domain.MembershipPending}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			joinQuiniela:   q,
			joinMembership: pending,
			membershipByID: pending, // ApproveMembership stub requires this to return non-nil
		},
	)

	m, err := svc.Join(context.Background(), membershipCode, 42)
	if err != nil {
		t.Fatalf("expected nil error on auto-approve join, got %v", err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("expected active membership on auto-approve, got %s", m.Status)
	}
}

func TestGroupMembershipService_Join_AutoApprove_ApproveMembershipError_Propagates(t *testing.T) {
	q := quinielaWithCode(1, membershipCode)
	q.RequireApproval = false
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			joinQuiniela:   q,
			joinMembership: &domain.GroupMembership{ID: 7, QuinielaID: 1, UserID: 42, Status: domain.MembershipPending},
			approveErr:     errors.New("approve failed"),
		},
	)

	_, err := svc.Join(context.Background(), membershipCode, 42)
	if err == nil {
		t.Error("expected error when ApproveMembership fails on auto-approve")
	}
}

func TestGroupMembershipService_JoinWithBalance_AutoApprove_ReturnsActiveMembership(t *testing.T) {
	q := quinielaWithCode(1, membershipCode)
	q.RequireApproval = false
	q.EntryFee = 100
	pending := &domain.GroupMembership{ID: 8, QuinielaID: 1, UserID: 42, Status: domain.MembershipPending}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			joinQuiniela:   q,
			joinMembership: pending,
			membershipByID: pending, // ApproveMembership stub requires this to return non-nil
			membership:     &domain.GroupMembership{ID: 8, QuinielaID: 1, UserID: 42, Paid: true},
		},
	)

	m, err := svc.JoinWithBalance(context.Background(), membershipCode, 42)
	if err != nil {
		t.Fatalf("expected nil error on auto-approve JoinWithBalance, got %v", err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("expected active membership on auto-approve, got %s", m.Status)
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

// ── JoinWithBalance ───────────────────────────────────────────────────────────

func TestGroupMembershipService_JoinWithBalance_Success(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 200
	q.Currency = "GTQ"

	// joinMembership is returned by RequestJoinByInviteCode (pending join).
	// membership is returned by DebitBalanceAndMarkPaid (paid/active after debit).
	joinMem := pendingMembership(3, 1, 42)
	now := time.Now()
	paidMem := &domain.GroupMembership{
		ID: 5, QuinielaID: 1, UserID: 42,
		Status: domain.MembershipActive, Paid: true, JoinedAt: &now,
	}
	mr := &stubMemberRepo{
		joinQuiniela:   q,
		joinMembership: joinMem,
		membership:     paidMem,
	}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
	)

	m, err := svc.JoinWithBalance(context.Background(), "PAIDCODE", 42)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected membership, got nil")
	}
}

func TestGroupMembershipService_JoinWithBalance_NoEntryFee_ReturnsConflict(t *testing.T) {
	q := quinielaWithCode(1, "FREECODE")
	q.EntryFee = 0
	mr := &stubMemberRepo{joinQuiniela: q}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
	)

	_, err := svc.JoinWithBalance(context.Background(), "FREECODE", 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict for free group, got %v", err)
	}
}

func TestGroupMembershipService_JoinWithBalance_RepoJoinError_Propagates(t *testing.T) {
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: nil},
		&stubMemberRepo{},
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
	)

	_, err := svc.JoinWithBalance(context.Background(), "BAD", 42)
	if err == nil {
		t.Error("expected error when repo cannot find invite code")
	}
}

func TestGroupMembershipService_JoinWithBalance_DebitError_Propagates(t *testing.T) {
	q := quinielaWithCode(1, "PAIDCODE")
	q.EntryFee = 100
	mr := &stubMemberRepo{joinQuiniela: q, err: errors.New("insufficient balance")}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
	)

	_, err := svc.JoinWithBalance(context.Background(), "PAIDCODE", 42)
	if err == nil {
		t.Error("expected error when DebitBalanceAndMarkPaid fails")
	}
}

// ── writeMembershipEvent ──────────────────────────────────────────────────────

// stubOutboxWriter is a minimal outbox.Writer for testing membership fan-out.
type stubOutboxWriter struct {
	err    error
	writes int
}

func (w *stubOutboxWriter) Write(_ context.Context, _ notification.EventType, _, _ string, _ any) error {
	w.writes++
	return w.err
}
func (w *stubOutboxWriter) WriteBatch(_ context.Context, _ []outbox.BatchEvent) error { return nil }
func (w *stubOutboxWriter) WriteDedup(_ context.Context, _ string, _ notification.EventType, _, _ string, _ any) (bool, error) {
	return true, nil
}
func (w *stubOutboxWriter) WriteInTx(_ context.Context, _ outbox.TxExecer, _ notification.EventType, _, _ string, _ any) error {
	return nil
}

func TestGroupMembershipService_ApproveJoin_WithOutboxWriter_WritesEvent(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	outboxW := &stubOutboxWriter{}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    1,
		},
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipOutboxWriter(outboxW),
	)

	if _, err := svc.ApproveJoin(context.Background(), 1, 99, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outboxW.writes != 1 {
		t.Errorf("expected 1 outbox write, got %d", outboxW.writes)
	}
}

func TestGroupMembershipService_Leave_WithOutboxWriter_WritesEvent(t *testing.T) {
	q := quinielaWithCode(1, "CODE")
	outboxW := &stubOutboxWriter{}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{membership: activeMembership(1, 42)},
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipOutboxWriter(outboxW),
	)

	if err := svc.Leave(context.Background(), 1, 42); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outboxW.writes != 1 {
		t.Errorf("expected 1 outbox write, got %d", outboxW.writes)
	}
}

// errOnNthGetQuinielaRepo wraps stubQuinielaRepo and returns an error on the
// Nth call to GetByID. ApproveJoin's own lookup (call 1) succeeds while the
// secondary lookup inside writeMembershipEvent (call 2) fails, exercising the
// best-effort fallback path.
type errOnNthGetQuinielaRepo struct {
	*stubQuinielaRepo
	n     int
	calls int
}

func (r *errOnNthGetQuinielaRepo) GetByID(ctx context.Context, id int) (*domain.Quiniela, error) {
	r.calls++
	if r.calls == r.n {
		return nil, errors.New("quiniela lookup failed")
	}
	return r.stubQuinielaRepo.GetByID(ctx, id)
}

func TestGroupMembershipService_WriteMembershipEvent_QuinielaError_BestEffort(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	outboxW := &stubOutboxWriter{}

	// The second GetByID call (inside writeMembershipEvent) returns an error;
	// the service must swallow it, fall back to a minimal Quiniela, and still
	// write the outbox event.
	qRepo := &errOnNthGetQuinielaRepo{
		stubQuinielaRepo: &stubQuinielaRepo{quiniela: q},
		n:                2,
	}

	svc := NewGroupMembershipService(
		qRepo,
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    1,
		},
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipOutboxWriter(outboxW),
	)

	if _, err := svc.ApproveJoin(context.Background(), 1, 99, 10); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if outboxW.writes != 1 {
		t.Errorf("expected 1 outbox write despite quiniela lookup error, got %d", outboxW.writes)
	}
}

func TestGroupMembershipService_WriteMembershipEvent_OutboxWriteError_BestEffort(t *testing.T) {
	approver := activeMembership(1, 10)
	pending := pendingMembership(99, 1, 42)
	q := quinielaWithCode(1, "CODE")
	outboxW := &stubOutboxWriter{err: errors.New("outbox unavailable")}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{
			membership:     approver,
			membershipByID: pending,
			activeCount:    1,
		},
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipOutboxWriter(outboxW),
	)

	// Should NOT return an error even if outbox write fails (best-effort).
	if _, err := svc.ApproveJoin(context.Background(), 1, 99, 10); err != nil {
		t.Fatalf("expected success despite outbox write error, got: %v", err)
	}
}

// ── entryFeeInUserCurrency (USD conversion) ───────────────────────────────────

// memberUserRepoStub is a minimal UserRepository for group membership tests
// that require a wired userRepo (USD entry-fee conversion path).
type memberUserRepoStub struct {
	currency string
}

func (r *memberUserRepoStub) Create(_ context.Context, _ *domain.User) error { return nil }
func (r *memberUserRepoStub) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, nil
}
func (r *memberUserRepoStub) GetByExternalSubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *memberUserRepoStub) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *memberUserRepoStub) Update(_ context.Context, _ *domain.User) error { return nil }
func (r *memberUserRepoStub) Delete(_ context.Context, _ int) error          { return nil }
func (r *memberUserRepoStub) List(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *memberUserRepoStub) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, nil
}
func (r *memberUserRepoStub) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *memberUserRepoStub) Unban(_ context.Context, _ int) error                 { return nil }
func (r *memberUserRepoStub) ListBanned(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *memberUserRepoStub) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", nil
}
func (r *memberUserRepoStub) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, nil
}
func (r *memberUserRepoStub) GetBalance(_ context.Context, _ int) (int, int, error) { return 0, 0, nil }
func (r *memberUserRepoStub) GetBalanceCurrency(_ context.Context, _ int) (string, error) {
	return r.currency, nil
}
func (r *memberUserRepoStub) UpdateLocale(_ context.Context, _ int, _ string) error   { return nil }
func (r *memberUserRepoStub) UpdateTimezone(_ context.Context, _ int, _ string) error { return nil }
func (r *memberUserRepoStub) SetRole(_ context.Context, _ int, _ domain.UserRole) (*domain.User, error) {
	return nil, nil
}

// memberFxSvcStub implements ExchangeRateService for group membership fee tests.
type memberFxSvcStub struct {
	usdCents int64
	convErr  error
}

func (f *memberFxSvcStub) RefreshRate(_ context.Context) (*domain.ExchangeRates, error) {
	return nil, nil
}
func (f *memberFxSvcStub) GetCurrentRates(_ context.Context) (*domain.ExchangeRates, error) {
	return nil, nil
}
func (f *memberFxSvcStub) OverrideRate(_ context.Context, _ decimal.Decimal, _ string, _ int) (*domain.ExchangeRates, error) {
	return nil, nil
}
func (f *memberFxSvcStub) ConvertUSDToGTQ(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return 0, decimal.Zero, nil
}
func (f *memberFxSvcStub) ConvertGTQToUSD(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return f.usdCents, decimal.Zero, f.convErr
}
func (f *memberFxSvcStub) WarmCache(_ context.Context) error { return nil }

func TestGroupMembershipService_JoinWithBalance_USDUser_UsesConvertedFee(t *testing.T) {
	q := quinielaWithCode(1, "USDCODE")
	q.EntryFee = 775 // GTQ; should be converted to USD for USD users
	q.Currency = "GTQ"

	joinMem := pendingMembership(3, 1, 42)
	now := time.Now()
	paidMem := &domain.GroupMembership{
		ID: 5, QuinielaID: 1, UserID: 42,
		Status: domain.MembershipActive, Paid: true, JoinedAt: &now,
	}
	mr := &stubMemberRepo{
		joinQuiniela:   q,
		joinMembership: joinMem,
		membership:     paidMem,
	}
	ur := &memberUserRepoStub{currency: "USD"}
	fx := &memberFxSvcStub{usdCents: 100}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipFxSvc(ur, fx),
	)

	m, err := svc.JoinWithBalance(context.Background(), "USDCODE", 42)
	if err != nil {
		t.Fatalf("expected success for USD user JoinWithBalance, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected membership, got nil")
	}
}

func TestGroupMembershipService_SetFxSvc_WiresConversionAndDoesNotPanic(t *testing.T) {
	svc := NewGroupMembershipService(
		&stubQuinielaRepo{}, &stubMemberRepo{},
		&noopSystemParamService{}, &noopAuditLogger{}, &noopPaymentService{},
		zap.NewNop(),
	)
	ur := &memberUserRepoStub{currency: "USD"}
	fx := &memberFxSvcStub{usdCents: 100}
	svc.(*groupMembershipService).SetFxSvc(ur, fx)
}

func TestGroupMembershipService_JoinWithBalance_USDUser_FxConversionError_FallsBackToGTQ(t *testing.T) {
	q := quinielaWithCode(1, "FXERRCODE")
	q.EntryFee = 775
	q.Currency = "GTQ"

	joinMem := pendingMembership(3, 1, 42)
	now := time.Now()
	paidMem := &domain.GroupMembership{
		ID: 5, QuinielaID: 1, UserID: 42,
		Status: domain.MembershipActive, Paid: true, JoinedAt: &now,
	}
	mr := &stubMemberRepo{
		joinQuiniela:   q,
		joinMembership: joinMem,
		membership:     paidMem,
	}
	ur := &memberUserRepoStub{currency: "USD"}
	fx := &memberFxSvcStub{convErr: errors.New("fx unavailable")}

	svc := NewGroupMembershipService(
		&stubQuinielaRepo{quiniela: q},
		mr,
		&noopSystemParamService{},
		&noopAuditLogger{},
		&noopPaymentService{},
		zap.NewNop(),
		WithGroupMembershipFxSvc(ur, fx),
	)

	// Should still succeed using the GTQ fee as fallback.
	m, err := svc.JoinWithBalance(context.Background(), "FXERRCODE", 42)
	if err != nil {
		t.Fatalf("expected success with GTQ fallback fee, got: %v", err)
	}
	if m == nil {
		t.Fatal("expected membership, got nil")
	}
}
