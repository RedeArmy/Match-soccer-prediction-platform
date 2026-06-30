// Package auth defines the IdentityProvider contract for JWT validation.
//
// This abstraction decouples HTTP middleware from any specific identity provider
// (Clerk, Auth0, AWS Cognito, Google Identity Platform, etc.).  The middleware
// layer consumes the IdentityProvider interface; concrete implementations live
// in this package and can be supplied by the wiring layer for custom providers.
//
// Sentinel errors give the caller enough context to map provider failures to
// the correct HTTP status without coupling the interface to HTTP semantics:
//
//	ErrProviderUnavailable → 503 Service Unavailable (transient outage)
//	ErrInvalidToken        → 401 Unauthorised (bad or expired credentials)
package auth

import (
	"context"
	"errors"
	"time"
)

// DefaultWarmupTimeout is the upper bound for the initial JWKS prefetch at
// startup. A Clerk outage at boot does not block the process indefinitely;
// the cache retries on the first request after a failed warmup.
const DefaultWarmupTimeout = 5 * time.Second

// ErrProviderUnavailable is returned by ValidateToken when the identity
// provider is temporarily unreachable (e.g. the JWKS endpoint is down or
// returning 5xx responses). The middleware should respond with 503 Service
// Unavailable rather than 401 Unauthorised, since the client's credentials
// may well be valid — the provider simply cannot verify them right now.
var ErrProviderUnavailable = errors.New("identity provider temporarily unavailable")

// ErrInvalidToken is returned by ValidateToken when the token is
// syntactically malformed, carries an invalid signature, or has expired.
// The middleware should respond with 401 Unauthorised.
var ErrInvalidToken = errors.New("token is invalid or expired")

// Claims holds the verified fields extracted from a validated JWT.
// These values are stored in the request context by RequireAuth and are
// available to downstream middleware and handlers.
type Claims struct {
	// Subject is the identity-provider's opaque principal identifier (JWT "sub").
	// For Clerk tokens this is the Clerk user_id (e.g. "user_2abc…").
	Subject string
	// IssuedAt is when this specific JWT was minted (JWT "iat"). Clerk refreshes
	// short-lived tokens (~60 s) automatically, so IssuedAt is always recent and
	// must NOT be used alone for session-age enforcement.
	IssuedAt time.Time
	// SessionStartedAt is when the user's session was first authenticated.
	// Derived from Clerk's "fva" (factors_verified_at) claim in v2 JWTs:
	//   fva[0] = seconds-since-first-factor-verified relative to iat
	//   SessionStartedAt = iat − fva[0]
	// This value persists across JWT refreshes and correctly represents the
	// session origin for max-age enforcement. Zero when fva is absent.
	SessionStartedAt time.Time
	// SessionID is the provider's session identifier (JWT "sid"). Used by
	// PolicyProvider to check the local revocation blocklist on logout.
	// Empty when the JWT does not carry a "sid" claim (e.g. test tokens).
	SessionID string
	// MFAVerifiedAt is when the user completed the second authentication factor.
	// Derived from Clerk's fva[1]: MFAVerifiedAt = iat − fva[1].
	// Zero when fva is absent, fva has fewer than 2 elements, or fva[1] is
	// negative (second factor not yet completed, e.g. during MFA step-up flow).
	// PolicyProvider enforces this when auth.require_mfa is true.
	MFAVerifiedAt time.Time
}

// IdentityProvider validates a raw Bearer token and returns the verified Claims
// for the authenticated principal. The subject is stored in the request context
// and used downstream to resolve the internal User row (e.g. via GetByExternalSubject).
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
// Error convention: wrap ErrProviderUnavailable for transient outages and
// ErrInvalidToken for authentication failures. Callers use errors.Is to
// select the appropriate HTTP response.
type IdentityProvider interface {
	ValidateToken(ctx context.Context, rawToken string) (Claims, error)
}
