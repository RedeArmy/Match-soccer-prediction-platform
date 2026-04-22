package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// paymentService is the concrete implementation of PaymentService.
type paymentService struct {
	paymentRepo repository.PaymentRecordRepository
	audit       AuditLogger
	log         *zap.Logger
}

// NewPaymentService constructs a paymentService.
func NewPaymentService(
	paymentRepo repository.PaymentRecordRepository,
	audit AuditLogger,
	log *zap.Logger,
) PaymentService {
	return &paymentService{paymentRepo: paymentRepo, audit: audit, log: log}
}

func (s *paymentService) CreateRecord(ctx context.Context, quinielaID, userID, amount int, currency, reference string) (*domain.PaymentRecord, error) {
	record := &domain.PaymentRecord{
		QuinielaID: quinielaID,
		UserID:     userID,
		Amount:     amount,
		Currency:   currency,
		Reference:  &reference,
	}
	if err := s.paymentRepo.Create(ctx, record); err != nil {
		return nil, err
	}

	resType := "payment_record"
	s.audit.Log(ctx, &userID, nil, domain.AuditActionPaymentCreated, &resType, &record.ID, map[string]any{
		"quiniela_id": quinielaID,
		"amount":      amount,
		"currency":    currency,
	})
	return record, nil
}

func (s *paymentService) ValidateDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error) {
	record, err := s.paymentRepo.Validate(ctx, paymentID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "payment_record"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionPaymentValidated, &resType, &paymentID, map[string]any{
		"notes": notes,
	})
	return record, nil
}

func (s *paymentService) RejectDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error) {
	record, err := s.paymentRepo.Reject(ctx, paymentID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "payment_record"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionPaymentRejected, &resType, &paymentID, map[string]any{
		"notes": notes,
	})
	return record, nil
}

func (s *paymentService) ListPending(ctx context.Context) ([]*domain.PaymentRecord, error) {
	return s.paymentRepo.ListPending(ctx)
}

func (s *paymentService) ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.PaymentRecord, error) {
	return s.paymentRepo.ListByQuiniela(ctx, quinielaID, repository.PaymentFilters{})
}

var _ PaymentService = (*paymentService)(nil)
