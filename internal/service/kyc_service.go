package service

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── Request/response value objects ───────────────────────────────────────────

// SubmitKYCRequest carries user-provided identity data for a KYC submission.
type SubmitKYCRequest struct {
	FullName          string
	DateOfBirth       *time.Time
	Nationality       string
	DocumentType      domain.KYCDocumentType
	DocumentNumber    string
	AddressLine       string
	City              string
	Country           string
	PostalCode        string
	DeviceFingerprint string // optional; SHA-256 hex from X-Device-Fingerprint header
}

// UploadDocRequest carries metadata for a new KYC document upload.
// StorageKey is the opaque path returned by FileStore.Put; the caller is
// responsible for streaming the file bytes to the store before calling this.
type UploadDocRequest struct {
	ProfileID    int
	ProfileType  domain.KYCProfileType
	DocumentType domain.KYCDocumentType
	StorageKey   string
	ContentType  string
	FileSize     int
	FileHash     string // hex-encoded SHA-256 of the raw bytes
}

// KYCRequirements describes the documents still needed for the next tier.
type KYCRequirements struct {
	CurrentTier   domain.KYCTier
	CurrentStatus domain.KYCStatus
	RequiredDocs  []domain.KYCDocumentType
	SubmittedDocs []domain.KYCDocumentType
	MissingDocs   []domain.KYCDocumentType
}

// ── Service interface ─────────────────────────────────────────────────────────

// KYCService manages the full KYC lifecycle: user submissions, admin
// review queue, document management, and balance-freeze operations.
type KYCService interface {
	// ── User-facing ────────────────────────────────────────────────────────
	// GetProfile returns the user's KYC profile. Returns nil when no profile
	// has been submitted yet (user is implicitly at Tier 0).
	GetProfile(ctx context.Context, userID int) (*domain.KYCProfile, error)
	// Submit creates or updates the user's KYC profile and transitions the
	// status to pending. Resubmission is allowed after rejection.
	Submit(ctx context.Context, userID int, req SubmitKYCRequest) (*domain.KYCProfile, error)
	// UploadDocument records an uploaded KYC document's metadata.
	UploadDocument(ctx context.Context, userID int, req UploadDocRequest) (*domain.KYCDocument, error)
	// ListDocuments returns all documents uploaded for the user's profile.
	ListDocuments(ctx context.Context, userID int) ([]*domain.KYCDocument, error)
	// GetRequirements returns the documents still needed for the next tier.
	GetRequirements(ctx context.Context, userID int) (*KYCRequirements, error)
	// ListEvents returns the full audit trail for a KYC profile.
	ListEvents(ctx context.Context, profileID int, profileType domain.KYCProfileType, p repository.CursorPage) ([]*domain.KYCEvent, string, error)

	// ── Admin ──────────────────────────────────────────────────────────────
	// ListQueue returns profiles in the review queue with optional filtering.
	ListQueue(ctx context.Context, f repository.KYCProfileFilters, p repository.Pagination) ([]*domain.KYCProfile, error)
	// GetProfileByID returns any profile by its primary key (admin access).
	GetProfileByID(ctx context.Context, profileID int) (*domain.KYCProfile, error)
	// Approve transitions a profile to approved and updates the user's tier.
	Approve(ctx context.Context, profileID, adminID int, tier domain.KYCTier) error
	// Reject transitions a profile to rejected with a mandatory reason.
	Reject(ctx context.Context, profileID, adminID int, reason string) error
	// Escalate flags a profile for senior compliance review.
	Escalate(ctx context.Context, profileID, adminID int, reason string) error
	// RequestDocument asks the user to upload additional documentation.
	RequestDocument(ctx context.Context, profileID, adminID int, docType domain.KYCDocumentType, reason string) error
	// VerifyDocument marks a single uploaded document as verified.
	VerifyDocument(ctx context.Context, docID int64, adminID int) error
	// ListFrozenBalances returns all accounts with a frozen balance.
	ListFrozenBalances(ctx context.Context) ([]*domain.FrozenBalanceSummary, error)
	// ReleaseFrozenBalance clears the balance freeze for a user after the
	// admin confirms the KYC review is satisfactory.
	ReleaseFrozenBalance(ctx context.Context, userID, adminID int) error
	// GetRiskDashboard returns aggregated KYC risk metrics. Results are cached
	// for DefaultKYCRiskDashboardCacheTTLSecs seconds when a cache.Store is wired.
	GetRiskDashboard(ctx context.Context) (*domain.KYCRiskDashboardStats, error)
	// RecalculateRiskScore computes and persists a 0–100 risk score for the
	// given user's profile. Returns the new score. Safe to call on any financial
	// event; idempotent when the profile hasn't changed.
	RecalculateRiskScore(ctx context.Context, userID int) (int, error)

	// ── Internal ──────────────────────────────────────────────────────────
	// FreezeBalance is called by payment services when a prize credit for a
	// Tier 0 or Tier 1 user must be held in escrow pending KYC approval.
	FreezeBalance(ctx context.Context, userID, prizeCents int, reason string) error
	// ListDueForReview returns approved profiles whose next_review_at is in the
	// past, using time.Now() as the threshold. The scheduler calls this daily to
	// trigger re-verification reminder notifications.
	ListDueForReview(ctx context.Context) ([]*domain.KYCProfile, error)
}

