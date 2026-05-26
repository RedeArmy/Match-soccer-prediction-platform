package service

import (
	"context"
	"errors"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type kycUserRepoStub struct {
	user *domain.User
	err  error
}

func (r *kycUserRepoStub) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return r.user, r.err
}
func (r *kycUserRepoStub) GetByClerkSubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *kycUserRepoStub) Create(_ context.Context, _ *domain.User) error { return nil }
func (r *kycUserRepoStub) Update(_ context.Context, _ *domain.User) error { return nil }
func (r *kycUserRepoStub) Delete(_ context.Context, _ int) error          { return nil }
func (r *kycUserRepoStub) List(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *kycUserRepoStub) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, nil
}
func (r *kycUserRepoStub) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, nil
}
func (r *kycUserRepoStub) Unban(_ context.Context, _ int) error                 { return nil }
func (r *kycUserRepoStub) ListBanned(_ context.Context) ([]*domain.User, error) { return nil, nil }
func (r *kycUserRepoStub) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", nil
}
func (r *kycUserRepoStub) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, nil
}
func (r *kycUserRepoStub) GetBalance(_ context.Context, _ int) (int, int, error) { return 0, 0, nil }

// paramReturning returns a SystemParamService whose Get returns the given value.
type paramReturning struct {
	noopSystemParamService
	value string
}

func (p *paramReturning) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return &domain.SystemParam{Value: p.value}, nil
}

func isForbidden(err error) bool {
	var ae *apperrors.AppError
	return errors.As(err, &ae) && ae.Code == apperrors.CodeForbidden
}

func newKYCGate(tier domain.KYCTier) KYCGate {
	u := &domain.User{ID: 1, KYCTier: tier}
	return NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{})
}

func newKYCGateWithParam(tier domain.KYCTier, paramVal string) KYCGate {
	u := &domain.User{ID: 1, KYCTier: tier}
	return NewKYCGate(&kycUserRepoStub{user: u}, &paramReturning{value: paramVal})
}

// ── CheckWithdrawal ───────────────────────────────────────────────────────────

