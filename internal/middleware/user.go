package middleware

import (
	"context"
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// msgUserNotSynced is returned when the Clerk JWT is valid but no matching
// internal user row exists yet. This happens in the brief window between a
// user signing up on Clerk and the user.created webhook being delivered and
// processed. The client should retry after a short delay.
const msgUserNotSynced = "user account not found; please try again shortly"

// ResolveUser is middleware that resolves the Clerk subject stored in the
// request context (by RequireAuth) to a full domain.User and stores it under
// contextKeyUser. Handlers that need the caller's identity can then call
// UserFromContext instead of querying the database themselves.
//
// Must be placed after RequireAuth in the middleware chain. Returns 401 when
// the Clerk subject has no matching internal user row - this is transient and
// the client should retry.
func ResolveUser(userRepo repository.UserRepository, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject, ok := UserIDFromContext(r.Context())
			if !ok {
				WriteError(w, r, log, apperrors.Unauthorised(apperrors.MsgUnauthorised))
				return
			}
			user, err := userRepo.GetByExternalSubject(r.Context(), subject)
			if err != nil {
				WriteError(w, r, log, apperrors.Internal(err))
				return
			}
			if user == nil {
				WriteError(w, r, log, apperrors.Unauthorised(msgUserNotSynced))
				return
			}
			if user.BannedAt != nil {
				WriteError(w, r, log, apperrors.Forbidden("your account has been suspended"))
				return
			}
			ctx := context.WithValue(r.Context(), contextKeyUser, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// UserFromContext returns the domain.User stored by ResolveUser. The second
// return value is false when the middleware was not applied to the route.
func UserFromContext(ctx context.Context) (*domain.User, bool) {
	u, ok := ctx.Value(contextKeyUser).(*domain.User)
	return u, ok
}

// ContextWithUser returns a new context with user stored under the same key
// as ResolveUser. Use this in tests to simulate a resolved user without running
// real JWT validation or database lookups.
func ContextWithUser(ctx context.Context, user *domain.User) context.Context {
	return context.WithValue(ctx, contextKeyUser, user)
}

// TrustedClientIP is the authoritative IP-extraction middleware for this
// application. It replaces chi's RealIP and is safe against header-injection
// attacks that would let clients bypass per-IP rate limits.
//
// On Fly.io the edge sets Fly-Client-IP from the real TCP connection address
// before the request reaches the app; clients cannot forge or override it.
// This app's actual production deployment is Hetzner + Caddy (see
// Caddyfile), not Fly, so Fly-Client-IP is never present there — it is kept
// as the first, highest-priority check only for forward-compatibility with a
// possible future Fly deployment.
//
// For the Caddy deployment, the Go backend's HTTP port is never reachable
// directly from the public internet: docker-compose.prod.yml binds it to
// 127.0.0.1 only (reachable exclusively by Caddy on the same host) and, for
// requests proxied through the Next.js BFF, to the docker-compose bridge
// network (reachable exclusively by the frontend container — see
// BACKEND_INTERNAL_URL). Both of those intermediaries are trusted
// infrastructure we control: Caddy's reverse_proxy always appends the true
// connecting IP as the LAST entry of X-Forwarded-For regardless of what a
// client sent (this cannot be influenced by the client), and the BFF proxy
// (frontend/src/app/api/[...path]/route.ts) faithfully relays that same
// header unmodified on its internal call to the backend. So the last entry
// of X-Forwarded-For is safe to trust here specifically because nothing
// other than those two trusted hops can ever reach this port — it is NOT
// safe to trust in general, and this reasoning must be revisited if the
// deployment topology changes (e.g. exposing the API port directly, or
// adding a proxy hop that does not control this header).
//
// Without Fly-Client-IP or X-Forwarded-For (local dev, CI, or any direct
// caller not behind Caddy) we fall back to r.RemoteAddr, the raw TCP peer
// address, which is unforgeable but collapses every BFF-proxied user onto
// the frontend container's address in that scenario — acceptable outside
// production. We deliberately never read X-Real-IP or True-Client-IP: Caddy
// does not manage those, so trusting them would let a client set arbitrary
// values to cycle fake IPs past rate limiters.
func TrustedClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Header.Get("Fly-Client-IP") != "":
			r.RemoteAddr = r.Header.Get("Fly-Client-IP")
		case r.Header.Get("X-Forwarded-For") != "":
			r.RemoteAddr = lastForwardedFor(r.Header.Get("X-Forwarded-For"))
		}
		// else: r.RemoteAddr is already the unforgeable TCP peer address.
		next.ServeHTTP(w, r)
	})
}

// lastForwardedFor returns the right-most (most-recently-appended) entry of
// a comma-separated X-Forwarded-For chain, trimmed of whitespace. The last
// entry is the one our own trusted reverse proxy (Caddy) appended; any
// earlier entries may have been supplied by the client and must never be
// trusted for rate-limiting or audit purposes.
func lastForwardedFor(xff string) string {
	parts := strings.Split(xff, ",")
	return strings.TrimSpace(parts[len(parts)-1])
}

// StoreClientIP extracts the host portion of r.RemoteAddr (already normalised
// by TrustedClientIP) and stores it via repository.ContextWithClientIP.
// Must be placed after TrustedClientIP in the middleware chain.
func StoreClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			host = r.RemoteAddr // already host-only (no port)
		}
		ctx := repository.ContextWithClientIP(r.Context(), host)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// maxUserAgentLen caps the stored User-Agent to prevent oversized writes.
const maxUserAgentLen = 512

// StoreUserAgent reads the User-Agent request header, truncates it to
// maxUserAgentLen bytes, and stores it via repository.ContextWithUserAgent.
// Must be placed in the root middleware chain so session-start writes have
// access to the value for all authenticated endpoints.
func StoreUserAgent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if len(ua) > maxUserAgentLen {
			ua = ua[:maxUserAgentLen]
		}
		next.ServeHTTP(w, r.WithContext(repository.ContextWithUserAgent(r.Context(), ua)))
	})
}
