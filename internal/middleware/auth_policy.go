package middleware

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/pkg/auth"
)

// IntGetter is the subset of SystemParamService consumed by PolicyProvider.
// Using a narrow interface prevents an import cycle between middleware and service.
type IntGetter interface {
	GetInt(ctx context.Context, key string, def int) int
}

// SessionBlocklist checks whether a Clerk session ID has been locally revoked.
// PostgresSessionRepository satisfies this interface.
type SessionBlocklist interface {
	IsRevoked(ctx context.Context, sid string) (bool, error)
}

// PolicyProvider wraps an IdentityProvider with system-controlled session policies:
//
//  1. Maximum session age — tokens whose "iat" is older than
//     auth.session_max_age_seconds (default 7 days) are rejected with 401.
//     This lets the system enforce shorter lifetimes than Clerk's configured
//     value without touching Clerk settings or paying for a plan upgrade.
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
	inner     auth.IdentityProvider
	blocklist SessionBlocklist
	params    IntGetter
	log       *zap.Logger
}

// NewPolicyProvider wraps inner with the two session-policy enforcement layers.
// blocklist may be nil to disable revocation checks (used in tests or when the
// revoked_sessions table is not yet migrated).
func NewPolicyProvider(
	inner auth.IdentityProvider,
	blocklist SessionBlocklist,
	params IntGetter,
	log *zap.Logger,
) auth.IdentityProvider {
	return &PolicyProvider{
		inner:     inner,
		blocklist: blocklist,
		params:    params,
		log:       log,
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
	if time.Since(claims.IssuedAt) > time.Duration(maxAgeSecs)*time.Second {
		return auth.Claims{}, fmt.Errorf("%w: session exceeded maximum age of %d seconds", auth.ErrInvalidToken, maxAgeSecs)
	}

	if claims.SessionID != "" && p.blocklist != nil {
		revoked, checkErr := p.blocklist.IsRevoked(ctx, claims.SessionID)
		if checkErr != nil {
			p.log.Warn("session blocklist check failed; treating as not revoked (fail-open)",
				zap.String("sid", claims.SessionID),
				zap.Error(checkErr),
			)
		} else if revoked {
			return auth.Claims{}, fmt.Errorf("%w: session has been revoked", auth.ErrInvalidToken)
		}
	}

	return claims, nil
}

var _ auth.IdentityProvider = (*PolicyProvider)(nil)
