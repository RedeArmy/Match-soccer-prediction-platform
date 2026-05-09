package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
)

// JWKSProvider validates RS256-signed JWTs against public keys fetched from a
// JWKS endpoint. It is compatible with any OIDC-compliant identity provider
// that publishes a JWKS URL: Clerk, Auth0, AWS Cognito, and Google Identity
// Platform all follow this standard.
//
// Keys are fetched eagerly at construction time and cached in memory. The
// last known-good keyset is retained as a fallback so that a transient
// provider outage does not immediately reject requests that carry valid,
// recently-issued, unexpired tokens. Key rotations only happen every ~24 h,
// so a stale keyset is safe to use during short outages.
type JWKSProvider struct {
	jwkCache   *jwk.Cache
	jwksURL    string
	log        *zap.Logger
	fallbackMu sync.RWMutex
	fallback   jwk.Set
}

// NewJWKSProvider constructs a JWKSProvider and eagerly warms the JWKS cache.
//
// If jwksURL is empty the returned provider is fail-closed: every call to
// ValidateToken returns ErrProviderUnavailable. This mirrors the expected
// behaviour in a misconfigured deployment — fail visibly rather than grant
// unauthenticated access.
//
// warmupTimeout caps the initial cache prefetch so that a provider outage at
// startup does not block the process indefinitely. The cache retries on the
// first request after a failed warmup.
func NewJWKSProvider(jwksURL string, warmupTimeout time.Duration, log *zap.Logger) IdentityProvider {
	if jwksURL == "" {
		log.Error("auth: JWKS URL is not configured — all requests will be rejected; set WCQ_CLERK_JWKSURL")
		return &failClosedProvider{msg: "authentication is not configured"}
	}

	cache := jwk.NewCache(context.Background())
	if err := cache.Register(jwksURL); err != nil {
		log.Error("auth: failed to register JWKS URL", zap.String("url", jwksURL), zap.Error(err))
	}

	warmCtx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
	defer cancel()
	if _, err := cache.Refresh(warmCtx, jwksURL); err != nil {
		log.Warn("auth: JWKS prefetch failed; will retry on first request",
			zap.String("url", jwksURL), zap.Error(err))
	}

	p := &JWKSProvider{
		jwkCache: cache,
		jwksURL:  jwksURL,
		log:      log,
	}
	// Pre-populate the fallback from the warm-up fetch so it is available
	// immediately if the live endpoint becomes unreachable on the first request.
	if ks, err := cache.Get(context.Background(), jwksURL); err == nil {
		p.fallback = ks
	}
	return p
}

// ValidateToken fetches the current JWKS keyset, parses and validates the JWT,
// and returns the "sub" claim on success.
//
// When the JWKS endpoint is unreachable the last known-good keyset is used as
// a fallback. Returns ErrProviderUnavailable when no keyset is available at
// all (neither live nor cached). Returns ErrInvalidToken when the JWT is
// malformed, has an invalid signature, or has expired.
func (p *JWKSProvider) ValidateToken(ctx context.Context, rawToken string) (string, error) {
	keySet, err := p.jwkCache.Get(ctx, p.jwksURL)
	if err != nil {
		p.fallbackMu.RLock()
		fk := p.fallback
		p.fallbackMu.RUnlock()
		if fk != nil {
			p.log.Warn("auth: JWKS fetch failed; using cached keyset fallback",
				zap.Error(err))
			keySet = fk
		} else {
			p.log.Error("auth: JWKS fetch failed and no fallback available",
				zap.Error(err))
			return "", fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
		}
	} else {
		// Successful fetch: keep the fallback fresh.
		p.fallbackMu.Lock()
		p.fallback = keySet
		p.fallbackMu.Unlock()
	}

	token, err := jwt.Parse([]byte(rawToken), jwt.WithKeySet(keySet), jwt.WithValidate(true))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return token.Subject(), nil
}

// failClosedProvider is returned by NewJWKSProvider when the JWKS URL is
// empty. Every call to ValidateToken returns ErrProviderUnavailable, ensuring
// that a misconfigured deployment rejects all requests rather than granting
// open access.
type failClosedProvider struct {
	msg string
}

func (p *failClosedProvider) ValidateToken(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("%w: %s", ErrProviderUnavailable, p.msg)
}

var _ IdentityProvider = (*JWKSProvider)(nil)
var _ IdentityProvider = (*failClosedProvider)(nil)
