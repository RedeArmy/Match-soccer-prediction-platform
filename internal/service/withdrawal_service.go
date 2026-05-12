package service

import (
	"context"
	"strconv"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

type withdrawalService struct {
	withdrawalRepo repository.WithdrawalRequestRepository
	paramRepo      repository.SystemParamRepository
	audit          AuditLogger
	log            *zap.Logger
}

// NewWithdrawalService constructs a WithdrawalService.
func NewWithdrawalService(
	withdrawalRepo repository.WithdrawalRequestRepository,
	paramRepo repository.SystemParamRepository,
	audit AuditLogger,
	log *zap.Logger,
) WithdrawalService {
	return &withdrawalService{
		withdrawalRepo: withdrawalRepo,
		paramRepo:      paramRepo,
		audit:          audit,
		log:            log,
	}
}

func (s *withdrawalService) Create(ctx context.Context, userID, amountCents int, currency string, method domain.WithdrawalMethod, payoutDetails map[string]string) (*domain.WithdrawalRequest, error) {
	minCents, maxCents, err := s.withdrawalLimits(ctx)
	if err != nil {
		return nil, err
	}
	if amountCents < minCents {
		return nil, apperrors.Validation("withdrawal amount below minimum")
	}
	if amountCents > maxCents {
		return nil, apperrors.Validation("withdrawal amount above maximum")
	}
	if currency == "" {
		currency = "GTQ"
	}

	req := &domain.WithdrawalRequest{
		UserID:        userID,
		AmountCents:   amountCents,
		Currency:      currency,
		Method:        method,
		PayoutDetails: payoutDetails,
	}
	if err := s.withdrawalRepo.CreateAndReserve(ctx, req); err != nil {
		return nil, err
	}

	resType := "withdrawal_request"
	reqID := int(req.ID)
	s.audit.Log(ctx, &userID, nil, domain.AuditActionWithdrawalRequested, &resType, &reqID, map[string]any{
		"amount_cents": amountCents,
		"currency":     currency,
		"method":       string(method),
	})
	return req, nil
}

func (s *withdrawalService) GetByID(ctx context.Context, id int) (*domain.WithdrawalRequest, error) {
	return s.withdrawalRepo.GetByID(ctx, id)
}

func (s *withdrawalService) ListByUser(ctx context.Context, userID int) ([]*domain.WithdrawalRequest, error) {
	return s.withdrawalRepo.ListByUser(ctx, userID)
}

func (s *withdrawalService) ListPending(ctx context.Context) ([]*domain.WithdrawalRequest, error) {
	return s.withdrawalRepo.ListPending(ctx)
}

func (s *withdrawalService) ApproveRequest(ctx context.Context, requestID, adminID int, notes string) (*domain.WithdrawalRequest, error) {
	req, err := s.withdrawalRepo.Approve(ctx, requestID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "withdrawal_request"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionWithdrawalApproved, &resType, &requestID, map[string]any{
		"notes": notes,
	})
	return req, nil
}

func (s *withdrawalService) RejectRequest(ctx context.Context, requestID, adminID int, notes string) (*domain.WithdrawalRequest, error) {
	req, err := s.withdrawalRepo.RejectAndRelease(ctx, requestID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "withdrawal_request"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionWithdrawalRejected, &resType, &requestID, map[string]any{
		"notes": notes,
	})
	return req, nil
}

func (s *withdrawalService) ProcessWithdrawal(ctx context.Context, requestID, adminID int) (*domain.WithdrawalRequest, error) {
	req, err := s.withdrawalRepo.MarkProcessedAndCommit(ctx, requestID)
	if err != nil {
		return nil, err
	}

	resType := "withdrawal_request"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionWithdrawalRejected, &resType, &requestID, nil)
	return req, nil
}

func (s *withdrawalService) withdrawalLimits(ctx context.Context) (minCents, maxCents int, err error) {
	minCents = domain.DefaultWithdrawalMinCents
	maxCents = domain.DefaultWithdrawalMaxCents

	if p, err := s.paramRepo.Get(ctx, domain.ParamKeyWithdrawalMinCents); err == nil && p != nil {
		if v, err := strconv.Atoi(p.Value); err == nil {
			minCents = v
		}
	}
	if p, err := s.paramRepo.Get(ctx, domain.ParamKeyWithdrawalMaxCents); err == nil && p != nil {
		if v, err := strconv.Atoi(p.Value); err == nil {
			maxCents = v
		}
	}
	return minCents, maxCents, nil
}

var _ WithdrawalService = (*withdrawalService)(nil)
