package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// stubQuinielaRepo implements repository.QuinielaRepository with configurable returns.
// updateStatusErr, if set, is returned exclusively by UpdateStatus so that
// syncGroupStatus error paths can be tested without affecting earlier calls.
type stubQuinielaRepo struct {
	quiniela        *domain.Quiniela
	quinielas       []*domain.Quiniela
	err             error
	updateStatusErr error
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
func (r *stubQuinielaRepo) RotateInviteCode(_ context.Context, _ int, _ string, _ *time.Time) (*domain.Quiniela, error) {
	return r.quiniela, r.err
}
func (r *stubQuinielaRepo) UpdateStatus(_ context.Context, _ int, _ domain.QuinielaStatus) error {
	return r.updateStatusErr
}

// stubMemberRepo implements repository.GroupMembershipRepository for service tests.
// membershipByID is returned by GetByID (used in ApproveJoin to load the pending
// request). membership is returned by GetByQuinielaAndUser (used to look up the
// approver and in Leave). activeCount is returned by CountActive.
// countActiveErr, if set, is returned exclusively by CountActive so that
// syncGroupStatus error paths can be tested without affecting earlier calls.
type stubMemberRepo struct {
	membership     *domain.GroupMembership
	membershipByID *domain.GroupMembership
	memberships    []*domain.GroupMembership
	activeCount    int
	err            error
	countActiveErr error
}

func (r *stubMemberRepo) Create(_ context.Context, _ *domain.GroupMembership) error { return r.err }
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

func TestQuinielaService_Create_InviteCodeNeverExpires(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.InviteCodeExpiresAt != nil {
		t.Errorf("expected InviteCodeExpiresAt to be nil (no expiry), got %v", q.InviteCodeExpiresAt)
	}
}

func TestQuinielaService_Create_InitialStatusIsInactive(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Oficina 2026", OwnerID: 1}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.Status != domain.QuinielaStatusInactive {
		t.Errorf("expected initial status %q, got %q", domain.QuinielaStatusInactive, q.Status)
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

func TestQuinielaService_Create_DefaultsCurrencyToMXN(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Pool", OwnerID: 1} // no Currency set

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.Currency != "MXN" {
		t.Errorf("expected default currency MXN, got %q", q.Currency)
	}
}

func TestQuinielaService_Create_DefaultsPrizeThresholdWhenZero(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Pool", OwnerID: 1} // no PrizeThreshold set

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.PrizeThreshold != domain.DefaultPrizeThreshold {
		t.Errorf("expected default prize_threshold %d, got %d", domain.DefaultPrizeThreshold, q.PrizeThreshold)
	}
}

func TestQuinielaService_Create_ExplicitPrizeThreshold_Preserved(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{}, &stubMemberRepo{})
	q := &domain.Quiniela{Name: "Pool", OwnerID: 1, PrizeThreshold: 5}

	if err := svc.Create(context.Background(), q); err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if q.PrizeThreshold != 5 {
		t.Errorf("explicit prize_threshold should be preserved, got %d", q.PrizeThreshold)
	}
}

func TestQuinielaService_Create_RepoConflict_ReturnsConflict(t *testing.T) {
	svc := NewQuinielaService(
		&stubQuinielaRepo{err: apperrors.Conflict("a group with this name already exists")},
		&stubMemberRepo{},
	)
	q := &domain.Quiniela{Name: "Duplicate", OwnerID: 1}

	if err := svc.Create(context.Background(), q); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("expected conflict error, got %v", err)
	}
}

func TestQuinielaService_GetByInviteCode_Found(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Pool", InviteCode: "ABC123"}
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.GetByInviteCode(context.Background(), "ABC123")
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got.ID != 1 {
		t.Errorf("expected ID 1, got %d", got.ID)
	}
}

func TestQuinielaService_GetByInviteCode_NotFound_ReturnsNotFound(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: nil}, &stubMemberRepo{})

	if _, err := svc.GetByInviteCode(context.Background(), "BADCODE"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected not-found error, got %v", err)
	}
}

// ── RotateInviteCode ──────────────────────────────────────────────────────────

func TestQuinielaService_RotateInviteCode_Success_ReturnsUpdatedQuiniela(t *testing.T) {
	q := &domain.Quiniela{ID: 1, Name: "Pool", OwnerID: 10, InviteCode: "OLDCODE123"}
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	got, err := svc.RotateInviteCode(context.Background(), 1, 10)
	if err != nil {
		t.Fatalf(fmtExpectNil, err)
	}
	if got == nil {
		t.Fatal("expected non-nil quiniela after RotateInviteCode")
	}
}

func TestQuinielaService_RotateInviteCode_NotFound_ReturnsNotFound(t *testing.T) {
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: nil}, &stubMemberRepo{})

	_, err := svc.RotateInviteCode(context.Background(), 99, 10)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("expected ErrNotFound for missing quiniela, got %v", err)
	}
}

func TestQuinielaService_RotateInviteCode_WrongOwner_ReturnsForbidden(t *testing.T) {
	q := &domain.Quiniela{ID: 1, OwnerID: 10}
	svc := NewQuinielaService(&stubQuinielaRepo{quiniela: q}, &stubMemberRepo{})

	// ownerID=99 does not match q.OwnerID=10.
	_, err := svc.RotateInviteCode(context.Background(), 1, 99)
	if !errors.Is(err, apperrors.ErrForbidden) {
		t.Errorf("expected ErrForbidden for non-owner caller, got %v", err)
	}
}

// rotateErrRepo returns a valid quiniela from GetByID but an error from
// RotateInviteCode, isolating the repo failure path inside the service.
type rotateErrRepo struct {
	quiniela  *domain.Quiniela
	rotateErr error
}

func (r *rotateErrRepo) Create(_ context.Context, _ *domain.Quiniela) error { return nil }
func (r *rotateErrRepo) GetByID(_ context.Context, _ int) (*domain.Quiniela, error) {
	return r.quiniela, nil
}
func (r *rotateErrRepo) GetByInviteCode(_ context.Context, _ string) (*domain.Quiniela, error) {
	return nil, nil
}
func (r *rotateErrRepo) Update(_ context.Context, _ *domain.Quiniela) error { return nil }
func (r *rotateErrRepo) Delete(_ context.Context, _ int) error              { return nil }
func (r *rotateErrRepo) ListByOwner(_ context.Context, _ int) ([]*domain.Quiniela, error) {
	return nil, nil
}
func (r *rotateErrRepo) RotateInviteCode(_ context.Context, _ int, _ string, _ *time.Time) (*domain.Quiniela, error) {
	return nil, r.rotateErr
}
func (r *rotateErrRepo) UpdateStatus(_ context.Context, _ int, _ domain.QuinielaStatus) error {
	return nil
}

func TestQuinielaService_RotateInviteCode_RepoError_Propagated(t *testing.T) {
	q := &domain.Quiniela{ID: 1, OwnerID: 10}
	repo := &rotateErrRepo{quiniela: q, rotateErr: errors.New("db write failed")}
	svc := NewQuinielaService(repo, &stubMemberRepo{})

	_, err := svc.RotateInviteCode(context.Background(), 1, 10)
	if err == nil {
		t.Fatal("expected error from repo RotateInviteCode, got nil")
	}
}
