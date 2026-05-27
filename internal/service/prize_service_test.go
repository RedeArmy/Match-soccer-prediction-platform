package service

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type prizeLedgerStub struct {
	credited    bool
	creditedAmt int
}

func (s *prizeLedgerStub) Credit(_ context.Context, _ int, amount int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	s.credited = true
	s.creditedAmt = amount
	return nil
}
func (s *prizeLedgerStub) CreditIdempotent(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ string) (bool, error) {
	return false, nil
}
func (s *prizeLedgerStub) Debit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return nil
}
func (s *prizeLedgerStub) Reserve(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *prizeLedgerStub) ReleaseReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *prizeLedgerStub) CommitReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return nil
}
func (s *prizeLedgerStub) ListByUser(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return nil, nil
}
func (s *prizeLedgerStub) SumTransactionsByUserAndPeriod(_ context.Context, _ int, _ []domain.BalanceLedgerKind, _ time.Time) (int64, error) {
	return 0, nil
}

// prizeKYCGateStub controls whether freeze is triggered.
type prizeKYCGateStub struct {
	shouldFreeze bool
}

func (g *prizeKYCGateStub) CheckWithdrawal(_ context.Context, _, _ int) error { return nil }
func (g *prizeKYCGateStub) CheckDeposit(_ context.Context, _, _ int) error    { return nil }
func (g *prizeKYCGateStub) CheckWinFreeze(_ context.Context, _, _ int) (bool, string, error) {
	return g.shouldFreeze, "KYC required", nil
}
func (g *prizeKYCGateStub) ExceedsAMLThreshold(_ context.Context, _ int) (bool, error) {
	return false, nil
}
func (g *prizeKYCGateStub) ExceedsCumulativeAMLThreshold(_ context.Context, _, _ int) (bool, error) {
	return false, nil
}
func (g *prizeKYCGateStub) CheckDepositVelocity(_ context.Context, _, _ int) error    { return nil }
func (g *prizeKYCGateStub) CheckWithdrawalVelocity(_ context.Context, _, _ int) error { return nil }

type prizeKYCSvcStub struct {
	frozen   bool
	frozeAmt int
}

func (s *prizeKYCSvcStub) FreezeBalance(_ context.Context, _ int, amountCents int, _ string) error {
	s.frozen = true
	s.frozeAmt = amountCents
	return nil
}
func (s *prizeKYCSvcStub) GetProfile(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) Submit(_ context.Context, _ int, _ SubmitKYCRequest) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) UploadDocument(_ context.Context, _ int, _ UploadDocRequest) (*domain.KYCDocument, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) ListDocuments(_ context.Context, _ int) ([]*domain.KYCDocument, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) GetRequirements(_ context.Context, _ int) (*KYCRequirements, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) ListEvents(_ context.Context, _ int, _ domain.KYCProfileType, _ repository.CursorPage) ([]*domain.KYCEvent, string, error) {
	return nil, "", nil
}
func (s *prizeKYCSvcStub) ListQueue(_ context.Context, _ repository.KYCProfileFilters, _ repository.Pagination) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) GetProfileByID(_ context.Context, _ int) (*domain.KYCProfile, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) Approve(_ context.Context, _, _ int, _ domain.KYCTier) error { return nil }
func (s *prizeKYCSvcStub) Reject(_ context.Context, _, _ int, _ string) error          { return nil }
func (s *prizeKYCSvcStub) Escalate(_ context.Context, _, _ int, _ string) error        { return nil }
func (s *prizeKYCSvcStub) RequestDocument(_ context.Context, _, _ int, _ domain.KYCDocumentType, _ string) error {
	return nil
}
func (s *prizeKYCSvcStub) VerifyDocument(_ context.Context, _ int64, _ int) error { return nil }
func (s *prizeKYCSvcStub) ListFrozenBalances(_ context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) ReleaseFrozenBalance(_ context.Context, _, _ int) error { return nil }
func (s *prizeKYCSvcStub) ListDueForReview(_ context.Context) ([]*domain.KYCProfile, error) {
	return nil, nil
}
func (s *prizeKYCSvcStub) GetRiskDashboard(_ context.Context) (*domain.KYCRiskDashboardStats, error) {
	return &domain.KYCRiskDashboardStats{TierDistribution: map[domain.KYCTier]int64{}}, nil
}
func (s *prizeKYCSvcStub) RecalculateRiskScore(_ context.Context, _ int) (int, error) { return 0, nil }

type prizeNotifierStub struct {
	fired   bool
	userID  int
	amount  int
	traceID string
}

func (n *prizeNotifierStub) NotifyKYCWinnerFreeze(_ context.Context, userID, amount int, traceID string) {
	n.fired = true
	n.userID = userID
	n.amount = amount
	n.traceID = traceID
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestPrizeService_CreditPrize_AboveThreshold_Freezes(t *testing.T) {
	ledger := &prizeLedgerStub{}
	gate := &prizeKYCGateStub{shouldFreeze: true}
	kycSvc := &prizeKYCSvcStub{}
	notifier := &prizeNotifierStub{}

	svc := NewPrizeService(ledger, gate, kycSvc, notifier, zap.NewNop())
	credited, err := svc.CreditPrize(context.Background(), 42, 500_000, 1, "quiniela")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if credited {
		t.Error("expected credited=false when freeze is triggered")
	}
	if !kycSvc.frozen {
		t.Error("FreezeBalance was not called")
	}
	if kycSvc.frozeAmt != 500_000 {
		t.Errorf("frozen amount: got %d, want 500000", kycSvc.frozeAmt)
	}
	if ledger.credited {
		t.Error("ledger was credited despite freeze being triggered")
	}
	if !notifier.fired {
		t.Error("KYCWinnerFreeze notification was not fired")
	}
	if notifier.amount != 500_000 {
		t.Errorf("notification amount: got %d, want 500000", notifier.amount)
	}
}

func TestPrizeService_CreditPrize_BelowThreshold_Credits(t *testing.T) {
	ledger := &prizeLedgerStub{}
	gate := &prizeKYCGateStub{shouldFreeze: false}
	kycSvc := &prizeKYCSvcStub{}
	notifier := &prizeNotifierStub{}

	svc := NewPrizeService(ledger, gate, kycSvc, notifier, zap.NewNop())
	credited, err := svc.CreditPrize(context.Background(), 42, 5_000, 1, "quiniela")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !credited {
		t.Error("expected credited=true when no freeze is triggered")
	}
	if !ledger.credited {
		t.Error("ledger.Credit was not called")
	}
	if ledger.creditedAmt != 5_000 {
		t.Errorf("ledger amount: got %d, want 5000", ledger.creditedAmt)
	}
	if kycSvc.frozen {
		t.Error("FreezeBalance was called unexpectedly")
	}
	if notifier.fired {
		t.Error("notifier was fired unexpectedly for non-frozen prize")
	}
}
