package handler

import (
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

var (
	docTypeGovID = domain.KYCDocGovID
	statusPend   = domain.KYCStatusPending
)

// ── kycProfileToResponse ──────────────────────────────────────────────────────

func TestKYCProfileToResponse_OptionalFieldsNil(t *testing.T) {
	now := time.Now()
	p := &domain.KYCProfile{
		ID:        1,
		UserID:    5,
		Status:    domain.KYCStatusPending,
		Tier:      domain.KYCTierOne,
		FullName:  "Juan",
		CreatedAt: now,
		UpdatedAt: now,
	}
	r := kycProfileToResponse(p)
	if r.ID != 1 || r.UserID != 5 || r.Status != "pending" {
		t.Errorf("unexpected base fields: %+v", r)
	}
	if r.DocumentType != nil || r.DateOfBirth != nil || r.SubmittedAt != nil ||
		r.ReviewedAt != nil || r.NextReviewAt != nil {
		t.Errorf("expected all optional fields nil, got %+v", r)
	}
}

func TestKYCProfileToResponse_AllOptionalFieldsSet(t *testing.T) {
	now := time.Now()
	dob := time.Date(1990, 3, 15, 0, 0, 0, 0, time.UTC)
	p := &domain.KYCProfile{
		ID:            2,
		UserID:        9,
		Status:        domain.KYCStatusApproved,
		Tier:          domain.KYCTierTwo,
		DocumentType:  &docTypeGovID,
		DateOfBirth:   &dob,
		SubmittedAt:   &now,
		ReviewedAt:    &now,
		NextReviewAt:  &now,
		PEPFlag:       true,
		SanctionsFlag: true,
		BalanceFrozen: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r := kycProfileToResponse(p)
	if r.DocumentType == nil || *r.DocumentType != "gov_id" {
		t.Errorf("expected document_type=gov_id, got %v", r.DocumentType)
	}
	if r.DateOfBirth == nil || *r.DateOfBirth != "1990-03-15" {
		t.Errorf("expected date_of_birth=1990-03-15, got %v", r.DateOfBirth)
	}
	if r.SubmittedAt == nil {
		t.Error("expected submitted_at set")
	}
	if r.ReviewedAt == nil {
		t.Error("expected reviewed_at set")
	}
	if r.NextReviewAt == nil {
		t.Error("expected next_review_at set")
	}
	if !r.PEPFlag || !r.SanctionsFlag || !r.BalanceFrozen {
		t.Errorf("expected flags set, got pep=%v sanctions=%v frozen=%v", r.PEPFlag, r.SanctionsFlag, r.BalanceFrozen)
	}
}

// ── kycDocumentToResponse ─────────────────────────────────────────────────────

func TestKYCDocumentToResponse_WithoutVerifiedAt(t *testing.T) {
	d := &domain.KYCDocument{
		ID:           10,
		ProfileID:    1,
		ProfileType:  domain.KYCProfileTypeUser,
		DocumentType: domain.KYCDocGovID,
		StorageKey:   "kyc/1/dpi.jpg",
		ContentType:  "image/jpeg",
		FileSize:     2048,
		Verified:     false,
		UploadedAt:   time.Now(),
	}
	r := kycDocumentToResponse(d)
	if r.ID != 10 || r.Verified || r.VerifiedAt != nil {
		t.Errorf("unexpected response: %+v", r)
	}
}

func TestKYCDocumentToResponse_WithVerifiedAt(t *testing.T) {
	now := time.Now()
	adminID := 99
	d := &domain.KYCDocument{
		ID:           11,
		DocumentType: domain.KYCDocSelfie,
		Verified:     true,
		VerifiedAt:   &now,
		VerifiedBy:   &adminID,
		UploadedAt:   time.Now(),
	}
	r := kycDocumentToResponse(d)
	if !r.Verified || r.VerifiedAt == nil || r.VerifiedBy == nil || *r.VerifiedBy != 99 {
		t.Errorf("unexpected response: %+v", r)
	}
}

// ── kycEventToResponse ────────────────────────────────────────────────────────

func TestKYCEventToResponse_WithoutOldStatus(t *testing.T) {
	e := &domain.KYCEvent{
		ID:        1,
		EventType: domain.KYCEventSubmitted,
		NewStatus: domain.KYCStatusPending,
		CreatedAt: time.Now(),
	}
	r := kycEventToResponse(e)
	if r.OldStatus != nil {
		t.Errorf("expected nil old_status, got %v", r.OldStatus)
	}
	if r.EventType != "submitted" {
		t.Errorf("expected event_type=submitted, got %s", r.EventType)
	}
}

func TestKYCEventToResponse_WithOldStatus(t *testing.T) {
	actorID := 7
	e := &domain.KYCEvent{
		ID:        2,
		EventType: domain.KYCEventApproved,
		ActorID:   &actorID,
		OldStatus: &statusPend,
		NewStatus: domain.KYCStatusApproved,
		Reason:    "all docs verified",
		TraceID:   "abc123",
		Metadata:  map[string]any{"tier": 2},
		CreatedAt: time.Now(),
	}
	r := kycEventToResponse(e)
	if r.OldStatus == nil || *r.OldStatus != "pending" {
		t.Errorf("expected old_status=pending, got %v", r.OldStatus)
	}
	if r.ActorID == nil || *r.ActorID != 7 {
		t.Errorf("expected actor_id=7, got %v", r.ActorID)
	}
	if r.TraceID != "abc123" {
		t.Errorf("expected trace_id=abc123, got %s", r.TraceID)
	}
}

// ── kycRequirementsToResponse ─────────────────────────────────────────────────

func TestKYCRequirementsToResponse_Empty(t *testing.T) {
	req := &service.KYCRequirements{
		CurrentTier:   domain.KYCTierUnverified,
		CurrentStatus: domain.KYCStatusUnverified,
	}
	r := kycRequirementsToResponse(req)
	if r.CurrentTier != 0 || r.CurrentStatus != "unverified" {
		t.Errorf("unexpected: %+v", r)
	}
	if len(r.RequiredDocs) != 0 || len(r.SubmittedDocs) != 0 || len(r.MissingDocs) != 0 {
		t.Errorf("expected empty doc lists, got %+v", r)
	}
}

func TestKYCRequirementsToResponse_WithDocs(t *testing.T) {
	req := &service.KYCRequirements{
		CurrentTier:   domain.KYCTierOne,
		CurrentStatus: domain.KYCStatusPending,
		RequiredDocs:  []domain.KYCDocumentType{domain.KYCDocGovID, domain.KYCDocSelfie},
		SubmittedDocs: []domain.KYCDocumentType{domain.KYCDocGovID},
		MissingDocs:   []domain.KYCDocumentType{domain.KYCDocSelfie},
	}
	r := kycRequirementsToResponse(req)
	if len(r.RequiredDocs) != 2 || len(r.SubmittedDocs) != 1 || len(r.MissingDocs) != 1 {
		t.Errorf("unexpected doc list sizes: req=%d sub=%d miss=%d", len(r.RequiredDocs), len(r.SubmittedDocs), len(r.MissingDocs))
	}
	if r.MissingDocs[0] != "selfie" {
		t.Errorf("expected missing_docs[0]=selfie, got %s", r.MissingDocs[0])
	}
}

// ── frozenBalanceToResponse ───────────────────────────────────────────────────

func TestFrozenBalanceToResponse(t *testing.T) {
	now := time.Now()
	s := &domain.FrozenBalanceSummary{
		UserID:            3,
		UserName:          "Ana García",
		UserEmail:         "ana@example.com",
		KYCStatus:         domain.KYCStatusApproved,
		KYCTier:           domain.KYCTierTwo,
		FrozenAmountCents: 100_000,
		FrozenReason:      "prize_win_freeze",
		FrozenSince:       now,
	}
	r := frozenBalanceToResponse(s)
	if r.UserID != 3 || r.UserName != "Ana García" || r.UserEmail != "ana@example.com" {
		t.Errorf("unexpected user fields: %+v", r)
	}
	if r.KYCStatus != "approved" || r.KYCTier != 2 {
		t.Errorf("unexpected kyc fields: %+v", r)
	}
	if r.FrozenAmountCents != 100_000 || r.FrozenReason != "prize_win_freeze" {
		t.Errorf("unexpected frozen fields: %+v", r)
	}
}

// ── riskDashboardToResponse ───────────────────────────────────────────────────

func TestRiskDashboardToResponse_WithTierDistribution(t *testing.T) {
	stats := &domain.KYCRiskDashboardStats{
		QueueDepth:              5,
		AvgReviewTimeSecs:       3600.0,
		TierDistribution:        map[domain.KYCTier]int64{domain.KYCTierOne: 3, domain.KYCTierTwo: 10},
		FrozenBalanceTotalCents: 250_000,
		PEPFlagCount:            2,
		SanctionsFlagCount:      1,
	}
	r := riskDashboardToResponse(stats)
	if r.QueueDepth != 5 || r.AvgReviewTimeSecs != 3600.0 {
		t.Errorf("unexpected base fields: %+v", r)
	}
	if r.FrozenBalanceTotalCents != 250_000 {
		t.Errorf("expected frozen=250000, got %d", r.FrozenBalanceTotalCents)
	}
	if len(r.TierDistribution) != 2 {
		t.Errorf("expected 2 tier entries, got %d", len(r.TierDistribution))
	}
}

func TestRiskDashboardToResponse_EmptyTierDistribution(t *testing.T) {
	stats := &domain.KYCRiskDashboardStats{
		TierDistribution: map[domain.KYCTier]int64{},
	}
	r := riskDashboardToResponse(stats)
	if len(r.TierDistribution) != 0 {
		t.Errorf("expected empty tier distribution, got %v", r.TierDistribution)
	}
}

// ── kybProfileToResponse ──────────────────────────────────────────────────────

func TestKYBProfileToResponse_OptionalFieldsNil(t *testing.T) {
	now := time.Now()
	p := &domain.KYBProfile{
		ID:        1,
		UserID:    5,
		Status:    domain.KYCStatusPending,
		LegalName: "Acme S.A.",
		TaxID:     "CF1234",
		CreatedAt: now,
		UpdatedAt: now,
	}
	r := kybProfileToResponse(p)
	if r.ID != 1 || r.LegalName != "Acme S.A." || r.Status != "pending" {
		t.Errorf("unexpected base fields: %+v", r)
	}
	if r.IncorporationDate != nil || r.SubmittedAt != nil || r.ReviewedAt != nil {
		t.Errorf("expected all optional fields nil, got %+v", r)
	}
}

func TestKYBProfileToResponse_AllOptionalFieldsSet(t *testing.T) {
	now := time.Now()
	incDate := time.Date(2010, 6, 1, 0, 0, 0, 0, time.UTC)
	p := &domain.KYBProfile{
		ID:                2,
		UserID:            9,
		Status:            domain.KYCStatusApproved,
		LegalName:         "Global Corp",
		TaxID:             "GT9999",
		Jurisdiction:      "GT",
		IncorporationDate: &incDate,
		SubmittedAt:       &now,
		ReviewedAt:        &now,
		RejectionReason:   "",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	r := kybProfileToResponse(p)
	if r.IncorporationDate == nil || *r.IncorporationDate != "2010-06-01" {
		t.Errorf("expected incorporation_date=2010-06-01, got %v", r.IncorporationDate)
	}
	if r.SubmittedAt == nil {
		t.Error("expected submitted_at set")
	}
	if r.ReviewedAt == nil {
		t.Error("expected reviewed_at set")
	}
}
