package middleware

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/auth"
)

// GetInter is the subset of SystemParamService consumed by PolicyProvider.
// Using a narrow interface prevents an import cycle between middleware and service.
type GetInter interface {
	GetInt(ctx context.Context, key string, def int) int
}

// IsRevoker checks whether a Clerk session ID has been locally revoked.
// PostgresSessionRepository satisfies this interface.
type IsRevoker interface {
	IsRevoked(ctx context.Context, sid string) (bool, error)
}

// SessionStarter records the first-seen timestamp for a Clerk session ID.
// PostgresSessionStartRepository satisfies this interface.
// It is used as a fallback when the JWT lacks the fva claim (e.g. OAuth flows).
type SessionStarter interface {
	UpsertSessionStart(ctx context.Context, sid string) (startedAt time.Time, err error)
}

// PolicyProvider wraps an IdentityProvider with system-controlled session policies:
//
//  1. Maximum session age — tokens are rejected when the session origin exceeds
//     auth.session_max_age_seconds (default 7 days). The origin is taken from
//     Clerk's fva[0] claim (stable across JWT refreshes). When fva is absent
//     (OAuth / social / automatic-login flows), the origin is fetched from the
//     session_starts table (see SessionStarter). Falling back to IssuedAt would
//     make the check a no-op because Clerk refreshes JWTs every ~60 s.
//
//  2. Local revocation blocklist — tokens whose "sid" appears in the local
//     revoked_sessions table are rejected with 401. This implements logout:
//     the POST /api/v1/auth/logout handler inserts the current sid so that
//     all subsequent requests carrying the same token are rejected immediately,
//     even before the token's cryptographic expiry or Clerk's own invalidation.
//
// Clerk still issues and signs every token; this layer only decides whether to
// honour a token that Clerk considers valid. The two mechanisms are complementary:
// max-age gives system-wide session-lifetime control; the blocklist gives
// per-session immediate revocation.
//
// PolicyProvider is constructed in Routes() and replaces the raw JWKSProvider
// in RequireAuth. It implements IdentityProvider so RequireAuth is unmodified.
type PolicyProvider struct {
	inner         auth.IdentityProvider
	blocklist     IsRevoker
	sessionStarts SessionStarter // nil disables DB-backed session-start tracking
	params        GetInter
	log           *zap.Logger
	revokedCache  sync.Map // string → struct{}; SIDs confirmed revoked by the DB
}

// NewPolicyProvider wraps inner with the two session-policy enforcement layers.
// blocklist may be nil to disable revocation checks (used in tests or when the
// revoked_sessions table is not yet migrated).
// sessionStarts may be nil to disable DB-backed session-start tracking; when nil
// and fva is absent, max-age falls back to IssuedAt (preserving legacy behaviour).
func NewPolicyProvider(
	inner auth.IdentityProvider,
	blocklist IsRevoker,
	params GetInter,
	log *zap.Logger,
	opts ...PolicyOption,
) auth.IdentityProvider {
	p := &PolicyProvider{
		inner:     inner,
		blocklist: blocklist,
		params:    params,
		log:       log,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PolicyOption configures optional behaviour for PolicyProvider.
type PolicyOption func(*PolicyProvider)

// WithSessionStarter enables DB-backed session-start tracking via the provided
// SessionStarter. When set, PolicyProvider records the first-seen timestamp for
// sessions that lack the fva claim and uses it for max-age enforcement instead
// of falling back to IssuedAt.
func WithSessionStarter(ss SessionStarter) PolicyOption {
	return func(p *PolicyProvider) {
		p.sessionStarts = ss
	}
}

// ValidateToken delegates cryptographic validation to the inner provider, then
// applies local session policies in order: max-age check, then blocklist check.
// Either check failing returns ErrInvalidToken so RequireAuth maps it to 401.
func (p *PolicyProvider) ValidateToken(ctx context.Context, rawToken string) (auth.Claims, error) {
	claims, err := p.inner.ValidateToken(ctx, rawToken)
	if err != nil {
		return auth.Claims{}, err
	}

	maxAgeSecs := p.params.GetInt(ctx, domain.ParamKeyAuthSessionMaxAgeSecs, domain.DefaultAuthSessionMaxAgeSecs)
	// Resolve the session origin for max-age enforcement.
	// Priority:
	//   1. fva[0] claim (SessionStartedAt) — stable across JWT refreshes, present
	//      for password / passkey / email-code login flows.
	//   2. DB-backed session_starts table — used when fva is absent (OAuth / social
	//      / automatic-login). UpsertSessionStart records NOW() on first call and
	//      returns the stored value on subsequent calls.
	//   3. IssuedAt — last-resort fallback when neither fva nor DB tracking is
	//      available (e.g. test tokens, sessionStarts=nil). Logs a warning because
	//      this makes max-age enforcement inaccurate for long-lived sessions.
	sessionStart := claims.SessionStartedAt
	if sessionStart.IsZero() && claims.SessionID != "" && p.sessionStarts != nil {
		t, err := p.sessionStarts.UpsertSessionStart(ctx, claims.SessionID)
		if err != nil {
			p.log.Warn("session_starts upsert failed; falling back to IssuedAt for max-age (enforcement may be inaccurate)",
				zap.String("sid", claims.SessionID),
				zap.Error(err),
			)
			sessionStart = claims.IssuedAt
		} else {
			sessionStart = t
		}
	} else if sessionStart.IsZero() {
		// fva absent and no DB tracker configured. Log when sid is present so
		// the operator knows max-age enforcement is degraded for this session type.
		if claims.SessionID != "" {
			p.log.Warn("fva claim absent and session_starts tracker not configured; using IssuedAt for max-age (inaccurate for OAuth sessions)",
				zap.String("sid", claims.SessionID),
			)
		}
		sessionStart = claims.IssuedAt
	}
	if time.Since(sessionStart) > time.Duration(maxAgeSecs)*time.Second {
		return auth.Claims{}, fmt.Errorf("%w: session exceeded maximum age of %d seconds", auth.ErrInvalidToken, maxAgeSecs)
	}

	if claims.SessionID != "" && p.blocklist != nil {
		revoked, checkErr := p.blocklist.IsRevoked(ctx, claims.SessionID)
		if checkErr != nil {
			// DB failure: check whether we previously confirmed this SID was
			// revoked.  If so, fail-closed to prevent a revoked session from
			// regaining access during a DB outage.  Unknown SIDs remain
			// fail-open so a DB outage doesn't lock out all active users.
			if _, cached := p.revokedCache.Load(claims.SessionID); cached {
				return auth.Claims{}, fmt.Errorf("%w: session has been revoked", auth.ErrInvalidToken)
			}
			p.log.Warn("session blocklist check failed; treating as not revoked (fail-open for uncached session)",
				zap.String("sid", claims.SessionID),
				zap.Error(checkErr),
			)
		} else if revoked {
			p.revokedCache.Store(claims.SessionID, struct{}{})
			return auth.Claims{}, fmt.Errorf("%w: session has been revoked", auth.ErrInvalidToken)
		}
	}

	return claims, nil
}

var _ auth.IdentityProvider = (*PolicyProvider)(nil)
