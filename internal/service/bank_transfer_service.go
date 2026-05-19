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

type bankTransferService struct {
	proofRepo    repository.BankTransferProofRepository
	outboxWriter *outbox.Writer
	audit        AuditLogger
	log          *zap.Logger
}

// NewBankTransferService constructs a BankTransferService.
func NewBankTransferService(
	proofRepo repository.BankTransferProofRepository,
	outboxWriter *outbox.Writer,
	audit AuditLogger,
	log *zap.Logger,
) BankTransferService {
	return &bankTransferService{
		proofRepo:    proofRepo,
		outboxWriter: outboxWriter,
		audit:        audit,
		log:          log,
	}
}

func (s *bankTransferService) Upload(ctx context.Context, userID, amountCents int, currency, storageKey, contentType string, fileSize int) (*domain.BankTransferProof, error) {
	if amountCents <= 0 {
		return nil, apperrors.Validation("amount_cents must be positive")
	}
	if storageKey == "" {
		return nil, apperrors.Validation("storage_key is required")
	}

	proof := &domain.BankTransferProof{
		UserID:      userID,
		AmountCents: amountCents,
		Currency:    currency,
		StorageKey:  storageKey,
		ContentType: contentType,
		FileSize:    fileSize,
	}
	if proof.Currency == "" {
		proof.Currency = "GTQ"
	}

	if err := s.proofRepo.Create(ctx, proof); err != nil {
		return nil, err
	}

	resType := "bank_transfer_proof"
	proofID := int(proof.ID)
	s.audit.Log(ctx, &userID, nil, domain.AuditActionBankTransferUploaded, &resType, &proofID, map[string]any{
		"amount_cents": amountCents,
		"currency":     proof.Currency,
		"file_size":    fileSize,
	})

	s.writeOutbox(ctx, notification.EventAdminBankTransferPending,
		"bank_transfer_proof", strconv.FormatInt(proof.ID, 10),
		notification.AdminBankTransferPayload{
			ProofID:     proof.ID,
			UserID:      userID,
			AmountCents: amountCents,
			Currency:    proof.Currency,
		})
	return proof, nil
}

func (s *bankTransferService) GetByID(ctx context.Context, id int) (*domain.BankTransferProof, error) {
	return s.proofRepo.GetByID(ctx, id)
}

func (s *bankTransferService) ListByUser(ctx context.Context, userID int) ([]*domain.BankTransferProof, error) {
	return s.proofRepo.ListByUser(ctx, userID)
}

func (s *bankTransferService) ListPending(ctx context.Context) ([]*domain.BankTransferProof, error) {
	return s.proofRepo.ListPending(ctx)
}

func (s *bankTransferService) ApproveTransfer(ctx context.Context, proofID, adminID int, notes string) (*domain.BankTransferProof, error) {
	proof, err := s.proofRepo.ApproveAndCredit(ctx, proofID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "bank_transfer_proof"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionBankTransferApproved, &resType, &proofID, map[string]any{
		"notes":        notes,
		"user_id":      proof.UserID,
		"amount_cents": proof.AmountCents,
	})

	s.writeOutbox(ctx, notification.EventPaymentBankTransferApproved,
		"bank_transfer_proof", strconv.Itoa(proofID),
		notification.BankTransferPayload{
			UserID:      proof.UserID,
			ProofID:     proof.ID,
			AmountCents: proof.AmountCents,
			Currency:    proof.Currency,
			AdminID:     &adminID,
			Notes:       notes,
		})
	return proof, nil
}

func (s *bankTransferService) RejectTransfer(ctx context.Context, proofID, adminID int, notes string) (*domain.BankTransferProof, error) {
	proof, err := s.proofRepo.Reject(ctx, proofID, adminID, notes)
	if err != nil {
		return nil, err
	}

	resType := "bank_transfer_proof"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionBankTransferRejected, &resType, &proofID, map[string]any{
		"notes": notes,
	})

	s.writeOutbox(ctx, notification.EventPaymentBankTransferRejected,
		"bank_transfer_proof", strconv.Itoa(proofID),
		notification.BankTransferPayload{
			UserID:      proof.UserID,
			ProofID:     proof.ID,
			AmountCents: proof.AmountCents,
			Currency:    proof.Currency,
			AdminID:     &adminID,
			Notes:       notes,
		})
	return proof, nil
}

// writeOutbox is a fire-and-forget helper that writes an outbox event using a
// pool-level connection (best-effort path).  Errors are logged and swallowed so
// that a transient outbox failure never rolls back or fails the primary domain
// operation that already committed.
func (s *bankTransferService) writeOutbox(
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

var _ BankTransferService = (*bankTransferService)(nil)
