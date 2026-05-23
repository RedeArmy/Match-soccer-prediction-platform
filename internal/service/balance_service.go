package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// BalanceService exposes read-only balance information to users.
//
// All balance mutations go through BankTransferService and WithdrawalService
// so that every change is paired with a ledger row atomically.
type BalanceService interface {
	// GetBalance returns the current balance_cents and reserved_cents for the
	// given user. Returns NotFound for unknown or deleted users.
	GetBalance(ctx context.Context, userID int) (balanceCents, reservedCents int, err error)
	// GetLedger returns ledger entries for userID ordered by created_at DESC.
	GetLedger(ctx context.Context, userID int, p repository.Pagination) ([]*domain.BalanceLedger, error)
}

type balanceService struct {
	userRepo   repository.UserRepository
	ledgerRepo repository.BalanceLedgerRepository
	log        *zap.Logger
}

// NewBalanceService constructs a BalanceService.
func NewBalanceService(
	userRepo repository.UserRepository,
	ledgerRepo repository.BalanceLedgerRepository,
	log *zap.Logger,
) BalanceService {
	return &balanceService{userRepo: userRepo, ledgerRepo: ledgerRepo, log: log}
}

func (s *balanceService) GetBalance(ctx context.Context, userID int) (int, int, error) {
	return s.userRepo.GetBalance(ctx, userID)
}

func (s *balanceService) GetLedger(ctx context.Context, userID int, p repository.Pagination) ([]*domain.BalanceLedger, error) {
	return s.ledgerRepo.ListByUser(ctx, userID, p)
}

var _ BalanceService = (*balanceService)(nil)