func TestKYCGate_CheckWithdrawal_Tier0_Blocked(t *testing.T) {
	err := newKYCGate(domain.KYCTierUnverified).CheckWithdrawal(context.Background(), 1, 1000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for Tier 0, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_Tier1_Blocked(t *testing.T) {
	err := newKYCGate(domain.KYCTierOne).CheckWithdrawal(context.Background(), 1, 1000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for Tier 1, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_Tier2_BelowCap_Allowed(t *testing.T) {
	// default cap = Q15,000; withdraw Q5,000
	if err := newKYCGate(domain.KYCTierTwo).CheckWithdrawal(context.Background(), 1, 500_000); err != nil {
		t.Fatalf("expected nil for Tier 2 within cap, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_Tier2_ExceedsCap_Blocked(t *testing.T) {
	gate := newKYCGateWithParam(domain.KYCTierTwo, "100000")
	err := gate.CheckWithdrawal(context.Background(), 1, 200_000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden when Tier 2 exceeds cap, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_Tier3_Unlimited(t *testing.T) {
	if err := newKYCGate(domain.KYCTierThree).CheckWithdrawal(context.Background(), 1, 99_999_999); err != nil {
		t.Fatalf("expected nil for Tier 3, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_UserNotFound_ReturnsError(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{user: nil}, &noopSystemParamService{})
	if err := gate.CheckWithdrawal(context.Background(), 999, 1000); err == nil {
		t.Fatal("expected error when user not found, got nil")
	}
}

func TestKYCGate_CheckWithdrawal_RepoError_Propagates(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{err: errors.New("db down")}, &noopSystemParamService{})
	if err := gate.CheckWithdrawal(context.Background(), 1, 1000); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── CheckDeposit ──────────────────────────────────────────────────────────────

func TestKYCGate_CheckDeposit_Tier0_BelowCap_Allowed(t *testing.T) {
	if err := newKYCGate(domain.KYCTierUnverified).CheckDeposit(context.Background(), 1, 100_000); err != nil {
		t.Fatalf("expected nil for Tier 0 within cap, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_Tier0_ExceedsCap_Blocked(t *testing.T) {
	gate := newKYCGateWithParam(domain.KYCTierUnverified, "50000")
	err := gate.CheckDeposit(context.Background(), 1, 100_000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for Tier 0 exceeding cap, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_Tier1_BelowCap_Allowed(t *testing.T) {
	if err := newKYCGate(domain.KYCTierOne).CheckDeposit(context.Background(), 1, 100_000); err != nil {
		t.Fatalf("expected nil for Tier 1 within cap, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_Tier2_BelowCap_Allowed(t *testing.T) {
	if err := newKYCGate(domain.KYCTierTwo).CheckDeposit(context.Background(), 1, 500_000); err != nil {
		t.Fatalf("expected nil for Tier 2 within cap, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_Tier2_ExceedsCap_Blocked(t *testing.T) {
	gate := newKYCGateWithParam(domain.KYCTierTwo, "100000")
	err := gate.CheckDeposit(context.Background(), 1, 200_000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for Tier 2 exceeding deposit cap, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_Tier3_Unlimited(t *testing.T) {
	if err := newKYCGate(domain.KYCTierThree).CheckDeposit(context.Background(), 1, 99_999_999); err != nil {
		t.Fatalf("expected nil for Tier 3, got %v", err)
	}
}

func TestKYCGate_CheckDeposit_RepoError_Propagates(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{err: errors.New("db down")}, &noopSystemParamService{})
	if err := gate.CheckDeposit(context.Background(), 1, 1000); err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── CheckWinFreeze ────────────────────────────────────────────────────────────

func TestKYCGate_CheckWinFreeze_Tier0_AlwaysFreezes(t *testing.T) {
	freeze, reason, err := newKYCGate(domain.KYCTierUnverified).CheckWinFreeze(context.Background(), 1, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !freeze || reason == "" {
		t.Errorf("expected freeze=true + reason for Tier 0, got freeze=%v reason=%q", freeze, reason)
	}
}

func TestKYCGate_CheckWinFreeze_Tier1_AlwaysFreezes(t *testing.T) {
	freeze, _, err := newKYCGate(domain.KYCTierOne).CheckWinFreeze(context.Background(), 1, 500)
	if err != nil || !freeze {
		t.Errorf("expected freeze=true for Tier 1, got freeze=%v err=%v", freeze, err)
	}
}

func TestKYCGate_CheckWinFreeze_Tier2_NeverFreezes(t *testing.T) {
	freeze, _, err := newKYCGate(domain.KYCTierTwo).CheckWinFreeze(context.Background(), 1, 999_999_999)
	if err != nil || freeze {
		t.Errorf("expected freeze=false for Tier 2, got freeze=%v err=%v", freeze, err)
	}
}

func TestKYCGate_CheckWinFreeze_Tier3_NeverFreezes(t *testing.T) {
	freeze, _, err := newKYCGate(domain.KYCTierThree).CheckWinFreeze(context.Background(), 1, 999_999_999)
	if err != nil || freeze {
		t.Errorf("expected freeze=false for Tier 3, got freeze=%v err=%v", freeze, err)
	}
}

func TestKYCGate_CheckWinFreeze_RepoError_Propagates(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{err: errors.New("db down")}, &noopSystemParamService{})
	_, _, err := gate.CheckWinFreeze(context.Background(), 1, 1000)
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

// ── ExceedsAMLThreshold ───────────────────────────────────────────────────────

func TestKYCGate_ExceedsAMLThreshold_BelowDefault_ReturnsFalse(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{}, &noopSystemParamService{})
	exceeds, err := gate.ExceedsAMLThreshold(context.Background(), 1_000_000)
	if err != nil || exceeds {
		t.Errorf("expected false below default threshold, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsAMLThreshold_AtThreshold_ReturnsTrue(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{}, &noopSystemParamService{})
	exceeds, err := gate.ExceedsAMLThreshold(context.Background(), domain.DefaultKYCAMLThresholdCents)
	if err != nil || !exceeds {
		t.Errorf("expected true at default threshold, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsAMLThreshold_CustomThreshold(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{}, &paramReturning{value: "100000"})
	exceeds, err := gate.ExceedsAMLThreshold(context.Background(), 100_000)
	if err != nil || !exceeds {
		t.Errorf("expected true at custom threshold, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsAMLThreshold_AboveDefault_ReturnsTrue(t *testing.T) {
	gate := NewKYCGate(&kycUserRepoStub{}, &noopSystemParamService{})
	exceeds, err := gate.ExceedsAMLThreshold(context.Background(), domain.DefaultKYCAMLThresholdCents+1)
	if err != nil || !exceeds {
		t.Errorf("expected true above threshold, got exceeds=%v err=%v", exceeds, err)
	}
}
