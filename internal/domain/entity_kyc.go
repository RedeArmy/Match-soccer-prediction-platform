package domain

import "time"

// ── KYC tier ─────────────────────────────────────────────────────────────────

// KYCTier represents the identity-verification level of a user.
// Tiers gate money-movement operations: higher tiers unlock larger limits.
// The zero value (KYCTierUnverified) is the default for every new account.
type KYCTier int

const (
	// KYCTierUnverified — email only (via Clerk). No money movement permitted.
	KYCTierUnverified KYCTier = 0
	// KYCTierOne — phone verified. Low-limit deposit and payout allowed.
	KYCTierOne KYCTier = 1
	// KYCTierTwo — government photo ID + selfie verified. Mid-limit allowed.
	KYCTierTwo KYCTier = 2
	// KYCTierThree — full KYC (ID + selfie + proof of address). Unlimited
	// with manual review for transactions above the AML reporting threshold.
	KYCTierThree KYCTier = 3
)

// ── KYC profile status ────────────────────────────────────────────────────────

// KYCStatus is the lifecycle state of a KYC profile submission.
type KYCStatus string

const (
	// KYCStatusUnverified — no profile submitted yet; initial state.
	KYCStatusUnverified KYCStatus = "unverified"
	// KYCStatusPending — profile submitted, awaiting admin pick-up.
	KYCStatusPending KYCStatus = "pending"
	// KYCStatusUnderReview — admin has claimed the profile for review.
	KYCStatusUnderReview KYCStatus = "under_review"
	// KYCStatusApproved — profile reviewed and identity confirmed.
	KYCStatusApproved KYCStatus = "approved"
	// KYCStatusRejected — profile rejected; user may resubmit.
	KYCStatusRejected KYCStatus = "rejected"
	// KYCStatusExpired — approved profile has passed its re-verification
	// deadline and is no longer valid; user must resubmit.
	KYCStatusExpired KYCStatus = "expired"
	// KYCStatusEscalated — profile flagged for compliance review; a senior
	// reviewer must act before approval or rejection.
	KYCStatusEscalated KYCStatus = "escalated"
)

// ── KYC document type ─────────────────────────────────────────────────────────

// KYCDocumentType identifies the category of an uploaded identity document.
type KYCDocumentType string

const (
	// KYCDocGovID is a national ID, passport, or driver's licence.
	KYCDocGovID KYCDocumentType = "gov_id"
	// KYCDocSelfie is a liveness selfie photograph.
	KYCDocSelfie KYCDocumentType = "selfie"
	// KYCDocProofOfAddress is a utility bill or bank statement.
	KYCDocProofOfAddress KYCDocumentType = "proof_of_address"
	// KYCDocProofOfFunds is a bank statement or payslip.
	KYCDocProofOfFunds KYCDocumentType = "proof_of_funds"
)

// KYCProfileType identifies the profile table referenced by kyc_documents and kyc_events.
type KYCProfileType string

const (
	// KYCProfileTypeUser is an individual user KYC profile.
	KYCProfileTypeUser KYCProfileType = "user"
)

// ── KYC event type ────────────────────────────────────────────────────────────

// KYCEventType records the action that triggered a kyc_events row.
type KYCEventType string

const (
	// KYCEventSubmitted records a new or resubmitted profile.
	KYCEventSubmitted KYCEventType = "submitted"
	// KYCEventUnderReview records a profile entering manual review.
	KYCEventUnderReview KYCEventType = "under_review"
	// KYCEventApproved records an admin approval with tier assignment.
	KYCEventApproved KYCEventType = "approved"
	// KYCEventRejected records an admin rejection with reason.
	KYCEventRejected KYCEventType = "rejected"
	// KYCEventEscalated records escalation to senior compliance review.
	KYCEventEscalated KYCEventType = "escalated"
	// KYCEventExpired records a profile lapsing past its review date.
	KYCEventExpired KYCEventType = "expired"
	// KYCEventTierChanged records a manual tier adjustment.
	KYCEventTierChanged KYCEventType = "tier_changed"
	// KYCEventDocRequested records an admin request for additional documents.
	KYCEventDocRequested KYCEventType = "doc_requested"
	// KYCEventFrozen records a balance freeze pending KYC completion.
	KYCEventFrozen KYCEventType = "frozen"
	// KYCEventUnfrozen records the release of a frozen balance.
	KYCEventUnfrozen KYCEventType = "unfrozen"
)

