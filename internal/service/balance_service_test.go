package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type balanceSvcUserRepo struct {
	balance     int
	reserved    int
	balCurrency string
	currencyErr error
	err         error
}

func (r *balanceSvcUserRepo) Create(_ context.Context, _ *domain.User) error { return r.err }
func (r *balanceSvcUserRepo) GetByID(_ context.Context, _ int) (*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) GetByExternalSubject(_ context.Context, _ string) (*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) GetByEmail(_ context.Context, _ string) (*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) Update(_ context.Context, _ *domain.User) error { return r.err }
func (r *balanceSvcUserRepo) Delete(_ context.Context, _ int) error          { return r.err }
func (r *balanceSvcUserRepo) List(_ context.Context) ([]*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) ListByIDs(_ context.Context, _ []int) ([]*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) Ban(_ context.Context, _, _ int, _ string) (*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) Unban(_ context.Context, _ int) error { return r.err }
func (r *balanceSvcUserRepo) ListBanned(_ context.Context) ([]*domain.User, error) {
	return nil, r.err
}
func (r *balanceSvcUserRepo) ListFiltered(_ context.Context, _ repository.UserFilters, _ repository.CursorPage) ([]*domain.User, string, error) {
	return nil, "", r.err
}
func (r *balanceSvcUserRepo) GetStatusCounts(_ context.Context) (repository.UserStatusCounts, error) {
	return repository.UserStatusCounts{}, r.err
}
func (r *balanceSvcUserRepo) GetBalance(_ context.Context, _ int) (int, int, error) {
	return r.balance, r.reserved, r.err
}
func (r *balanceSvcUserRepo) GetBalanceCurrency(_ context.Context, _ int) (string, error) {
	if r.currencyErr != nil {
		return "", r.currencyErr
	}
	if r.balCurrency != "" {
		return r.balCurrency, nil
	}
	return "GTQ", nil
}
func (r *balanceSvcUserRepo) UpdateLocale(_ context.Context, _ int, _ string) error { return r.err }
func (r *balanceSvcUserRepo) SetRole(_ context.Context, _ int, _ domain.UserRole) (*domain.User, error) {
	return nil, r.err
}

type balanceLedgerRepoStub struct {
	entries []*domain.BalanceLedger
	err     error
}

func (r *balanceLedgerRepoStub) Credit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return r.err
}
func (r *balanceLedgerRepoStub) Debit(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ int64, _ string, _ int) error {
	return r.err
}
func (r *balanceLedgerRepoStub) Reserve(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return r.err
}
func (r *balanceLedgerRepoStub) ReleaseReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return r.err
}
func (r *balanceLedgerRepoStub) CommitReservation(_ context.Context, _ int, _ int, _ int64, _ string, _ int) error {
	return r.err
}
func (r *balanceLedgerRepoStub) ListByUser(_ context.Context, _ int, _ repository.Pagination) ([]*domain.BalanceLedger, error) {
	return r.entries, r.err
}
func (r *balanceLedgerRepoStub) CreditIdempotent(_ context.Context, _ int, _ int, _ domain.BalanceLedgerKind, _ string, _ string, _ int) (bool, error) {
	return true, nil
}
func (r *balanceLedgerRepoStub) SumTransactionsByUserAndPeriod(_ context.Context, _ int, _ []domain.BalanceLedgerKind, _ time.Time) (int64, error) {
	return 0, r.err
}
func (r *balanceLedgerRepoStub) CountRows(_ context.Context) (int64, error) { return 0, nil }

func newBalanceSvc(ur *balanceSvcUserRepo, lr *balanceLedgerRepoStub) BalanceService {
	return NewBalanceService(ur, lr, zap.NewNop())
}

// ── GetBalance ────────────────────────────────────────────────────────────────

func TestBalanceService_GetBalance_ReturnsBothColumns(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{balance: 10000, reserved: 2000}, &balanceLedgerRepoStub{})

	bal, res, err := svc.GetBalance(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal != 10000 {
		t.Errorf("balance: got %d, want 10000", bal)
	}
	if res != 2000 {
		t.Errorf("reserved: got %d, want 2000", res)
	}
}

func TestBalanceService_GetBalance_ZeroBalance(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{balance: 0, reserved: 0}, &balanceLedgerRepoStub{})

	bal, res, err := svc.GetBalance(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal != 0 || res != 0 {
		t.Errorf("expected 0/0, got %d/%d", bal, res)
	}
}

func TestBalanceService_GetBalance_PropagatesRepoError(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{err: errors.New("db down")}, &balanceLedgerRepoStub{})

	_, _, err := svc.GetBalance(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── GetLedger ─────────────────────────────────────────────────────────────────

func TestBalanceService_GetLedger_ReturnsEntries(t *testing.T) {
	entries := []*domain.BalanceLedger{
		{ID: 1, UserID: 5, DeltaCents: 5000, Kind: domain.LedgerKindBankTransfer},
		{ID: 2, UserID: 5, DeltaCents: -1000, Kind: domain.LedgerKindEntryFee},
	}
	svc := newBalanceSvc(&balanceSvcUserRepo{}, &balanceLedgerRepoStub{entries: entries})

	got, err := svc.GetLedger(context.Background(), 5, repository.Pagination{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 entries, got %d", len(got))
	}
}

func TestBalanceService_GetLedger_EmptyReturnsNil(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{}, &balanceLedgerRepoStub{entries: nil})

	got, err := svc.GetLedger(context.Background(), 5, repository.Pagination{Limit: 50})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(got))
	}
}

func TestBalanceService_GetLedger_PropagatesRepoError(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{}, &balanceLedgerRepoStub{err: errors.New("db error")})

	_, err := svc.GetLedger(context.Background(), 5, repository.Pagination{Limit: 50})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── GetBalanceWithCurrency ─────────────────────────────────────────────────────

func TestBalanceService_GetBalanceWithCurrency_ReturnsBalanceAndGTQ(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{balance: 10000, reserved: 500}, &balanceLedgerRepoStub{})

	bal, res, cur, err := svc.GetBalanceWithCurrency(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if bal != 10000 {
		t.Errorf("balance: got %d, want 10000", bal)
	}
	if res != 500 {
		t.Errorf("reserved: got %d, want 500", res)
	}
	if cur != "GTQ" {
		t.Errorf("currency: got %q, want GTQ", cur)
	}
}

func TestBalanceService_GetBalanceWithCurrency_ReturnsUSDCurrency(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{balance: 500, balCurrency: "USD"}, &balanceLedgerRepoStub{})

	_, _, cur, err := svc.GetBalanceWithCurrency(context.Background(), 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cur != "USD" {
		t.Errorf("currency: got %q, want USD", cur)
	}
}

func TestBalanceService_GetBalanceWithCurrency_PropagatesGetBalanceError(t *testing.T) {
	svc := newBalanceSvc(&balanceSvcUserRepo{err: errors.New("db down")}, &balanceLedgerRepoStub{})

	_, _, _, err := svc.GetBalanceWithCurrency(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from GetBalance, got nil")
	}
}

func TestBalanceService_GetBalanceWithCurrency_PropagatesGetBalanceCurrencyError(t *testing.T) {
	svc := newBalanceSvc(
		&balanceSvcUserRepo{balance: 100, currencyErr: errors.New("currency query failed")},
		&balanceLedgerRepoStub{},
	)

	_, _, _, err := svc.GetBalanceWithCurrency(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error from GetBalanceCurrency, got nil")
	}
}
