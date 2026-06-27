package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"
)

// KeysetPersister persists the raw JWKS JSON payload across process restarts.
// It is used as a second-level fallback when the live Clerk endpoint is
// unreachable at startup and the in-memory fallback is not yet populated.
//
// Redis is the recommended backing store; a TTL of 15 minutes provides
// resilience against short Clerk outages while ensuring keys do not become
// excessively stale relative to Clerk's ~24 h rotation cycle.
type KeysetPersister interface {
	// Save stores the JSON-serialised JWKS payload for later retrieval.
	Save(ctx context.Context, payload []byte, ttl time.Duration) error
	// Load retrieves the last-saved JWKS payload. Returns (nil, nil) when no
	// persisted keyset is available (cache miss or TTL expired).
	Load(ctx context.Context) ([]byte, error)
}

// Option configures JWKSProvider at construction time.
type Option func(*JWKSProvider)

// WithOnDegraded registers a hook invoked each time ValidateToken falls back to
// the in-memory keyset because the live JWKS endpoint is unreachable. Callers
// use this to increment an OTel counter so that Clerk outages are observable in
// Grafana without relying on log parsing.
func WithOnDegraded(fn func(ctx context.Context)) Option {
	return func(p *JWKSProvider) { p.onDegraded = fn }
}

// WithKeysetPersister wires a KeysetPersister for cross-restart fallback.
//
// On every successful JWKS fetch the serialised keyset is written to the
// persister with the given TTL. On startup, if all warmup attempts fail and
// the in-memory fallback is still nil, the persister is queried: a valid
// persisted payload seeds the fallback so that existing, unexpired user tokens
// can continue to be validated during a Clerk outage — even after a process
// restart.
//
// The TTL controls how long the persisted keyset remains valid. A value of
// 15 m covers typical short Clerk degradations; operators may increase it
// towards 24 h to match Clerk's key rotation period for longer resilience.
func WithKeysetPersister(p KeysetPersister, ttl time.Duration) Option {
	return func(j *JWKSProvider) {
		j.persister = p
		j.persisterTTL = ttl
	}
}

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
//
// For cross-restart resilience, an optional KeysetPersister (e.g. Redis) can
// be injected via WithKeysetPersister. When present, the keyset is written on
// every successful fetch and loaded as a last-resort fallback at startup.
type JWKSProvider struct {
	jwkCache     *jwk.Cache
	jwksURL      string
	log          *zap.Logger
	fallbackMu   sync.RWMutex
	fallback     jwk.Set
	onDegraded   func(ctx context.Context) // optional; called when using the fallback keyset
	persister    KeysetPersister           // optional; nil means no cross-restart persistence
	persisterTTL time.Duration
}

// NewJWKSProvider constructs a JWKSProvider and eagerly warms the JWKS cache.
//
// ctx is used for the startup warmup fetch and the initial fallback population.
// It should carry any values (OTel trace IDs, request IDs) the caller wants
// propagated into those one-time operations. Its cancellation signal is NOT
// used for the internal background refresh goroutine, which intentionally runs
// for the lifetime of the process regardless of ctx cancellation.
//
// If jwksURL is empty the returned provider is fail-closed: every call to
// ValidateToken returns ErrProviderUnavailable. This mirrors the expected
// behaviour in a misconfigured deployment — fail visibly rather than grant
// unauthenticated access.
//
// warmupTimeout caps the initial cache prefetch so that a provider outage at
// startup does not block the process indefinitely. The cache retries on the
// first request after a failed warmup.
func NewJWKSProvider(ctx context.Context, jwksURL string, warmupTimeout time.Duration, log *zap.Logger, opts ...Option) IdentityProvider {
	if jwksURL == "" {
		log.Error("auth: JWKS URL is not configured — all requests will be rejected; set WCQ_CLERK_JWKSURL")
		return &failClosedProvider{msg: "authentication is not configured"}
	}

	// context.WithoutCancel strips the cancellation signal so the internal
	// refresh goroutine outlives any specific request or startup context, while
	// still inheriting OTel trace values carried by ctx.
	jwkCache := jwk.NewCache(context.WithoutCancel(ctx))
	if err := jwkCache.Register(jwksURL); err != nil {
		log.Error("auth: failed to register JWKS URL", zap.String("url", jwksURL), zap.Error(err))
	}

	p := &JWKSProvider{jwkCache: jwkCache, jwksURL: jwksURL, log: log}
	for _, opt := range opts {
		opt(p)
	}

	// Retry the warmup up to 3 times with independent timeouts. The Clerk JWKS
	// endpoint can return stale Cache-Control headers (age > max-age), which causes
	// httprc to re-fetch synchronously inside Refresh(), sometimes exceeding a
	// single 5 s window. Independent per-attempt contexts prevent a slow first
	// attempt from consuming the entire budget.
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		warmCtx, cancel := context.WithTimeout(ctx, warmupTimeout)
		_, err := jwkCache.Refresh(warmCtx, jwksURL)
		cancel()
		if err == nil {
			break
		}
		log.Warn("auth: JWKS prefetch attempt failed",
			zap.Int("attempt", attempt),
			zap.Int("max_attempts", maxAttempts),
			zap.String("url", jwksURL),
			zap.Error(err))
		if attempt == maxAttempts {
			log.Warn("auth: all JWKS prefetch attempts failed; background goroutine will retry — authenticated requests may fail transiently")
		}
	}

	// Pre-populate the fallback from the warm-up fetch so it is available
	// immediately if the live endpoint becomes unreachable on the first request.
	if ks, err := jwkCache.Get(ctx, jwksURL); err == nil {
		p.fallback = ks
		p.persistKeyset(ctx, ks)
	} else if p.persister != nil {
		// Warmup failed: attempt to seed the fallback from the persisted keyset.
		// This covers the restart-during-Clerk-outage scenario: even though the
		// live endpoint is down, previously validated requests can continue while
		// Clerk recovers, provided the persisted TTL has not yet expired.
		p.loadPersistedKeyset(ctx)
	}
	return p
}