// ── Implementation ────────────────────────────────────────────────────────────

type kycService struct {
	profileRepo repository.KYCProfileRepository
	docRepo     repository.KYCDocumentRepository
	eventRepo   repository.KYCEventRepository
	params      SystemParamService
	audit       AuditLogger
	log         *zap.Logger
	metrics     *KYCMetrics
	cache       cache.Store                        // optional; nil disables risk-dashboard caching
	ledger      repository.BalanceLedgerRepository // optional; nil skips deposit velocity in risk score
}

// NewKYCService constructs a KYCService. Pass a non-nil KYCMetrics (variadic)
// to activate OTel instrumentation; nil is safe and skips all recording.
func NewKYCService(
	profileRepo repository.KYCProfileRepository,
	docRepo repository.KYCDocumentRepository,
	eventRepo repository.KYCEventRepository,
	params SystemParamService,
	audit AuditLogger,
	log *zap.Logger,
	metrics ...*KYCMetrics,
) KYCService {
	svc := &kycService{
		profileRepo: profileRepo,
		docRepo:     docRepo,
		eventRepo:   eventRepo,
		params:      params,
		audit:       audit,
		log:         log,
	}
	if len(metrics) > 0 {
		svc.metrics = metrics[0]
	}
	return svc
}

// SetCache wires a cache.Store for risk-dashboard caching. Called once at
// startup; nil means no caching (acceptable in tests).
func (s *kycService) SetCache(store cache.Store) { s.cache = store }

// SetLedger wires the ledger repo for deposit-velocity scoring.
// Called once at startup; nil omits the deposit-velocity component.
func (s *kycService) SetLedger(ledger repository.BalanceLedgerRepository) { s.ledger = ledger }

func (s *kycService) GetProfile(ctx context.Context, userID int) (*domain.KYCProfile, error) {
	return s.profileRepo.GetByUserID(ctx, userID)
}

func validateSubmitRequest(req SubmitKYCRequest) error {
	if req.FullName == "" {
		return apperrors.Validation("full_name is required")
	}
	if req.DocumentType == "" {
		return apperrors.Validation("document_type is required")
	}
	if req.DocumentNumber == "" {
		return apperrors.Validation("document_number is required")
	}
	return nil
}

func checkSubmitConflicts(existing *domain.KYCProfile) error {
	if existing == nil {
		return nil
	}
	if existing.Status == domain.KYCStatusPending {
		return apperrors.Conflict("a KYC submission is already pending review")
	}
	if existing.Status == domain.KYCStatusUnderReview {
		return apperrors.Conflict("your KYC profile is currently under review")
	}
	return nil
}

