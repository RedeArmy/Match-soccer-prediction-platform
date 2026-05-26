package service

import (
	"context"
	"strconv"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// WithdrawalService manages the full payout request lifecycle.
//
// Create reserves the requested amount from the user's available balance.
// Approve advances the status without a balance change.
// RejectRequest releases the reserved balance back to available.
// ProcessWithdrawal commits the reserved amount as permanently paid out.
type WithdrawalService interface {
	// Create creates a withdrawal request and reserves the balance.
	// Returns Conflict when available balance is insufficient.
	Create(ctx context.Context, userID, amountCents int, currency string, method domain.WithdrawalMethod, payoutDetails map[string]string) (*domain.WithdrawalRequest, error)
	// GetByID returns the request or nil when not found.
	GetByID(ctx context.Context, id int) (*domain.WithdrawalRequest, error)
	// ListByUser returns all requests for a user.
	ListByUser(ctx context.Context, userID int) ([]*domain.WithdrawalRequest, error)
	// ListPending returns all pending requests for admin review.
	ListPending(ctx context.Context) ([]*domain.WithdrawalRequest, error)
	// ApproveRequest transitions the request to approved (admin queue step).
	ApproveRequest(ctx context.Context, requestID, adminID int, notes string) (*domain.WithdrawalRequest, error)
	// RejectRequest transitions the request to rejected and releases the
	// reserved balance.
	RejectRequest(ctx context.Context, requestID, adminID int, notes string) (*domain.WithdrawalRequest, error)
	// ProcessWithdrawal transitions an approved request to processed and
	// commits the reserved amount as paid out.
	ProcessWithdrawal(ctx context.Context, requestID, adminID int) (*domain.WithdrawalRequest, error)
}

type withdrawalService struct {
	withdrawalRepo repository.WithdrawalRequestRepository
	paramRepo      repository.SystemParamRepository
	kycGate        KYCGate
	outboxWriter   outbox.Writer
	audit          AuditLogger
	log            *zap.Logger
}

// NewWithdrawalService constructs a WithdrawalService.
// kycGate enforces identity-verification requirements before any payout is
// created. Pass NoopKYCGate{} in tests or environments where enforcement is
// intentionally disabled.
func NewWithdrawalService(
	withdrawalRepo repository.WithdrawalRequestRepository,
	paramRepo repository.SystemParamRepository,
	kycGate KYCGate,
	outboxWriter outbox.Writer,
	audit AuditLogger,
	log *zap.Logger,
) WithdrawalService {
	return &withdrawalService{
		withdrawalRepo: withdrawalRepo,
		paramRepo:      paramRepo,
		kycGate:        kycGate,
		outboxWriter:   outboxWriter,
		audit:          audit,
		log:            log,
	}
}

func (s *withdrawalService) Create(ctx context.Context, userID, amountCents int, currency string, method domain.WithdrawalMethod, payoutDetails map[string]string) (*domain.WithdrawalRequest, error) {
	if err := s.kycGate.CheckWithdrawal(ctx, userID, amountCents); err != nil {
		return nil, err
	}

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
	reqID := req.ID
	s.audit.Log(ctx, &userID, nil, domain.AuditActionWithdrawalRequested, &resType, &reqID, map[string]any{
		"amount_cents": amountCents,
		"currency":     currency,
		"method":       string(method),
	})

	if exceeds, _ := s.kycGate.ExceedsAMLThreshold(ctx, amountCents); exceeds {
		s.audit.Log(ctx, &userID, nil, domain.AuditActionAMLFlagged, &resType, &reqID, map[string]any{
			"amount_cents": amountCents,
			"currency":     currency,
			"operation":    "withdrawal",
		})
	}

	s.writeOutbox(ctx, notification.EventAdminWithdrawalPending,
		"withdrawal_request", strconv.Itoa(req.ID),
		notification.AdminWithdrawalPayload{
			RequestID:   req.ID,
			UserID:      userID,
			AmountCents: amountCents,
			Currency:    currency,
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

	s.writeOutbox(ctx, notification.EventWithdrawalApproved,
		"withdrawal_request", strconv.Itoa(requestID),
		notification.WithdrawalPayload{
			UserID:      req.UserID,
			RequestID:   requestID,
			AmountCents: req.AmountCents,
			Currency:    req.Currency,
			AdminID:     &adminID,
			Notes:       notes,
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

	s.writeOutbox(ctx, notification.EventWithdrawalRejected,
		"withdrawal_request", strconv.Itoa(requestID),
		notification.WithdrawalPayload{
			UserID:      req.UserID,
			RequestID:   requestID,
			AmountCents: req.AmountCents,
			Currency:    req.Currency,
			AdminID:     &adminID,
			Notes:       notes,
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

	s.writeOutbox(ctx, notification.EventWithdrawalCompleted,
		"withdrawal_request", strconv.Itoa(requestID),
		notification.WithdrawalPayload{
			UserID:      req.UserID,
			RequestID:   requestID,
			AmountCents: req.AmountCents,
			Currency:    req.Currency,
			AdminID:     &adminID,
		})
	return req, nil
}

// writeOutbox is a fire-and-forget helper that writes an outbox event using a
// pool-level connection (best-effort path).  Errors are logged and swallowed so
// that a transient outbox failure never rolls back or fails the primary domain
// operation that already committed.
func (s *withdrawalService) writeOutbox(
	ctx context.Context,
	eventType notification.EventType,
	aggregateType, aggregateID string,
	payload any,
) {
	if s.outboxWriter == nil {
		return
	}
	if err := s.outboxWriter.Write(ctx, eventType, aggregateType, aggregateID, payload); err != nil {
		s.log.Warn("outbox write failed (best-effort)",
			zap.String("event_type", string(eventType)),
			zap.String("aggregate_id", aggregateID),
			zap.Error(err),
		)
	}
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