// ValidateToken fetches the current JWKS keyset, parses and validates the JWT,
// and returns Claims on success. The returned Claims carry the "sub", "iat",
// and "sid" fields extracted from the verified token.
//
// When the JWKS endpoint is unreachable the last known-good keyset is used as
// a fallback. Returns ErrProviderUnavailable when no keyset is available at
// all (neither live nor cached). Returns ErrInvalidToken when the JWT is
// malformed, has an invalid signature, or has expired.
func (p *JWKSProvider) ValidateToken(ctx context.Context, rawToken string) (Claims, error) {
	keySet, err := p.jwkCache.Get(ctx, p.jwksURL)
	if err != nil {
		p.fallbackMu.RLock()
		fk := p.fallback
		p.fallbackMu.RUnlock()
		if fk != nil {
			p.log.Warn("auth: JWKS fetch failed; using cached keyset fallback",
				zap.Error(err))
			if p.onDegraded != nil {
				p.onDegraded(ctx)
			}
			keySet = fk
		} else {
			p.log.Error("auth: JWKS fetch failed and no fallback available",
				zap.Error(err))
			return Claims{}, fmt.Errorf("%w: %v", ErrProviderUnavailable, err)
		}
	} else {
		// Successful fetch: keep the in-memory and persisted fallbacks fresh.
		p.fallbackMu.Lock()
		p.fallback = keySet
		p.fallbackMu.Unlock()
		p.persistKeyset(ctx, keySet)
	}

	// WithAcceptableSkew tolerates up to 10 s of clock drift between the host
	// and Clerk's token-signing servers, matching the clockSkewInMs: 10_000
	// configured in the Next.js Clerk middleware.
	token, err := jwt.Parse(
		[]byte(rawToken),
		jwt.WithKeySet(keySet),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(10*time.Second),
	)
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}

	claims := Claims{
		Subject:  token.Subject(),
		IssuedAt: token.IssuedAt(),
	}
	if sid, ok := token.PrivateClaims()["sid"].(string); ok {
		claims.SessionID = sid
	}
	return claims, nil
}

// persistKeyset serialises ks to JSON and writes it to the persister, if one
// is configured. Errors are logged and swallowed: persistence is best-effort
// and must never block the request path.
func (p *JWKSProvider) persistKeyset(ctx context.Context, ks jwk.Set) {
	if p.persister == nil {
		return
	}
	raw, err := json.Marshal(ks)
	if err != nil {
		p.log.Warn("auth: failed to serialise JWKS for persistence", zap.Error(err))
		return
	}
	if err := p.persister.Save(ctx, raw, p.persisterTTL); err != nil {
		p.log.Warn("auth: failed to persist JWKS keyset", zap.Error(err))
	}
}

// loadPersistedKeyset attempts to populate the in-memory fallback from the
// persisted payload. Called once at startup when all warmup attempts fail.
func (p *JWKSProvider) loadPersistedKeyset(ctx context.Context) {
	raw, err := p.persister.Load(ctx)
	if err != nil {
		p.log.Warn("auth: failed to load persisted JWKS keyset", zap.Error(err))
		return
	}
	if len(raw) == 0 {
		return // cache miss — no persisted keyset available
	}
	ks, err := jwk.Parse(raw)
	if err != nil {
		p.log.Warn("auth: persisted JWKS payload is invalid", zap.Error(err))
		return
	}
	p.fallbackMu.Lock()
	p.fallback = ks
	p.fallbackMu.Unlock()
	p.log.Warn("auth: JWKS warmup failed; loaded persisted keyset as fallback",
		zap.Duration("ttl", p.persisterTTL))
}

// failClosedProvider is returned by NewJWKSProvider when the JWKS URL is
// empty. Every call to ValidateToken returns ErrProviderUnavailable, ensuring
// that a misconfigured deployment rejects all requests rather than granting
// open access.
type failClosedProvider struct {
	msg string
}

func (p *failClosedProvider) ValidateToken(_ context.Context, _ string) (Claims, error) {
	return Claims{}, fmt.Errorf("%w: %s", ErrProviderUnavailable, p.msg)
}

var _ IdentityProvider = (*JWKSProvider)(nil)
var _ IdentityProvider = (*failClosedProvider)(nil)