func (s *kycService) checkDeviceFraud(ctx context.Context, fingerprint string, userID int) error {
	if fingerprint == "" {
		return nil
	}
	devCount, err := s.profileRepo.CountAccountsByDeviceFingerprint(ctx, fingerprint, userID)
	if err != nil {
		return err
	}
	if devCount > 0 {
		if s.metrics != nil {
			s.metrics.RecordFraudFlag(ctx, "device_fingerprint_collision")
		}
		return apperrors.Conflict("this device is already associated with another verified account")
	}
	return nil
}

func (s *kycService) Submit(ctx context.Context, userID int, req SubmitKYCRequest) (*domain.KYCProfile, error) {
	if err := validateSubmitRequest(req); err != nil {
		return nil, err
	}

	existing, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := checkSubmitConflicts(existing); err != nil {
		return nil, err
	}

	dupExists, err := s.profileRepo.ExistsByDocumentIdentity(ctx, req.DocumentType, req.DocumentNumber, req.DateOfBirth, userID)
	if err != nil {
		return nil, err
	}
	if dupExists {
		if s.metrics != nil {
			s.metrics.RecordFraudFlag(ctx, "duplicate_identity")
		}
		return nil, apperrors.Conflict("this identity document is already associated with another account")
	}

	if err := s.checkDeviceFraud(ctx, req.DeviceFingerprint, userID); err != nil {
		return nil, err
	}

	now := time.Now()
	dt := req.DocumentType
	var devFP *string
	if req.DeviceFingerprint != "" {
		devFP = &req.DeviceFingerprint
	}
	profile := &domain.KYCProfile{
		UserID:            userID,
		Status:            domain.KYCStatusPending,
		Tier:              domain.KYCTierUnverified,
		FullName:          req.FullName,
		DateOfBirth:       req.DateOfBirth,
		Nationality:       req.Nationality,
		DocumentType:      &dt,
		DocumentNumber:    req.DocumentNumber,
		AddressLine:       req.AddressLine,
		City:              req.City,
		Country:           req.Country,
		PostalCode:        req.PostalCode,
		DeviceFingerprint: devFP,
		SubmittedAt:       &now,
	}
	if existing != nil {
		profile.Tier = existing.Tier
	}

	if err := s.profileRepo.Upsert(ctx, profile); err != nil {
		return nil, err
	}

	var oldStatus *domain.KYCStatus
	if existing != nil {
		oldStatus = &existing.Status
	}
	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profile.ID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventSubmitted,
		ActorID:     &userID,
		OldStatus:   oldStatus,
		NewStatus:   domain.KYCStatusPending,
		TraceID:     traceIDFromCtx(ctx),
	})

	resType := "kyc_profile"
	resID := profile.ID
	s.audit.Log(ctx, &userID, nil,
		domain.AuditActionKYCSubmitted, &resType, &resID, nil)

	if s.metrics != nil {
		s.metrics.RecordSubmission(ctx, "success")
	}
	return profile, nil
}

func (s *kycService) UploadDocument(ctx context.Context, userID int, req UploadDocRequest) (*domain.KYCDocument, error) {
	if req.StorageKey == "" {
		return nil, apperrors.Validation("storage_key is required")
	}
	if req.FileSize <= 0 {
		return nil, apperrors.Validation("file_size must be positive")
	}
	maxBytes := s.params.GetInt(ctx, domain.ParamKeyKYCMaxDocUploadBytes, domain.DefaultKYCMaxDocUploadBytes)
	if req.FileSize > maxBytes {
		return nil, apperrors.Validation(fmt.Sprintf(
			"document size (%d bytes) exceeds the maximum allowed (%d bytes)",
			req.FileSize, maxBytes,
		))
	}
	doc := &domain.KYCDocument{
		ProfileID:    req.ProfileID,
		ProfileType:  req.ProfileType,
		DocumentType: req.DocumentType,
		StorageKey:   req.StorageKey,
		ContentType:  req.ContentType,
		FileSize:     req.FileSize,
		FileHash:     req.FileHash,
	}
	if err := s.docRepo.Create(ctx, doc); err != nil {
		return nil, err
	}
	resType := "kyc_document"
	docIDInt := int(doc.ID)
	s.audit.Log(ctx, &userID, nil,
		domain.AuditActionKYCDocUploaded, &resType, &docIDInt, map[string]any{
			"document_type": string(req.DocumentType),
		})
	return doc, nil
}

