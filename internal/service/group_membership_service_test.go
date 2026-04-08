package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── GroupMembershipService tests ──────────────────────────────────────────────

func newMemberSvc(qr *stubQuinielaRepo, mr *stubMemberRepo) GroupMembershipService {
	return NewGroupMembershipService(qr, mr)
}

func quinielaWithCode(id int, code string) *domain.Quiniela {
	return &domain.Quiniela{ID: id, Name: "Test", OwnerID: 1, InviteCode: code}
}

func TestGroupMembershipService_Join_NewMember_ReturnsActive(t *testing.T) {
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, "VALIDCODE")},
		&stubMemberRepo{membership: nil}, // no existing membership
	)

	m, err := svc.Join(context.Background(), "VALIDCODE", 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("expected active status, got %s", m.Status)
	}
	if m.JoinedAt == nil {
		t.Error("expected JoinedAt to be set")
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
	now := time.Now()
	existing := &domain.GroupMembership{
		ID:         1,
		QuinielaID: 1,
		UserID:     42,
		Status:     domain.MembershipActive,
		JoinedAt:   &now,
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, "CODE")},
		&stubMemberRepo{membership: existing},
	)

	_, err := svc.Join(context.Background(), "CODE", 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestGroupMembershipService_Join_PreviouslyLeft_ReturnsActive(t *testing.T) {
	existing := &domain.GroupMembership{
		ID:         1,
		QuinielaID: 1,
		UserID:     42,
		Status:     domain.MembershipLeft,
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: quinielaWithCode(1, "CODE")},
		&stubMemberRepo{membership: existing},
	)

	m, err := svc.Join(context.Background(), "CODE", 42)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if m.Status != domain.MembershipActive {
		t.Errorf("expected active status, got %s", m.Status)
	}
}

func TestGroupMembershipService_Join_MaxMembersReached_ReturnsConflict(t *testing.T) {
	maxMembers := 1
	q := &domain.Quiniela{ID: 1, Name: "Full", OwnerID: 1, InviteCode: "CODE", MaxMembers: &maxMembers}
	now := time.Now()
	activeMember := &domain.GroupMembership{
		ID: 1, QuinielaID: 1, UserID: 99, Status: domain.MembershipActive, JoinedAt: &now,
	}
	svc := newMemberSvc(
		&stubQuinielaRepo{quiniela: q},
		&stubMemberRepo{memberships: []*domain.GroupMembership{activeMember}},
	)

	_, err := svc.Join(context.Background(), "CODE", 42)
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict (full group) error, got %v", err)
	}
}

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
		t.Error("expected Paid = true for free group")
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
