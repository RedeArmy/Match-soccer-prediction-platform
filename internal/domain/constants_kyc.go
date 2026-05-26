package domain

// KYC audit action constants — recorded in the audit_log table by KYCService
// and money-movement services whenever a KYC lifecycle event occurs.
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
	// AuditActionAMLFlagged is written when a transaction meets or exceeds the
	// kyc.aml_threshold_cents param. It is non-blocking: the transaction commits
	// and the record is used for mandatory UAF reporting under Guatemalan law.
	AuditActionAMLFlagged = "kyc.aml_flagged"
)

// KYC system_params keys — all values are runtime-editable via the admin API
// without a server restart. Defaults are seeded by migration 000121.
const (
	// ParamKeyKYCTier1DepositLimitCents is the per-transaction deposit cap
	// (centavos) shared by Tier 0 (unverified) and Tier 1 (phone-verified) users.
	// Tier 1 is fully blocked from withdrawals; only deposits apply.
	ParamKeyKYCTier1DepositLimitCents = "kyc.tier1_deposit_limit_cents"

	// ParamKeyKYCTier2DepositLimitCents is the per-transaction deposit cap
	// (centavos) for Tier 2 (government ID + selfie approved) users.
	ParamKeyKYCTier2DepositLimitCents = "kyc.tier2_deposit_limit_cents"

	// ParamKeyKYCTier2PayoutLimitCents is the per-request withdrawal cap
	// (centavos) for Tier 2 users. Tier 3 is unlimited.
	ParamKeyKYCTier2PayoutLimitCents = "kyc.tier2_payout_limit_cents"

	// ParamKeyKYCAMLThresholdCents is the transaction amount (centavos) at or
	// above which a UAF AML report is mandatory under Guatemalan law.
	// Reaching the threshold writes an AuditActionAMLFlagged event; the
	// transaction is never rejected on this basis alone.
	ParamKeyKYCAMLThresholdCents = "kyc.aml_threshold_cents"

	// ParamKeyKYCReviewIntervalDays is the number of days after approval before
	// a Tier 2 or Tier 3 profile is due for re-verification. Written to
	// kyc_profiles.next_review_at by KYCService.Approve.
	ParamKeyKYCReviewIntervalDays = "kyc.review_interval_days"

	// ParamKeyKYCMaxDocUploadBytes is the maximum size in bytes for a single
	// KYC document upload. Enforced by KYCService.UploadDocument.
	ParamKeyKYCMaxDocUploadBytes = "kyc.max_doc_upload_bytes"

	// ParamKeyKYCRiskDashboardCacheTTLSec is the number of seconds the admin
	// risk dashboard response is cached. Seeded by migration 000125.
	ParamKeyKYCRiskDashboardCacheTTLSec = "kyc.risk_dashboard_cache_ttl_sec"
)

// KYC system_params default values — match the seeds in migration 000121 so
// the gate is fully functional even before the first admin configuration change.
const (
	// DefaultKYCTier1DepositLimitCents is Q2,500 (250,000 centavos).
	DefaultKYCTier1DepositLimitCents = 250_000

	// DefaultKYCTier2DepositLimitCents is Q15,000 (1,500,000 centavos).
	DefaultKYCTier2DepositLimitCents = 1_500_000

	// DefaultKYCTier2PayoutLimitCents is Q15,000 (1,500,000 centavos).
	DefaultKYCTier2PayoutLimitCents = 1_500_000

	// DefaultKYCAMLThresholdCents is Q25,000 (2,500,000 centavos), the
	// Guatemalan UAF mandatory reporting threshold.
	DefaultKYCAMLThresholdCents = 2_500_000

	// DefaultKYCReviewIntervalDays is 365 (annual re-verification).
	DefaultKYCReviewIntervalDays = 365

	// DefaultKYCMaxDocUploadBytes is 10,485,760 (10 MB per document).
	DefaultKYCMaxDocUploadBytes = 10_485_760

	// DefaultKYCRiskDashboardCacheTTLSecs is the number of seconds the risk
	// dashboard response is cached. 60 seconds balances freshness against DB load.
	// Matches the seed in migration 000125.
	DefaultKYCRiskDashboardCacheTTLSecs = 60
)

// CacheKeyKYCRiskDashboard is the cache key for the admin risk dashboard response.
const CacheKeyKYCRiskDashboard = "kyc:risk_dashboard"

// Velocity limit system_param keys — 24-hour rolling totals per KYC tier.
const (
	ParamKeyKYCTier1DepositVelocityCents    = "kyc.tier1_deposit_velocity_cents"
	ParamKeyKYCTier2DepositVelocityCents    = "kyc.tier2_deposit_velocity_cents"
	ParamKeyKYCTier1WithdrawalVelocityCents = "kyc.tier1_withdrawal_velocity_cents"
	ParamKeyKYCTier2WithdrawalVelocityCents = "kyc.tier2_withdrawal_velocity_cents"
)

// Default velocity limits in centavos (Guatemala quetzales × 100).
const (
	// DefaultKYCTier1DepositVelocityCents is Q50,000 per 24h for Tier 0/1 users.
	DefaultKYCTier1DepositVelocityCents = 5_000_000
	// DefaultKYCTier2DepositVelocityCents is Q200,000 per 24h for Tier 2 users.
	DefaultKYCTier2DepositVelocityCents = 20_000_000
	// DefaultKYCTier1WithdrawalVelocityCents is Q0 (withdrawals blocked below Tier 2).
	DefaultKYCTier1WithdrawalVelocityCents = 0
	// DefaultKYCTier2WithdrawalVelocityCents is Q100,000 per 24h for Tier 2 users.
	DefaultKYCTier2WithdrawalVelocityCents = 10_000_000
)

// KYCAllowedContentTypes lists the MIME types accepted for KYC document
// uploads. The handler rejects any upload whose Content-Type is not in this
// set before streaming the bytes to the FileStore.
var KYCAllowedContentTypes = map[string]bool{
	"image/jpeg":      true,
	"image/png":       true,
	"image/webp":      true,
	"application/pdf": true,
}