func (s *kycService) ListDocuments(ctx context.Context, userID int) ([]*domain.KYCDocument, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if profile == nil {
		return nil, nil
	}
	return s.docRepo.ListByProfile(ctx, profile.ID, domain.KYCProfileTypeUser)
}

func (s *kycService) GetRequirements(ctx context.Context, userID int) (*KYCRequirements, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	reqs := &KYCRequirements{
		CurrentTier:   domain.KYCTierUnverified,
		CurrentStatus: domain.KYCStatusUnverified,
		RequiredDocs:  tier2RequiredDocs,
	}
	if profile != nil {
		reqs.CurrentTier = profile.Tier
		reqs.CurrentStatus = profile.Status

		docs, err := s.docRepo.ListByProfile(ctx, profile.ID, domain.KYCProfileTypeUser)
		if err != nil {
			return nil, err
		}
		submitted := make(map[domain.KYCDocumentType]bool)
		for _, d := range docs {
			submitted[d.DocumentType] = true
			reqs.SubmittedDocs = append(reqs.SubmittedDocs, d.DocumentType)
		}
		for _, required := range tier2RequiredDocs {
			if !submitted[required] {
				reqs.MissingDocs = append(reqs.MissingDocs, required)
			}
		}
	} else {
		reqs.MissingDocs = tier2RequiredDocs
	}
	return reqs, nil
}

// tier2RequiredDocs lists the documents needed to achieve Tier 2 status.
var tier2RequiredDocs = []domain.KYCDocumentType{
	domain.KYCDocGovID,
	domain.KYCDocSelfie,
}

func (s *kycService) ListEvents(ctx context.Context, profileID int, profileType domain.KYCProfileType, p repository.CursorPage) ([]*domain.KYCEvent, string, error) {
	return s.eventRepo.ListByProfile(ctx, profileID, profileType, p)
}

func (s *kycService) ListQueue(ctx context.Context, f repository.KYCProfileFilters, p repository.Pagination) ([]*domain.KYCProfile, error) {
	return s.profileRepo.ListPending(ctx, f, p)
}

func (s *kycService) GetProfileByID(ctx context.Context, profileID int) (*domain.KYCProfile, error) {
	return s.profileRepo.GetByID(ctx, profileID)
}

func (s *kycService) Approve(ctx context.Context, profileID, adminID int, tier domain.KYCTier) error {
	profile, err := s.profileRepo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyc profile not found")
	}
	if err := s.profileRepo.UpdateStatus(ctx, profileID, domain.KYCStatusApproved, adminID, ""); err != nil {
		return err
	}
	intervalDays := s.params.GetInt(ctx, domain.ParamKeyKYCReviewIntervalDays, domain.DefaultKYCReviewIntervalDays)
	nextReview := time.Now().AddDate(0, 0, intervalDays)
	if err := s.profileRepo.UpdateTier(ctx, profile.UserID, tier, &nextReview); err != nil {
		return err
	}
	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventApproved,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   domain.KYCStatusApproved,
		TraceID:     traceIDFromCtx(ctx),
		Metadata:    map[string]any{"tier": int(tier)},
	})
	resType := "kyc_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role,
		domain.AuditActionKYCApproved, &resType, &profileID,
		map[string]any{"tier": int(tier)})
	if s.metrics != nil && profile.SubmittedAt != nil {
		s.metrics.RecordReviewDuration(ctx, time.Since(*profile.SubmittedAt).Seconds(), "approved")
	}
	return nil
}

func (s *kycService) Reject(ctx context.Context, profileID, adminID int, reason string) error {
	if reason == "" {
		return apperrors.Validation("rejection reason is required")
	}
	profile, err := s.profileRepo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyc profile not found")
	}
	if err := s.profileRepo.UpdateStatus(ctx, profileID, domain.KYCStatusRejected, adminID, reason); err != nil {
		return err
	}
	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventRejected,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   domain.KYCStatusRejected,
		Reason:      reason,
		TraceID:     traceIDFromCtx(ctx),
	})
	resType := "kyc_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role,
		domain.AuditActionKYCRejected, &resType, &profileID,
		map[string]any{"reason": reason})
	if s.metrics != nil && profile.SubmittedAt != nil {
		s.metrics.RecordReviewDuration(ctx, time.Since(*profile.SubmittedAt).Seconds(), "rejected")
	}
	return nil
}

