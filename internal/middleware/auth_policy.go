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

// revokedCacheTTL is the maximum lifetime of an in-process revocation entry.
// After this window, the max-age check on the session origin would reject the
// request independently — keeping the entry longer provides no security value.
// 8 days ≈ default max-age (7 days) + 1 day grace, matching the DB prune cutoff.
const revokedCacheTTL = 8 * 24 * time.Hour

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
	UpsertSessionStart(ctx context.Context, sid, userID string) (startedAt time.Time, err error)
}

// PolicyProvider wraps an IdentityProvider with system-controlled session policies:
//
//  1. Maximum session age — tokens are rejected when the session origin exceeds
//     auth.session_max_age_seconds (default 7 days). The origin is taken from
//     Clerk's fva[0] claim (stable across JWT refreshes). When fva is absent
//     (OAuth / social / automatic-login flows), the origin is fetched from the
//     session_starts table via SessionStarter. An in-process cache (startCache)
//     absorbs repeated DB calls for the same sid across JWT refreshes.
//
//  2. Local revocation blocklist — tokens whose "sid" appears in the local
//     revoked_sessions table are rejected with 401. This implements logout:
//     the POST /api/v1/auth/logout handler inserts the current sid so that
//     all subsequent requests carrying the same token are rejected immediately,
//     even before the token's cryptographic expiry or Clerk's own invalidation.
//     A bounded in-process cache (revokedEntries) provides fail-closed protection
//     during DB outages for sessions previously confirmed revoked.
//
// Clerk still issues and signs every token; this layer only decides whether to
// honour a token that Clerk considers valid. The two mechanisms are complementary:
// max-age gives system-wide session-lifetime control; the blocklist gives
// per-session immediate revocation.
type PolicyProvider struct {
	inner         auth.IdentityProvider
	blocklist     IsRevoker
	sessionStarts SessionStarter // nil disables DB-backed session-start tracking
	params        GetInter
	log           *zap.Logger

	// startCache is a write-once, read-many in-process cache for OAuth session
	// origins (sid → startedAt). It avoids a DB round-trip on every authenticated
	// request for sessions whose JWT lacks the fva claim. Keys are never explicitly
	// removed — entries become unreachable once the session exceeds max-age and the
	// subsequent max-age check rejects the request before the cache is consulted.
	// sync.Map is appropriate: entries are written once per sid and read on every
	// subsequent request from the same session.
	startCache sync.Map // string → time.Time

	// revokedMu guards revokedEntries.
	revokedMu sync.Mutex
	// revokedEntries maps confirmed-revoked SIDs to the time the revocation was
	// first observed by this process. Used for fail-closed behaviour when the DB is
	// temporarily unreachable: if we previously confirmed a sid was revoked, we
	// continue to reject it during an outage. Entries are lazily evicted after
	// revokedCacheTTL — beyond that window the session would be rejected by the
	// max-age check anyway, so fail-closed protection is no longer needed.
	revokedEntries map[string]time.Time
}

