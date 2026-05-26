package domain

// ── KYC audit action constants ────────────────────────────────────────────────

const (
	AuditActionKYCSubmitted    = "kyc.submitted"
	AuditActionKYCUnderReview  = "kyc.under_review"
	AuditActionKYCApproved     = "kyc.approved"
	AuditActionKYCRejected     = "kyc.rejected"
	AuditActionKYCEscalated    = "kyc.escalated"
	AuditActionKYCExpired      = "kyc.expired"
	AuditActionKYCTierUpgraded = "kyc.tier_upgraded"
	AuditActionKYCDocUploaded  = "kyc.document_uploaded"
	AuditActionKYCDocRequested = "kyc.document_requested"
	AuditActionKYCFrozen       = "kyc.balance_frozen"
	AuditActionKYCUnfrozen     = "kyc.balance_unfrozen"
	AuditActionAMLFlagged      = "kyc.aml_flagged"
	AuditActionKYBSubmitted    = "kyb.submitted"
	AuditActionKYBApproved     = "kyb.approved"
	AuditActionKYBRejected     = "kyb.rejected"
)

// ── KYC system parameter keys ─────────────────────────────────────────────────

const (
	// Tier 1 (phone verified): deposit cap shared with Tier 0.
	// Tier 1 cannot withdraw — withdrawals require Tier 2+ (gov ID).
	ParamKeyKYCTier1DepositLimitCents = "kyc.tier1_deposit_limit_cents"

	// Tier 2 (gov ID + selfie): deposit and per-request payout caps.
	ParamKeyKYCTier2DepositLimitCents = "kyc.tier2_deposit_limit_cents"
	ParamKeyKYCTier2PayoutLimitCents  = "kyc.tier2_payout_limit_cents"

	// AML reporting threshold (GTQ; transactions above this require UAF record).
	ParamKeyKYCAMLThresholdCents = "kyc.aml_threshold_cents"

	// Re-verification interval in days (Tier 2 and Tier 3 profiles).
	ParamKeyKYCReviewIntervalDays = "kyc.review_interval_days"

	// Maximum document upload size in bytes.
	ParamKeyKYCMaxDocUploadBytes = "kyc.max_doc_upload_bytes"
)

// ── KYC system parameter defaults ────────────────────────────────────────────

const (
	// Tier 0 and Tier 1 share this per-transaction deposit cap (Q2,500).
	DefaultKYCTier1DepositLimitCents = 250_000 // 2,500 GTQ

	// Tier 2 caps: Q15,000 per deposit and per withdrawal.
	DefaultKYCTier2DepositLimitCents = 1_500_000 // 15,000 GTQ
	DefaultKYCTier2PayoutLimitCents  = 1_500_000 // 15,000 GTQ

	// Guatemalan UAF mandatory AML reporting threshold (Q25,000).
	DefaultKYCAMLThresholdCents = 2_500_000 // 25,000 GTQ

	// Annual re-verification for Tier 2 and Tier 3 profiles.
	DefaultKYCReviewIntervalDays = 365

	// Maximum size per KYC document upload: 10 MB.
	DefaultKYCMaxDocUploadBytes = 10_485_760 // 10 MB
)

// ── Allowed KYC document MIME types ──────────────────────────────────────────

// KYCAllowedContentTypes lists the MIME types accepted for KYC document
// uploads. The handler rejects any upload whose Content-Type is not in this
// set before streaming the bytes to the FileStore.
var KYCAllowedContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"application/pdf": true,
}
