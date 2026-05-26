package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// KYBSubmitInput carries the organiser-supplied business identity data.
type KYBSubmitInput struct {
	LegalName          string
	TaxID              string
	RegistrationNumber string
	Jurisdiction       string
	IncorporationDate  *time.Time
	UBOName            string
	UBODocumentNumber  string
}

// KYBWinnerFreezeNotifier is the narrow interface for KYB admin-event webhooks.
type KYBAdminNotifier interface {
	NotifyKYBApproved(ctx context.Context, profileID, userID int)
	NotifyKYBRejected(ctx context.Context, profileID, userID int, reason string)
}

// KYBService manages the full KYB lifecycle for quiniela organisers.
type KYBService interface {
	// Submit creates a new KYB profile or replaces a previously rejected one.
	Submit(ctx context.Context, userID int, input KYBSubmitInput) (*domain.KYBProfile, error)
	// Approve transitions the profile to approved and emits an audit event.
	Approve(ctx context.Context, profileID, adminID int) error
	// Reject transitions the profile to rejected. reason is required.
	Reject(ctx context.Context, profileID, adminID int, reason string) error
	// GetStatus returns the organiser's current KYB profile, or nil when none exists.
	GetStatus(ctx context.Context, userID int) (*domain.KYBProfile, error)
	// ListPending returns profiles in active review states.
	ListPending(ctx context.Context, limit, offset int) ([]*domain.KYBProfile, error)
	// GetByID returns any profile by primary key (admin access).
	GetByID(ctx context.Context, profileID int) (*domain.KYBProfile, error)
}

// kybService is the concrete implementation of KYBService.
type kybService struct {
	repo      repository.KYBRepository
	eventRepo repository.KYCEventRepository
	audit     AuditLogger
	notifier  KYBAdminNotifier // may be nil
	log       *zap.Logger
}

// NewKYBService constructs a KYBService.
func NewKYBService(
	repo repository.KYBRepository,
	eventRepo repository.KYCEventRepository,
	audit AuditLogger,
	notifier KYBAdminNotifier,
	log *zap.Logger,
) KYBService {
	return &kybService{
		repo:      repo,
		eventRepo: eventRepo,
		audit:     audit,
		notifier:  notifier,
		log:       log,
	}
}

func (s *kybService) Submit(ctx context.Context, userID int, input KYBSubmitInput) (*domain.KYBProfile, error) {
	if input.LegalName == "" {
		return nil, apperrors.Validation("legal_name is required")
	}
	if input.TaxID == "" {
		return nil, apperrors.Validation("tax_id is required")
	}
	if input.Jurisdiction == "" {
		return nil, apperrors.Validation("jurisdiction is required")
	}
	if input.UBOName == "" {
		return nil, apperrors.Validation("ubo_name is required")
	}

	dup, err := s.repo.ExistsByTaxIDAndJurisdiction(ctx, input.TaxID, input.Jurisdiction, userID)
	if err != nil {
		return nil, err
	}
	if dup {
		return nil, apperrors.Conflict("ERR_DUPLICATE_KYB: a KYB profile with this tax_id and jurisdiction already exists")
	}

	existing, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil && (existing.Status == domain.KYCStatusPending || existing.Status == domain.KYCStatusUnderReview) {
		return nil, apperrors.Conflict("a KYB submission is already pending review")
	}

	now := time.Now()
	profile := &domain.KYBProfile{
		UserID:             userID,
		Status:             domain.KYCStatusPending,
		LegalName:          input.LegalName,
		TaxID:              input.TaxID,
		RegistrationNumber: input.RegistrationNumber,
		Jurisdiction:       input.Jurisdiction,
		IncorporationDate:  input.IncorporationDate,
		UBOName:            input.UBOName,
		UBODocumentNumber:  input.UBODocumentNumber,
		SubmittedAt:        &now,
	}

	if err := s.repo.Create(ctx, profile); err != nil {
		return nil, err
	}

	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profile.ID,
		ProfileType: domain.KYCProfileTypeOrg,
		EventType:   domain.KYCEventSubmitted,
		ActorID:     &userID,
		NewStatus:   domain.KYCStatusPending,
		TraceID:     traceIDFromCtx(ctx),
	})

	resType := "kyb_profile"
	s.audit.Log(ctx, &userID, nil, domain.AuditActionKYBSubmitted, &resType, &profile.ID, nil)
	return profile, nil
}

func (s *kybService) Approve(ctx context.Context, profileID, adminID int) error {
	profile, err := s.repo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyb profile not found")
	}
	if profile.Status == domain.KYCStatusApproved {
		return apperrors.Conflict("profile is already approved")
	}

	if err := s.repo.UpdateStatus(ctx, profileID, domain.KYCStatusApproved, adminID, ""); err != nil {
		return err
	}

	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeOrg,
		EventType:   domain.KYCEventApproved,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   domain.KYCStatusApproved,
		TraceID:     traceIDFromCtx(ctx),
	})

	resType := "kyb_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionKYBApproved, &resType, &profileID, nil)

	if s.notifier != nil {
		s.notifier.NotifyKYBApproved(ctx, profileID, profile.UserID)
	}
	return nil
}

func (s *kybService) Reject(ctx context.Context, profileID, adminID int, reason string) error {
	if reason == "" {
		return apperrors.Validation("rejection reason is required")
	}
	profile, err := s.repo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyb profile not found")
	}
	if profile.Status == domain.KYCStatusRejected {
		return apperrors.Conflict("profile is already rejected")
	}

	if err := s.repo.UpdateStatus(ctx, profileID, domain.KYCStatusRejected, adminID, reason); err != nil {
		return err
	}

	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeOrg,
		EventType:   domain.KYCEventRejected,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   domain.KYCStatusRejected,
		Reason:      reason,
		TraceID:     traceIDFromCtx(ctx),
	})

	resType := "kyb_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role, domain.AuditActionKYBRejected, &resType, &profileID,
		map[string]any{"reason": reason})

	if s.notifier != nil {
		s.notifier.NotifyKYBRejected(ctx, profileID, profile.UserID, reason)
	}
	return nil
}

func (s *kybService) GetStatus(ctx context.Context, userID int) (*domain.KYBProfile, error) {
	return s.repo.GetByUserID(ctx, userID)
}

func (s *kybService) ListPending(ctx context.Context, limit, offset int) ([]*domain.KYBProfile, error) {
	return s.repo.ListPending(ctx, limit, offset)
}

func (s *kybService) GetByID(ctx context.Context, profileID int) (*domain.KYBProfile, error) {
	return s.repo.GetByID(ctx, profileID)
}

func (s *kybService) appendEvent(ctx context.Context, event *domain.KYCEvent) {
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.log.Warn("kyb: failed to append audit event (best-effort)",
			zap.String("event_type", string(event.EventType)),
			zap.Int("profile_id", event.ProfileID),
			zap.Error(err),
		)
	}
}

var _ KYBService = (*kybService)(nil)
