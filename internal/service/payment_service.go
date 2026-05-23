package service

import (
	"context"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// PaymentService manages entry-fee payment records for quiniela groups.
//
// CreateRecord creates a pending record that must later be confirmed by an
// admin via ValidateDeposit. RejectDeposit marks a pending payment as denied
// without capturing funds. Only an admin may call Validate or Reject; the
// caller's identity is enforced at the HTTP layer via RequireRole.
type PaymentService interface {
	CreateRecord(ctx context.Context, quinielaID, userID, amount int, currency, reference string) (*domain.PaymentRecord, error)
	ValidateDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error)
	RejectDeposit(ctx context.Context, paymentID, adminID int, notes string) (*domain.PaymentRecord, error)
	ListPending(ctx context.Context) ([]*domain.PaymentRecord, error)
	ListByQuiniela(ctx context.Context, quinielaID int) ([]*domain.PaymentRecord, error)
	// List returns all payment records matching the given filters with pagination.
	List(ctx context.Context, f repository.PaymentFilters, p repository.Pagination) ([]*domain.PaymentRecord, error)
}

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
	// ValidateAndMarkPaid atomically confirms the payment and flips
	// group_memberships.paid=true in one transaction, eliminating the window
	// where the payment is confirmed but the member is still marked unpaid.
	record, err := s.paymentRepo.ValidateAndMarkPaid(ctx, paymentID, adminID, notes)
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

func (s *paymentService) List(ctx context.Context, f repository.PaymentFilters, p repository.Pagination) ([]*domain.PaymentRecord, error) {
	return s.paymentRepo.List(ctx, f, p)
}

var _ PaymentService = (*paymentService)(nil)
