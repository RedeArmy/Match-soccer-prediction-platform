package handler

import (
	"fmt"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// KYCProfileResponse is returned by user-facing KYC status and submission endpoints.
type KYCProfileResponse struct {
	ID              int     `json:"id"`
	UserID          int     `json:"user_id"`
	Status          string  `json:"status"`
	Tier            int     `json:"tier"`
	FullName        string  `json:"full_name"`
	DateOfBirth     *string `json:"date_of_birth,omitempty"`
	Nationality     string  `json:"nationality"`
	DocumentType    *string `json:"document_type,omitempty"`
	DocumentNumber  string  `json:"document_number,omitempty"`
	AddressLine     string  `json:"address_line,omitempty"`
	City            string  `json:"city,omitempty"`
	Country         string  `json:"country,omitempty"`
	PostalCode      string  `json:"postal_code,omitempty"`
	SubmittedAt     *string `json:"submitted_at,omitempty"`
	ReviewedAt      *string `json:"reviewed_at,omitempty"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
	RiskScore       int     `json:"risk_score,omitempty"`
	PEPFlag         bool    `json:"pep_flag,omitempty"`
	SanctionsFlag   bool    `json:"sanctions_flag,omitempty"`
	BalanceFrozen   bool    `json:"balance_frozen"`
	NextReviewAt    *string `json:"next_review_at,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// KYCDocumentResponse is returned by document upload and listing endpoints.
type KYCDocumentResponse struct {
	ID           int64   `json:"id"`
	ProfileID    int     `json:"profile_id"`
	ProfileType  string  `json:"profile_type"`
	DocumentType string  `json:"document_type"`
	StorageKey   string  `json:"storage_key"`
	ContentType  string  `json:"content_type"`
	FileSize     int     `json:"file_size"`
	FileHash     string  `json:"file_hash,omitempty"`
	Verified     bool    `json:"verified"`
	VerifiedAt   *string `json:"verified_at,omitempty"`
	VerifiedBy   *int    `json:"verified_by,omitempty"`
	UploadedAt   string  `json:"uploaded_at"`
}

// KYCEventResponse is a single immutable event from the KYC audit trail.
type KYCEventResponse struct {
	ID        int64          `json:"id"`
	EventType string         `json:"event_type"`
	ActorID   *int           `json:"actor_id,omitempty"`
	OldStatus *string        `json:"old_status,omitempty"`
	NewStatus string         `json:"new_status"`
	Reason    string         `json:"reason,omitempty"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	TraceID   string         `json:"trace_id,omitempty"`
	CreatedAt string         `json:"created_at"`
}

// KYCRequirementsResponse describes the documents still needed for the next tier.
type KYCRequirementsResponse struct {
	CurrentTier   int      `json:"current_tier"`
	CurrentStatus string   `json:"current_status"`
	RequiredDocs  []string `json:"required_docs"`
	SubmittedDocs []string `json:"submitted_docs"`
	MissingDocs   []string `json:"missing_docs"`
}

// FrozenBalanceResponse is returned by the admin frozen-balances endpoint.
type FrozenBalanceResponse struct {
	UserID            int    `json:"user_id"`
	UserName          string `json:"user_name"`
	UserEmail         string `json:"user_email"`
	KYCStatus         string `json:"kyc_status"`
	KYCTier           int    `json:"kyc_tier"`
	FrozenAmountCents int    `json:"frozen_amount_cents"`
	FrozenReason      string `json:"frozen_reason"`
	FrozenSince       string `json:"frozen_since"`
}

// ── Converters ────────────────────────────────────────────────────────────────

func kycProfileToResponse(p *domain.KYCProfile) KYCProfileResponse {
	r := KYCProfileResponse{
		ID:              p.ID,
		UserID:          p.UserID,
		Status:          string(p.Status),
		Tier:            int(p.Tier),
		FullName:        p.FullName,
		Nationality:     p.Nationality,
		DocumentNumber:  p.DocumentNumber,
		AddressLine:     p.AddressLine,
		City:            p.City,
		Country:         p.Country,
		PostalCode:      p.PostalCode,
		RejectionReason: p.RejectionReason,
		RiskScore:       p.RiskScore,
		PEPFlag:         p.PEPFlag,
		SanctionsFlag:   p.SanctionsFlag,
		BalanceFrozen:   p.BalanceFrozen,
		CreatedAt:       p.CreatedAt.Format(timeFormat),
		UpdatedAt:       p.UpdatedAt.Format(timeFormat),
	}
	if p.DocumentType != nil {
		s := string(*p.DocumentType)
		r.DocumentType = &s
	}
	if p.DateOfBirth != nil {
		s := p.DateOfBirth.Format("2006-01-02")
		r.DateOfBirth = &s
	}
	if p.SubmittedAt != nil {
		s := p.SubmittedAt.Format(timeFormat)
		r.SubmittedAt = &s
	}
	if p.ReviewedAt != nil {
		s := p.ReviewedAt.Format(timeFormat)
		r.ReviewedAt = &s
	}
	if p.NextReviewAt != nil {
		s := p.NextReviewAt.Format(timeFormat)
		r.NextReviewAt = &s
	}
	return r
}

func kycDocumentToResponse(d *domain.KYCDocument) KYCDocumentResponse {
	r := KYCDocumentResponse{
		ID:           d.ID,
		ProfileID:    d.ProfileID,
		ProfileType:  string(d.ProfileType),
		DocumentType: string(d.DocumentType),
		StorageKey:   d.StorageKey,
		ContentType:  d.ContentType,
		FileSize:     d.FileSize,
		FileHash:     d.FileHash,
		Verified:     d.Verified,
		VerifiedBy:   d.VerifiedBy,
		UploadedAt:   d.UploadedAt.Format(timeFormat),
	}
	if d.VerifiedAt != nil {
		s := d.VerifiedAt.Format(timeFormat)
		r.VerifiedAt = &s
	}
	return r
}

func kycEventToResponse(e *domain.KYCEvent) KYCEventResponse {
	r := KYCEventResponse{
		ID:        e.ID,
		EventType: string(e.EventType),
		ActorID:   e.ActorID,
		NewStatus: string(e.NewStatus),
		Reason:    e.Reason,
		Metadata:  e.Metadata,
		TraceID:   e.TraceID,
		CreatedAt: e.CreatedAt.Format(timeFormat),
	}
	if e.OldStatus != nil {
		s := string(*e.OldStatus)
		r.OldStatus = &s
	}
	return r
}

func kycRequirementsToResponse(req *service.KYCRequirements) KYCRequirementsResponse {
	r := KYCRequirementsResponse{
		CurrentTier:   int(req.CurrentTier),
		CurrentStatus: string(req.CurrentStatus),
	}
	for _, d := range req.RequiredDocs {
		r.RequiredDocs = append(r.RequiredDocs, string(d))
	}
	for _, d := range req.SubmittedDocs {
		r.SubmittedDocs = append(r.SubmittedDocs, string(d))
	}
	for _, d := range req.MissingDocs {
		r.MissingDocs = append(r.MissingDocs, string(d))
	}
	return r
}

// KYCRiskDashboardResponse is returned by GET /api/v1/admin/kyc/risk-dashboard.
type KYCRiskDashboardResponse struct {
	QueueDepth              int64            `json:"queue_depth"`
	AvgReviewTimeSecs       float64          `json:"avg_review_time_secs"`
	TierDistribution        map[string]int64 `json:"tier_distribution"`
	FrozenBalanceTotalCents int64            `json:"frozen_balance_total_cents"`
	PEPFlagCount            int64            `json:"pep_flag_count"`
	SanctionsFlagCount      int64            `json:"sanctions_flag_count"`
}

func riskDashboardToResponse(s *domain.KYCRiskDashboardStats) KYCRiskDashboardResponse {
	tierDist := make(map[string]int64, len(s.TierDistribution))
	for tier, cnt := range s.TierDistribution {
		tierDist[fmt.Sprintf("%d", int(tier))] = cnt
	}
	return KYCRiskDashboardResponse{
		QueueDepth:              s.QueueDepth,
		AvgReviewTimeSecs:       s.AvgReviewTimeSecs,
		TierDistribution:        tierDist,
		FrozenBalanceTotalCents: s.FrozenBalanceTotalCents,
		PEPFlagCount:            s.PEPFlagCount,
		SanctionsFlagCount:      s.SanctionsFlagCount,
	}
}

func frozenBalanceToResponse(s *domain.FrozenBalanceSummary) FrozenBalanceResponse {
	return FrozenBalanceResponse{
		UserID:            s.UserID,
		UserName:          s.UserName,
		UserEmail:         s.UserEmail,
		KYCStatus:         string(s.KYCStatus),
		KYCTier:           int(s.KYCTier),
		FrozenAmountCents: s.FrozenAmountCents,
		FrozenReason:      s.FrozenReason,
		FrozenSince:       s.FrozenSince.Format(timeFormat),
	}
}