func (s *kycService) Escalate(ctx context.Context, profileID, adminID int, reason string) error {
	profile, err := s.profileRepo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyc profile not found")
	}
	if err := s.profileRepo.UpdateStatus(ctx, profileID, domain.KYCStatusEscalated, adminID, ""); err != nil {
		return err
	}
	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventEscalated,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   domain.KYCStatusEscalated,
		Reason:      reason,
		TraceID:     traceIDFromCtx(ctx),
	})
	resType := "kyc_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role,
		domain.AuditActionKYCEscalated, &resType, &profileID,
		map[string]any{"reason": reason})
	return nil
}

func (s *kycService) RequestDocument(ctx context.Context, profileID, adminID int, docType domain.KYCDocumentType, reason string) error {
	profile, err := s.profileRepo.GetByID(ctx, profileID)
	if err != nil {
		return err
	}
	if profile == nil {
		return apperrors.NotFound("kyc profile not found")
	}
	s.appendEvent(ctx, &domain.KYCEvent{
		ProfileID:   profileID,
		ProfileType: domain.KYCProfileTypeUser,
		EventType:   domain.KYCEventDocRequested,
		ActorID:     &adminID,
		OldStatus:   &profile.Status,
		NewStatus:   profile.Status, // no status change
		Reason:      reason,
		TraceID:     traceIDFromCtx(ctx),
		Metadata:    map[string]any{"document_type": string(docType)},
	})
	resType := "kyc_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role,
		domain.AuditActionKYCDocRequested, &resType, &profileID,
		map[string]any{"document_type": string(docType), "reason": reason})
	return nil
}

func (s *kycService) VerifyDocument(ctx context.Context, docID int64, adminID int) error {
	return s.docRepo.MarkVerified(ctx, docID, adminID)
}

func (s *kycService) ListFrozenBalances(ctx context.Context) ([]*domain.FrozenBalanceSummary, error) {
	return s.profileRepo.ListFrozen(ctx)
}

func (s *kycService) ReleaseFrozenBalance(ctx context.Context, userID, adminID int) error {
	if err := s.profileRepo.SetFrozen(ctx, userID, false, 0, ""); err != nil {
		return err
	}
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	profileID := 0
	if profile != nil {
		profileID = profile.ID
		s.appendEvent(ctx, &domain.KYCEvent{
			ProfileID:   profileID,
			ProfileType: domain.KYCProfileTypeUser,
			EventType:   domain.KYCEventUnfrozen,
			ActorID:     &adminID,
			OldStatus:   &profile.Status,
			NewStatus:   profile.Status,
			TraceID:     traceIDFromCtx(ctx),
		})
	}
	resType := "kyc_profile"
	role := domain.RoleAdmin
	s.audit.Log(ctx, &adminID, &role,
		domain.AuditActionKYCUnfrozen, &resType, &profileID, nil)
	return nil
}

func (s *kycService) FreezeBalance(ctx context.Context, userID, prizeCents int, reason string) error {
	if err := s.profileRepo.SetFrozen(ctx, userID, true, prizeCents, reason); err != nil {
		return err
	}
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	if profile != nil {
		s.appendEvent(ctx, &domain.KYCEvent{
			ProfileID:   profile.ID,
			ProfileType: domain.KYCProfileTypeUser,
			EventType:   domain.KYCEventFrozen,
			OldStatus:   &profile.Status,
			NewStatus:   profile.Status,
			Reason:      reason,
			TraceID:     traceIDFromCtx(ctx),
			Metadata:    map[string]any{"frozen_amount_cents": prizeCents},
		})
	}
	return nil
}

