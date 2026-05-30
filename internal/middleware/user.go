package middleware

import (
	"context"
	"net"
	"net/http"

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
			user, err := userRepo.GetByClerkSubject(r.Context(), subject)
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
// Outside Fly.io (local dev, CI) the header is absent, so we fall back to
// r.RemoteAddr, which is the raw TCP peer address and equally unforgeable.
//
// We deliberately never read X-Forwarded-For, X-Real-IP, or True-Client-IP:
// any client can set those to arbitrary values to cycle fake IPs past rate
// limiters.
func TrustedClientIP(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("Fly-Client-IP"); ip != "" {
			r.RemoteAddr = ip
		}
		// else: r.RemoteAddr is already the unforgeable TCP peer address.
		next.ServeHTTP(w, r)
	})
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
