package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ledgerSumStub is a minimal BalanceLedgerRepository whose SumTransactionsByUserAndPeriod
// returns a configurable total. All other methods are no-ops.
type ledgerSumStub struct {
	sum int64
	err error
}

func (s *ledgerSumStub) Credit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (s *ledgerSumStub) CreditIdempotent(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ string) (bool, error) {
	return true, nil
}
func (s *ledgerSumStub) Debit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (s *ledgerSumStub) Reserve(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *ledgerSumStub) ReleaseReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *ledgerSumStub) CommitReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *ledgerSumStub) ListByUser(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return nil, nil
}
func (s *ledgerSumStub) SumTransactionsByUserAndPeriod(_ context.Context, _ int, _ []domain.BalanceLedgerKind, _ time.Time) (int64, error) {
	return s.sum, s.err
}

func newKYCGateWithLedger(tier domain.KYCTier, ledgerSum int64) *kycGate {
	u := &domain.User{ID: 1, KYCTier: tier}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetLedger(&ledgerSumStub{sum: ledgerSum})
	return g
}

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

// ── CheckDepositVelocity ──────────────────────────────────────────────────────

func TestKYCGate_CheckDepositVelocity_NoLedger_Allowed(t *testing.T) {
	gate := newKYCGate(domain.KYCTierUnverified)
	if err := gate.CheckDepositVelocity(context.Background(), 1, 1_000_000); err != nil {
		t.Errorf("expected nil without ledger, got %v", err)
	}
}

func TestKYCGate_CheckDepositVelocity_Tier0_BelowCap_Allowed(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierUnverified, 0)
	if err := gate.CheckDepositVelocity(context.Background(), 1, 1_000); err != nil {
		t.Errorf("expected nil below cap, got %v", err)
	}
}

func TestKYCGate_CheckDepositVelocity_Tier0_ExceedsCap_Blocked(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierUnverified, int64(domain.DefaultKYCTier1DepositVelocityCents))
	err := gate.CheckDepositVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected forbidden when velocity cap exceeded, got %v", err)
	}
}

func TestKYCGate_CheckDepositVelocity_Tier2_BelowCap_Allowed(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierTwo, 0)
	if err := gate.CheckDepositVelocity(context.Background(), 1, 1_000); err != nil {
		t.Errorf("expected nil below tier2 cap, got %v", err)
	}
}

func TestKYCGate_CheckDepositVelocity_Tier3_Unlimited(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierThree, int64(domain.DefaultKYCTier2DepositVelocityCents)*100)
	if err := gate.CheckDepositVelocity(context.Background(), 1, 1_000_000); err != nil {
		t.Errorf("expected nil for Tier 3 (unlimited), got %v", err)
	}
}

func TestKYCGate_CheckDepositVelocity_LedgerError_Propagates(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierOne}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetLedger(&ledgerSumStub{err: errors.New("db fail")})
	if err := g.CheckDepositVelocity(context.Background(), 1, 100); err == nil {
		t.Error("expected ledger error to propagate")
	}
}

// ── CheckWithdrawalVelocity ───────────────────────────────────────────────────

func TestKYCGate_CheckWithdrawalVelocity_NoLedger_Allowed(t *testing.T) {
	gate := newKYCGate(domain.KYCTierTwo)
	if err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1_000); err != nil {
		t.Errorf("expected nil without ledger, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawalVelocity_Tier0_AlwaysBlocked(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierUnverified, 0)
	err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected forbidden for Tier 0 (cap=0), got %v", err)
	}
}

func TestKYCGate_CheckWithdrawalVelocity_Tier2_BelowCap_Allowed(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierTwo, 0)
	if err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1_000); err != nil {
		t.Errorf("expected nil below tier2 cap, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawalVelocity_Tier2_ExceedsCap_Blocked(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierTwo, int64(domain.DefaultKYCTier2WithdrawalVelocityCents))
	err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected forbidden when velocity cap exceeded, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawalVelocity_Tier3_Unlimited(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierThree, int64(domain.DefaultKYCTier2WithdrawalVelocityCents)*100)
	if err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1_000_000); err != nil {
		t.Errorf("expected nil for Tier 3 (unlimited), got %v", err)
	}
}