// appendEvent is a best-effort helper that inserts a kyc_events row.
// Failures are logged and swallowed so that a transient DB issue never rolls
// back a primary domain operation that has already committed.
func (s *kycService) appendEvent(ctx context.Context, event *domain.KYCEvent) {
	if event.Metadata == nil {
		event.Metadata = map[string]any{}
	}
	if err := s.eventRepo.Create(ctx, event); err != nil {
		s.log.Warn("kyc: failed to append audit event (best-effort)",
			zap.String("event_type", string(event.EventType)),
			zap.Int("profile_id", event.ProfileID),
			zap.Error(err),
		)
	}
}

// traceIDFromCtx extracts the W3C trace ID string from the active OTel span.
// Returns an empty string when no valid span is present (tracing disabled, tests).
func traceIDFromCtx(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

func (s *kycService) ListDueForReview(ctx context.Context) ([]*domain.KYCProfile, error) {
	return s.profileRepo.ListDueForReview(ctx, time.Now())
}

// riskScoreWeights are the additive risk factors for the 0–100 score.
// Higher tier = lower base risk; flags and velocity add to the score.
const (
	riskWeightTierUnverified  = 30
	riskWeightTierOne         = 20
	riskWeightTierTwo         = 10
	riskWeightTierThree       = 0
	riskWeightPEP             = 25
	riskWeightSanctions       = 35
	riskWeightDepositVelocity = 20 // deposits last 24h exceed AML threshold
	riskWeightFrozenBalance   = 15
)

func (s *kycService) RecalculateRiskScore(ctx context.Context, userID int) (int, error) {
	profile, err := s.profileRepo.GetByUserID(ctx, userID)
	if err != nil {
		return 0, err
	}
	if profile == nil {
		return 0, apperrors.NotFound("kyc profile not found")
	}

	score := 0
	switch profile.Tier {
	case domain.KYCTierUnverified:
		score += riskWeightTierUnverified
	case domain.KYCTierOne:
		score += riskWeightTierOne
	case domain.KYCTierTwo:
		score += riskWeightTierTwo
	case domain.KYCTierThree:
		score += riskWeightTierThree
	}
	if profile.PEPFlag {
		score += riskWeightPEP
	}
	if profile.SanctionsFlag {
		score += riskWeightSanctions
	}
	if profile.BalanceFrozen {
		score += riskWeightFrozenBalance
	}
	if s.ledger != nil {
		threshold := s.params.GetInt(ctx, domain.ParamKeyKYCAMLThresholdCents, domain.DefaultKYCAMLThresholdCents)
		creditKinds := []domain.BalanceLedgerKind{
			domain.LedgerKindBankTransfer,
			domain.LedgerKindWebhookRecurrente,
			domain.LedgerKindWebhookPayPal,
			domain.LedgerKindPrize,
		}
		sum, err := s.ledger.SumTransactionsByUserAndPeriod(ctx, userID, creditKinds, time.Now().Add(-24*time.Hour))
		if err != nil {
			s.log.Warn("risk score: ledger sum failed, skipping velocity component", zap.Error(err))
		} else if sum >= int64(threshold) {
			score += riskWeightDepositVelocity
		}
	}
	if score > 100 {
		score = 100
	}

	if err := s.profileRepo.UpdateRiskScore(ctx, profile.ID, score); err != nil {
		return 0, err
	}
	return score, nil
}

func (s *kycService) GetRiskDashboard(ctx context.Context) (*domain.KYCRiskDashboardStats, error) {
	if s.cache != nil {
		if v, ok := cacheGet[*domain.KYCRiskDashboardStats](ctx, s.cache, domain.CacheKeyKYCRiskDashboard, s.log); ok {
			return v, nil
		}
	}
	stats, err := s.profileRepo.RiskDashboardStats(ctx)
	if err != nil {
		return nil, err
	}
	if s.cache != nil {
		ttl := time.Duration(s.params.GetInt(ctx, domain.ParamKeyKYCRiskDashboardCacheTTLSec, domain.DefaultKYCRiskDashboardCacheTTLSecs)) * time.Second
		cacheSet(ctx, s.cache, domain.CacheKeyKYCRiskDashboard, stats, ttl, s.log)
	}
	return stats, nil
}

var _ KYCService = (*kycService)(nil)
