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

// IdentityProvider validates a raw Bearer token and returns the provider's
// opaque identifier for the authenticated principal (the "subject"). The
// subject is stored in the request context and used downstream to resolve
// the internal User row (e.g. via GetByClerkSubject).
//
// Implementations must be safe for concurrent use by multiple goroutines.
//
// Error convention: wrap ErrProviderUnavailable for transient outages and
// ErrInvalidToken for authentication failures. Callers use errors.Is to
// select the appropriate HTTP response.
type IdentityProvider interface {
	ValidateToken(ctx context.Context, rawToken string) (subject string, err error)
}