// ── KYC profile (individual) ──────────────────────────────────────────────────

// KYCProfile holds the identity verification data for one user.
//
// One user has at most one active profile at a time. When a user resubmits
// after rejection the existing row is updated in place; the kyc_events table
// records the full history of status transitions.
//
// BalanceFrozen is set to true when a large win is credited and the user has
// not yet reached the required tier to release funds (see KYCGate). It is
// cleared by the admin via the release-frozen-balance endpoint after approval.
type KYCProfile struct {
	ID                int
	UserID            int
	Status            KYCStatus
	Tier              KYCTier
	FullName          string
	DateOfBirth       *time.Time
	Nationality       string
	DocumentType      *KYCDocumentType
	DocumentNumber    string
	AddressLine       string
	City              string
	Country           string
	PostalCode        string
	SubmittedAt       *time.Time
	ReviewedAt        *time.Time
	ReviewedBy        *int       // admin user ID
	RejectionReason   string     // non-empty when status = rejected
	RiskScore         int        // 0–100; recalculated on each financial event
	DeviceFingerprint *string    // SHA-256 hex digest of the client's device fingerprint; nil if not submitted
	PEPFlag           bool       // Politically Exposed Person
	SanctionsFlag     bool       // matched against sanctions list
	BalanceFrozen     bool       // true when balance is held pending KYC tier upgrade
	FrozenAmountCents int        // amount frozen (0 when not frozen)
	FrozenReason      string     // human-readable freeze trigger description
	NextReviewAt      *time.Time // periodic re-verification deadline
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ── KYC document ─────────────────────────────────────────────────────────────

// KYCDocument records one uploaded identity document attached to a KYC profile.
// Binary content is never stored in the database; StorageKey is an opaque
// reference into the configured FileStore (e.g. "kyc/2026/05/abc.pdf").
//
// FileHash is the hex-encoded SHA-256 digest of the raw file bytes, computed
// by the handler before the file is streamed to the FileStore. It enables
// integrity verification on retrieval and deduplication detection.
type KYCDocument struct {
	ID           int64
	ProfileID    int
	ProfileType  KYCProfileType
	DocumentType KYCDocumentType
	StorageKey   string
	ContentType  string
	FileSize     int
	FileHash     string // hex-encoded SHA-256
	Verified     bool
	VerifiedAt   *time.Time
	VerifiedBy   *int // admin user ID
	UploadedAt   time.Time
}

// ── KYC event (audit log) ─────────────────────────────────────────────────────

// KYCEvent is an immutable audit record for one KYC state-machine transition.
// Rows are append-only; they are never updated or deleted.
//
// OldStatus and NewStatus capture the before/after state. ActorID is nil for
// system-generated events (e.g. automatic expiry). TraceID links the event to
// the distributed trace that caused it.
type KYCEvent struct {
	ID          int64
	ProfileID   int
	ProfileType KYCProfileType
	EventType   KYCEventType
	ActorID     *int // nil = system
	OldStatus   *KYCStatus
	NewStatus   KYCStatus
	Reason      string
	Metadata    map[string]any
	TraceID     string
	CreatedAt   time.Time
}

// ── Frozen balance summary ────────────────────────────────────────────────────

// FrozenBalanceSummary is a read-only admin projection of one frozen account.
type FrozenBalanceSummary struct {
	UserID            int
	UserName          string
	UserEmail         string
	KYCStatus         KYCStatus
	KYCTier           KYCTier
	FrozenAmountCents int
	FrozenReason      string
	FrozenSince       time.Time // profile updated_at when freeze was applied
}

// ── Risk dashboard ────────────────────────────────────────────────────────────

// KYCRiskDashboardStats is the aggregate view returned by the admin risk
// dashboard endpoint. All counts are computed in a single DB query.
type KYCRiskDashboardStats struct {
	QueueDepth              int64             `json:"queue_depth"`
	AvgReviewTimeSecs       float64           `json:"avg_review_time_secs"`
	TierDistribution        map[KYCTier]int64 `json:"tier_distribution"`
	FrozenBalanceTotalCents int64             `json:"frozen_balance_total_cents"`
	PEPFlagCount            int64             `json:"pep_flag_count"`
	SanctionsFlagCount      int64             `json:"sanctions_flag_count"`
}
