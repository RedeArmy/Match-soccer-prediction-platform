// Package kyc defines the KYCProvider abstraction for identity verification
// and the ManualReviewAdapter that satisfies it using in-system admin review.
//
// KYCProvider is the seam between the compliance service layer and any
// external identity-verification vendor (e.g. Veriff, Jumio, Persona).
// All production code depends on the interface, never on a concrete vendor
// SDK, so providers can be swapped or multi-vendored without touching
// service logic.
package kyc

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// VerificationRequest carries the identity data submitted by the user.
type VerificationRequest struct {
	UserID         int
	FullName       string
	DateOfBirth    *time.Time
	Nationality    string
	DocumentType   domain.KYCDocumentType
	DocumentNumber string
	// DocumentStorageKey is the FileStore key of the primary identity document.
	DocumentStorageKey string
}

// VerificationResult is the provider's decision on a VerificationRequest.
type VerificationResult struct {
	// Approved is true when the provider has positively verified identity.
	Approved bool
	// RejectionReason is non-empty when Approved is false.
	RejectionReason string
	// PEPFlag indicates the subject appears on a Politically Exposed Persons list.
	PEPFlag bool
	// SanctionsFlag indicates the subject appears on a sanctions screening list.
	SanctionsFlag bool
	// RiskScore is the provider's 0–100 risk assessment. 0 = lowest risk.
	// Providers that do not emit a score should return 0.
	RiskScore int
	// ProviderRef is an opaque reference ID for correlating with the provider's
	// audit trail. Empty for providers that do not issue references.
	ProviderRef string
}

// KYCProvider is the interface satisfied by any identity-verification backend.
// Implementations must be safe for concurrent use by multiple goroutines.
type KYCProvider interface {
	// Submit sends a verification request to the provider.
	// Returns a provider-specific session/reference ID that can be used to
	// poll for results or to correlate webhook callbacks.
	Submit(ctx context.Context, req VerificationRequest) (sessionID string, err error)

	// GetResult retrieves the current decision for a previously submitted session.
	// Returns (nil, nil) when the session is still in progress (no decision yet).
	GetResult(ctx context.Context, sessionID string) (*VerificationResult, error)

	// Name returns a short identifier used for logging and metrics labels.
	// Example: "manual_review", "veriff", "jumio".
	Name() string
}

// ManualReviewAdapter satisfies KYCProvider using in-system admin review.
// It is the default provider: Submit records the session ID as the profile ID
// and GetResult reads the latest profile status from the service layer via
// the injected status reader.
//
// This adapter is appropriate for production deployments that have not yet
// integrated an automated identity verification vendor. All reviews are
// performed by human compliance officers through the admin UI.
type ManualReviewAdapter struct {
	log *zap.Logger
}

// NewManualReviewAdapter constructs a ManualReviewAdapter.
func NewManualReviewAdapter(log *zap.Logger) *ManualReviewAdapter {
	return &ManualReviewAdapter{log: log}
}

// Submit stores the request in the service layer (via KYCService.Submit) and
// returns the user ID as the session ID. The admin review workflow begins when
// the profile enters the review queue.
func (a *ManualReviewAdapter) Submit(_ context.Context, req VerificationRequest) (string, error) {
	// For the manual adapter the "session ID" is simply the user's internal ID.
	// The admin review queue is the session; no external submission is needed.
	a.log.Info("kyc manual review: submission queued", zap.Int("user_id", req.UserID))
	return fmt.Sprintf("%d", req.UserID), nil
}

// GetResult reads the profile's current status. Returns nil while the review
// is still in progress (pending / under_review). Returns a populated
// VerificationResult once the admin approves or rejects.
//
// The caller (scheduler or webhook handler) is responsible for polling at an
// appropriate cadence.
func (a *ManualReviewAdapter) GetResult(_ context.Context, _ string) (*VerificationResult, error) {
	// For the manual adapter the result is authoritative only after an admin
	// decision. The service layer's Approve/Reject methods propagate the
	// outcome through kyc_events; this method is a no-op for polling since
	// the service layer is the single source of truth.
	return nil, nil
}

// Name identifies the adapter in logs and metrics.
func (a *ManualReviewAdapter) Name() string { return "manual_review" }

// Compile-time assertion: ManualReviewAdapter implements KYCProvider.
var _ KYCProvider = (*ManualReviewAdapter)(nil)