// ── ExceedsCumulativeAMLThreshold ─────────────────────────────────────────────

func TestKYCGate_ExceedsCumulative_NoLedger_BelowSingle_ReturnsFalse(t *testing.T) {
	gate := newKYCGate(domain.KYCTierTwo)
	exceeds, err := gate.ExceedsCumulativeAMLThreshold(context.Background(), 1, 1_000)
	if err != nil || exceeds {
		t.Errorf("expected false below threshold with no ledger, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsCumulative_SingleTransactionAtThreshold_ReturnsTrue(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierTwo, 0)
	exceeds, err := gate.ExceedsCumulativeAMLThreshold(context.Background(), 1, domain.DefaultKYCAMLThresholdCents)
	if err != nil || !exceeds {
		t.Errorf("expected true at threshold, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsCumulative_RollingWindowPushesOverThreshold_ReturnsTrue(t *testing.T) {
	half := int64(domain.DefaultKYCAMLThresholdCents / 2)
	gate := newKYCGateWithLedger(domain.KYCTierTwo, half)
	exceeds, err := gate.ExceedsCumulativeAMLThreshold(context.Background(), 1, int(half)+1)
	if err != nil || !exceeds {
		t.Errorf("expected true when rolling sum + amount >= threshold, got exceeds=%v err=%v", exceeds, err)
	}
}

func TestKYCGate_ExceedsCumulative_LedgerError_Propagates(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetLedger(&ledgerSumStub{err: errors.New("db fail")})
	_, err := g.ExceedsCumulativeAMLThreshold(context.Background(), 1, 1_000)
	if err == nil {
		t.Error("expected ledger error to propagate")
	}
}

// ── SetMetrics ────────────────────────────────────────────────────────────────

func TestKYCGate_SetMetrics_AcceptsNilAndNonNil(t *testing.T) {
	g := NewKYCGate(&kycUserRepoStub{user: &domain.User{ID: 1}}, &noopSystemParamService{}).(*kycGate)
	g.SetMetrics(nil)
	g.SetMetrics(newTestMetrics(t))
}

// ── CheckWithdrawal with metrics ─────────────────────────────────────────────

func TestKYCGate_CheckWithdrawal_TierInsufficient_WithMetrics(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierUnverified}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetMetrics(newTestMetrics(t))
	err := g.CheckWithdrawal(context.Background(), 1, 1000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for Tier 0 with metrics, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawal_CapExceeded_WithMetrics(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &paramReturning{value: "100000"}).(*kycGate)
	g.SetMetrics(newTestMetrics(t))
	err := g.CheckWithdrawal(context.Background(), 1, 200_000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for cap exceeded with metrics, got %v", err)
	}
}

// ── CheckIPSubmissionVelocity ─────────────────────────────────────────────────

type ipCountStub struct {
	count int64
	err   error
}

func newIPCountGate(count int64, err error) *kycGate {
	g := NewKYCGate(&kycUserRepoStub{user: &domain.User{ID: 1}}, &noopSystemParamService{}).(*kycGate)
	g.SetProfileRepo(&ipCountStub{count: count, err: err})
	return g
}

func (s *ipCountStub) Upsert(_ context.Context, _ *domain.KYCProfile) error { return nil }
func (s *ipCountStub) GetByUserID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *ipCountStub) GetByID(_ context.Context, _ int) (*domain.KYCProfile, error) { return nil, nil }
func (s *ipCountStub) UpdateStatus(_ context.Context, _ int, _ domain.KYCStatus, _ int, _ string) error {
	return nil
}
func (s *ipCountStub) UpdateTier(_ context.Context, _ int, _ domain.KYCTier, _ *time.Time) error {
	return nil
}
func (s *ipCountStub) SetFrozen(_ context.Context, _ int, _ bool, _ int, _ string) error { return nil }
func (s *ipCountStub) ListPending(_ context.Context, _ repository.KYCProfileFilters, _ repository.Pagination) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *ipCountStub) ListFrozen(_ context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return nil, nil
}
func (s *ipCountStub) ListDueForReview(_ context.Context, _ time.Time) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *ipCountStub) CountReviewQueue(_ context.Context) (int64, error)     { return 0, nil }
func (s *ipCountStub) SumFrozenAmountCents(_ context.Context) (int64, error) { return 0, nil }
func (s *ipCountStub) RiskDashboardStats(_ context.Context) (*domain.KYCRiskDashboardStats, error) {
	return &domain.KYCRiskDashboardStats{TierDistribution: map[domain.KYCTier]int64{}}, nil
}
func (s *ipCountStub) ExistsByDocumentIdentity(_ context.Context, _ domain.KYCDocumentType, _ string, _ *time.Time, _ int) (bool, error) {
	return false, nil
}
func (s *ipCountStub) UpdateRiskScore(_ context.Context, _ int, _ int) error { return nil }
func (s *ipCountStub) CountRecentSubmissionsByIP(_ context.Context, _ string, _ time.Time) (int64, error) {
	return s.count, s.err
}
func (s *ipCountStub) ReleaseAndCreditFrozen(_ context.Context, _ int, _ int64, _ string) (int, error) {
	return 0, nil
}
func (s *ipCountStub) ApproveAndSetTier(_ context.Context, _, _ int, _ repository.KYCApprovalParams) error {
	return nil
}
func (s *ipCountStub) FreezeAtomic(_ context.Context, _ int, _ int, _ string, _ string) error {
	return nil
}
func (s *ipCountStub) FreezeAtomicWithTxHook(_ context.Context, _ int, _ int, _ string, _ string, _ func(context.Context, pgx.Tx) error) error {
	return nil
}
func (s *ipCountStub) UpdateStatusWithEvent(_ context.Context, _, _ int, _ repository.KYCStatusEvent) error {
	return nil
}
func (s *ipCountStub) EnsureStub(_ context.Context, _ int) error { return nil }

func isRateLimited(err error) bool {
	var ae *apperrors.AppError
	return errors.As(err, &ae) && ae.Code == apperrors.CodeRateLimited
}

func TestKYCGate_CheckIPSubmissionVelocity_EmptyIP_NoOp(t *testing.T) {
	g := newIPCountGate(999, nil)
	if err := g.CheckIPSubmissionVelocity(context.Background(), ""); err != nil {
		t.Errorf("expected nil for empty IP, got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_NilProfileRepo_NoOp(t *testing.T) {
	g := NewKYCGate(&kycUserRepoStub{user: &domain.User{ID: 1}}, &noopSystemParamService{}).(*kycGate)
	if err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4"); err != nil {
		t.Errorf("expected nil with nil profileRepo, got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_BelowLimit_Allowed(t *testing.T) {
	// count=2, default max=3 → allowed
	g := newIPCountGate(2, nil)
	if err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4"); err != nil {
		t.Errorf("expected nil below limit, got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_AtLimit_RateLimited(t *testing.T) {
	// count=3, default max=3 → blocked
	g := newIPCountGate(3, nil)
	err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4")
	if err == nil || !isRateLimited(err) {
		t.Errorf("expected RateLimited at limit, got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_AboveLimit_RateLimited(t *testing.T) {
	g := newIPCountGate(10, nil)
	err := g.CheckIPSubmissionVelocity(context.Background(), "192.168.1.1")
	if err == nil || !isRateLimited(err) {
		t.Errorf("expected RateLimited above limit, got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_RepoError_Propagates(t *testing.T) {
	g := newIPCountGate(0, errors.New("db fail"))
	err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4")
	if err == nil {
		t.Error("expected repo error to propagate")
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_ZeroMaxParam_DisablesCheck(t *testing.T) {
	// maxSub=0 disables the check entirely
	g := NewKYCGate(&kycUserRepoStub{user: &domain.User{ID: 1}}, &paramReturning{value: "0"}).(*kycGate)
	g.SetProfileRepo(&ipCountStub{count: 99, err: nil})
	if err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4"); err != nil {
		t.Errorf("expected nil when max_submissions=0 (disabled), got %v", err)
	}
}

func TestKYCGate_CheckIPSubmissionVelocity_WithMetrics_RecordsFraudFlag(t *testing.T) {
	g := newIPCountGate(10, nil)
	g.SetMetrics(newTestMetrics(t))
	err := g.CheckIPSubmissionVelocity(context.Background(), "1.2.3.4")
	if err == nil || !isRateLimited(err) {
		t.Errorf("expected RateLimited with metrics, got %v", err)
	}
}

func TestKYCGate_SetProfileRepo_AcceptsNilAndNonNil(t *testing.T) {
	g := NewKYCGate(&kycUserRepoStub{user: &domain.User{ID: 1}}, &noopSystemParamService{}).(*kycGate)
	g.SetProfileRepo(nil)
	g.SetProfileRepo(&ipCountStub{})
}

// ── CheckDeposit with metrics ─────────────────────────────────────────────────

func TestKYCGate_CheckDeposit_CapExceeded_WithMetrics(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierUnverified}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &paramReturning{value: "50000"}).(*kycGate)
	g.SetMetrics(newTestMetrics(t))
	err := g.CheckDeposit(context.Background(), 1, 100_000)
	if err == nil || !isForbidden(err) {
		t.Fatalf("expected Forbidden for deposit cap exceeded with metrics, got %v", err)
	}
}

// ── intParam ──────────────────────────────────────────────────────────────────

func TestKYCGate_intParam_UnparsableValue_ReturnsDefault(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierUnverified}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &paramReturning{value: "not-a-number"}).(*kycGate)
	// CheckDeposit calls intParam; with an unparseable param the gate falls back
	// to the default cap, so a small deposit within that default must pass.
	if err := g.CheckDeposit(context.Background(), 1, 1_000); err != nil {
		t.Errorf("expected nil using default cap when param is unparseable, got %v", err)
	}
}

// ── CheckWithdrawalVelocity with ledger and metrics ───────────────────────────

func TestKYCGate_CheckWithdrawalVelocity_LedgerError_Propagates(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierTwo}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetLedger(&ledgerSumStub{err: errors.New("db fail")})
	if err := g.CheckWithdrawalVelocity(context.Background(), 1, 1_000); err == nil {
		t.Error("expected ledger error to propagate")
	}
}

func TestKYCGate_CheckWithdrawalVelocity_NoAllowance_WithMetrics(t *testing.T) {
	u := &domain.User{ID: 1, KYCTier: domain.KYCTierOne}
	g := NewKYCGate(&kycUserRepoStub{user: u}, &noopSystemParamService{}).(*kycGate)
	g.SetLedger(&ledgerSumStub{sum: 0})
	g.SetMetrics(newTestMetrics(t))
	err := g.CheckWithdrawalVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected Forbidden for Tier 1 with no withdrawal allowance, got %v", err)
	}
}

func TestKYCGate_CheckWithdrawalVelocity_VelocityExceeded_WithMetrics(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierTwo, int64(domain.DefaultKYCTier2WithdrawalVelocityCents))
	gate.SetMetrics(newTestMetrics(t))
	err := gate.CheckWithdrawalVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected Forbidden when velocity exceeded with metrics, got %v", err)
	}
}

// ── CheckDepositVelocity with metrics ─────────────────────────────────────────

func TestKYCGate_CheckDepositVelocity_VelocityExceeded_WithMetrics(t *testing.T) {
	gate := newKYCGateWithLedger(domain.KYCTierUnverified, int64(domain.DefaultKYCTier1DepositVelocityCents))
	gate.SetMetrics(newTestMetrics(t))
	err := gate.CheckDepositVelocity(context.Background(), 1, 1)
	if err == nil || !isForbidden(err) {
		t.Errorf("expected Forbidden when deposit velocity exceeded with metrics, got %v", err)
	}
}