// NewPolicyProvider wraps inner with the two session-policy enforcement layers.
// blocklist may be nil to disable revocation checks (tests, fresh deployments).
// sessionStarts may be nil to disable DB-backed session-start tracking; when nil,
// max-age falls back to IssuedAt for fva-absent flows (inaccurate, avoid in prod).
// Pass WithSessionStarter to enable accurate OAuth session-age enforcement.
func NewPolicyProvider(
	inner auth.IdentityProvider,
	blocklist IsRevoker,
	params GetInter,
	log *zap.Logger,
	opts ...PolicyOption,
) auth.IdentityProvider {
	p := &PolicyProvider{
		inner:          inner,
		blocklist:      blocklist,
		params:         params,
		log:            log,
		revokedEntries: make(map[string]time.Time),
	}
	for _, opt := range opts {
		opt(p)
	}
	// Emit a single startup warning rather than one per request. This surfaces
	// misconfigured deployments in the first log scan without flooding production
	// logs with per-request noise.
	if p.sessionStarts == nil {
		log.Warn("PolicyProvider: WithSessionStarter not configured; max-age enforcement falls back to IssuedAt for OAuth/fva-absent sessions — pass WithSessionStarter to fix")
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
	sessionStart := p.resolveSessionStart(ctx, claims, maxAgeSecs)
	if time.Since(sessionStart) > time.Duration(maxAgeSecs)*time.Second {
		return auth.Claims{}, fmt.Errorf("%w: session exceeded maximum age of %d seconds", auth.ErrInvalidToken, maxAgeSecs)
	}

	if p.params.GetInt(ctx, domain.ParamKeyAuthRequireMFA, domain.DefaultAuthRequireMFA) != 0 && claims.MFAVerifiedAt.IsZero() {
		return auth.Claims{}, fmt.Errorf("%w: MFA second factor not completed", auth.ErrInvalidToken)
	}

	if err := p.checkRevocation(ctx, claims.SessionID); err != nil {
		return auth.Claims{}, err
	}

	return claims, nil
}

// resolveSessionStart returns the session origin timestamp for max-age enforcement.
//
// Priority:
//  1. fva[0] claim (SessionStartedAt) — stable across JWT refreshes; present for
//     password / passkey / email-code login flows.
//  2. In-process cache (startCache) — avoids a DB round-trip for returning OAuth
//     requests that already had their session start resolved.
//  3. DB-backed session_starts table (via SessionStarter) — used when fva is absent
//     (OAuth / social / automatic-login). Result is cached in startCache for
//     subsequent requests from the same session.
//  4. IssuedAt — last-resort fallback when neither fva nor DB tracking is available.
//     This path should only occur for test tokens or misconfigured deployments.
func (p *PolicyProvider) resolveSessionStart(ctx context.Context, claims auth.Claims, maxAgeSecs int) time.Time {
	// Fast path: fva claim present — always accurate, no I/O needed.
	if !claims.SessionStartedAt.IsZero() {
		return claims.SessionStartedAt
	}

	// OAuth / fva-absent path.
	if claims.SessionID != "" && p.sessionStarts != nil {
		// Check the in-process cache before going to DB.
		if cached, ok := p.startCache.Load(claims.SessionID); ok {
			return cached.(time.Time)
		}
		// Cache miss: resolve from DB and populate the cache.
		t, err := p.sessionStarts.UpsertSessionStart(ctx, claims.SessionID, claims.Subject)
		if err != nil {
			p.log.Warn("session_starts upsert failed; falling back to IssuedAt for max-age (enforcement may be inaccurate)",
				zap.String("sid", claims.SessionID),
				zap.Error(err),
			)
			return claims.IssuedAt
		}
		p.startCache.Store(claims.SessionID, t)
		return t
	}

	// Fallback: no fva, no tracker, or no sid (test tokens).
	return claims.IssuedAt
}

// checkRevocation returns ErrInvalidToken if the session identified by sid has
// been locally revoked. Returns nil when sid is empty, the blocklist is not
// configured, or the session is not revoked.
//
// DB errors are handled fail-open for uncached sids (a DB outage should not
// lock out all authenticated users) and fail-closed for sids previously
// confirmed revoked (prevents a revoked session from regaining access during
// an outage).
func (p *PolicyProvider) checkRevocation(ctx context.Context, sid string) error {
	if sid == "" || p.blocklist == nil {
		return nil
	}

	revoked, checkErr := p.blocklist.IsRevoked(ctx, sid)
	if checkErr != nil {
		if p.isRevokedCached(sid) {
			return fmt.Errorf("%w: session has been revoked", auth.ErrInvalidToken)
		}
		p.log.Warn("session blocklist check failed; treating as not revoked (fail-open for uncached session)",
			zap.String("sid", sid),
			zap.Error(checkErr),
		)
		return nil
	}
	if revoked {
		p.cacheRevoked(sid)
		return fmt.Errorf("%w: session has been revoked", auth.ErrInvalidToken)
	}
	return nil
}

// isRevokedCached reports whether sid has a live entry in the in-process revocation
// cache. Entries older than revokedCacheTTL are lazily evicted and treated as absent.
func (p *PolicyProvider) isRevokedCached(sid string) bool {
	p.revokedMu.Lock()
	defer p.revokedMu.Unlock()
	t, ok := p.revokedEntries[sid]
	if !ok {
		return false
	}
	if time.Since(t) > revokedCacheTTL {
		delete(p.revokedEntries, sid)
		return false
	}
	return true
}

// cacheRevoked records sid as revoked in the in-process cache.
func (p *PolicyProvider) cacheRevoked(sid string) {
	p.revokedMu.Lock()
	p.revokedEntries[sid] = time.Now()
	p.revokedMu.Unlock()
}

var _ auth.IdentityProvider = (*PolicyProvider)(nil)
