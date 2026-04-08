package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// stubQuinielaRepo implements repository.QuinielaRepository with configurable returns.
type stubQuinielaRepo struct {
	quiniela  *domain.Quiniela
	quinielas []*domain.Quiniela
	err       error
}

func (r *stubQuinielaRepo) Create(_ context.Context, _ *domain.Quiniela) error { return r.err }
func (r *stubQuinielaRepo) GetByID(_ context.Context, _ int) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) Update(_ context.Context, _ *domain.Quiniela) error { return r.err }
func (r *stubQuinielaRepo) Delete(_ context.Context, _ int) error              { return r.err }
func (r *stubQuinielaRepo) ListByOwner(_ context.Context, _ int) ([]*domain.Quiniela, error) {
	return r.quinielas, r.err
}

// stubMemberRepo implements repository.GroupMembershipRepository for service tests.
type stubMemberRepo struct {
	membership  *domain.GroupMembership
	memberships []*domain.GroupMembership
	err         error
}

func (r *stubMemberRepo) Create(_ context.Context, _ *domain.GroupMembership) error { return r.err }
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

// ── QuinielaService tests ─────────────────────────────────────────────────────

func TestQuinielaService_Create_ValidQuiniela_ReturnsNil(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Errorf(fmtExpectNil, err)
	}
}

func TestQuinielaService_Create_SetsInviteCode(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.InviteCode == "" {
		t.Error("expected InviteCode to be set after Create")
	}
	if len(q.InviteCode) != inviteCodeLength {
		t.Errorf("expected invite code length %d, got %d", inviteCodeLength, len(q.InviteCode))
	}
}

func TestQuinielaService_Create_EmptyName_ReturnsValidation(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{OwnerID: 1}

	if err := svc.Create(context.Background(), q); !errors.Is(err, apperrors.ErrValidation) {
		t.Errorf("expected validation error for empty name, got %v", err)
	}
}

func TestQuinielaService_GetByID_Found_ReturnsQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Test Pool", OwnerID: 2}
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.GetByID(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID 1, got %d", got.ID)
	}
}

func TestQuinielaService_GetByID_NotFound_ReturnsNotFound(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: nil}, &stubMemberRepo{})

	if _, err := svc.GetByID(context.Background(), 99); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

func TestQuinielaService_GetByOwner_ReturnsSlice(t *testing.T) {
	qs := []*domain.Quiniela{{ID: 1, Name: "Pool A", OwnerID: 1}}
	svc := NewQuinielaService(&stubQuinielaRepo{quinielas: qs}, &stubMemberRepo{})

	got, err := svc.GetByOwner(context.Background(), 1)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if len(got) != 1 {
		t.Errorf("expected 1 quiniela, got %d", len(got))
	}
}
