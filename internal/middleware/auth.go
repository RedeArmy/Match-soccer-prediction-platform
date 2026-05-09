// Package middleware provides HTTP middleware for the chi router.
//
// Each file in this package implements a single, self-contained concern.
// Middleware is applied in the Routes() method of internal/api/Server and
// must be stateless and safe for concurrent use by multiple goroutines.
//
// Custom middleware in this package supplements - rather than replaces - the
// middleware provided by go-chi/chi/v5/middleware. Generic HTTP concerns
// (RequestID, RealIP) are delegated to chi; application-specific concerns
// (JWT validation, structured zap logging, Clerk authentication) are
// implemented here to keep business context out of the chi package.
package middleware

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v2/jwk"
	"github.com/lestrrat-go/jwx/v2/jwt"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// contextKey is an unexported type for context keys defined in this package.
// Using a named type prevents collisions with keys defined in other packages
// that also use context.WithValue with a plain string or integer key.
type contextKey int

const (
	contextKeyUserID contextKey = iota
	contextKeyUser              // resolved *domain.User, set by ResolveUser middleware
)

// ContextWithUserID returns a new context with the given Clerk user ID stored
// under the same key as RequireAuth. Use this in tests to simulate an
// authenticated request without real JWT validation.
func ContextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, contextKeyUserID, userID)
}

// UserIDFromContext returns the Clerk user ID stored in ctx by RequireAuth.
// The second return value is false when the request did not pass through
// RequireAuth (e.g. public endpoints).
func UserIDFromContext(ctx context.Context) (string, bool) {
	id, ok := ctx.Value(contextKeyUserID).(string)
	return id, ok
}

// RequireRole returns a middleware that verifies the authenticated user holds
// at least one of the specified roles. It must be placed after RequireAuth in
// the middleware chain so that a valid Clerk subject is already in the context.
//
// The subject is resolved to an internal User row via GetByClerkSubject. If the
// subject has no matching row - i.e. the user-sync webhook has not yet fired -
// the request is rejected with 401. If the user exists but lacks the required
// role, the request is rejected with 403.
func RequireRole(userRepo repository.UserRepository, log *zap.Logger, roles ...domain.UserRole) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return requireRoleHandler(next, userRepo, log, roles)
	}
}

func requireRoleHandler(next http.Handler, userRepo repository.UserRepository, log *zap.Logger, roles []domain.UserRole) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, r, ok := resolveRequestUser(w, r, userRepo, log)
		if !ok {
			return
		}
		if user.BannedAt != nil {
			WriteError(w, r, log, apperrors.Forbidden("your account has been suspended"))
			return
		}
		for _, role := range roles {
			if user.Role == role {
				next.ServeHTTP(w, r)
				return
			}
		}
		WriteError(w, r, log, apperrors.Forbidden(apperrors.MsgForbidden))
	}
}

// resolveRequestUser returns the domain.User for the current request.
// It first checks the context (set by ResolveUser or a prior RequireRole call)
// and falls back to a database lookup via GetByClerkSubject. On any failure it
// writes the appropriate error response and returns (nil, r, false). On success
// it returns (user, r, true); when the user was fetched from the database r
// carries an updated context with the user stored under contextKeyUser so that
// any downstream middleware or handler can call UserFromContext without a
// second round-trip.
func resolveRequestUser(w http.ResponseWriter, r *http.Request, userRepo repository.UserRepository, log *zap.Logger) (*domain.User, *http.Request, bool) {
	if user, ok := UserFromContext(r.Context()); ok {
		return user, r, true
	}
	subject, ok := UserIDFromContext(r.Context())
	if !ok {
		WriteError(w, r, log, apperrors.Unauthorised(apperrors.MsgUnauthorised))
		return nil, r, false
	}
	user, err := userRepo.GetByClerkSubject(r.Context(), subject)
	if err != nil {
		WriteError(w, r, log, apperrors.Internal(err))
		return nil, r, false
	}
	if user == nil {
		WriteError(w, r, log, apperrors.Unauthorised(msgUserNotSynced))
		return nil, r, false
	}
	return user, r.WithContext(context.WithValue(r.Context(), contextKeyUser, user)), true
}

// RequireAuth returns a middleware that validates the Clerk JWT present in
// the Authorization: Bearer header.
//
// Clerk signs tokens with RS256 using a rotating key pair. Public keys are
// fetched from the JWKS endpoint on the first request and cached in memory.
// The cache refreshes automatically in the background every 15 minutes so
// that key rotations are picked up without restarting the server.
//
// On success the Clerk user ID (the "sub" claim) is stored in the request
// context and is retrievable via UserIDFromContext. On failure a 401
// response is written and the next handler is not called.
//
// jwksURL is the Clerk JWKS endpoint (WCQ_CLERK_JWKSURL in config). If it
// is empty the middleware is bypassed and a warning is logged. Startup
// validation must ensure this only happens in development environments.
// DefaultJWKSWarmupTimeout is the fallback JWKS warm-up timeout when
// auth.validation_timeout_seconds is absent from system_params or the DB
// is not yet available (e.g. the db-unavailable route fallback path).
const DefaultJWKSWarmupTimeout = 5 * time.Second

// RequireAuth builds a Clerk JWT authentication middleware.
// warmupTimeout caps the JWKS prefetch at startup; pass defaultJWKSWarmupTimeout
// (5s) when no system_param override is available.
func RequireAuth(jwksURL string, warmupTimeout time.Duration, log *zap.Logger) func(http.Handler) http.Handler {
	if jwksURL == "" {
		// Fail-closed: an unconfigured JWKS endpoint means we cannot verify any
		// token. Returning a pass-through handler here would open the entire API
		// to unauthenticated callers, which is never the correct production
		// behaviour and can mask misconfiguration in staging environments.
		log.Error("RequireAuth: WCQ_CLERK_JWKSURL is not set - all requests will be rejected; set WCQ_CLERK_JWKSURL")
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				WriteError(w, r, log, apperrors.Unauthorised("authentication is not configured"))
			})
		}
	}

	jwkCache := jwk.NewCache(context.Background())
	if err := jwkCache.Register(jwksURL); err != nil {
		log.Error("RequireAuth: failed to register JWKS URL", zap.String("url", jwksURL), zap.Error(err))
	}

	// Eagerly warm the JWKS cache at startup so the first request is never
	// delayed by a cold fetch. warmupTimeout avoids blocking startup indefinitely
	// if Clerk is temporarily unreachable; the cache retries on the first request.
	warmCtx, cancel := context.WithTimeout(context.Background(), warmupTimeout)
	defer cancel()
	if _, err := jwkCache.Refresh(warmCtx, jwksURL); err != nil {
		log.Warn("RequireAuth: JWKS prefetch failed; will retry on first request",
			zap.String("url", jwksURL), zap.Error(err))
	}

	// fallbackMu guards fallbackKeySet, which holds the last successfully
	// fetched JWKS keyset. It is used as a fallback when jwkCache.Get fails so
	// that a transient Clerk outage does not reject every in-flight request.
	var fallbackMu sync.RWMutex
	var fallbackKeySet jwk.Set

	// Populate the fallback from the warm-up fetch if it succeeded.
	if ks, err := jwkCache.Get(context.Background(), jwksURL); err == nil {
		fallbackKeySet = ks
	}

	return func(next http.Handler) http.Handler {
		return requireAuthHandler(next, jwkCache, jwksURL, log, &fallbackMu, &fallbackKeySet)
	}
}

func requireAuthHandler(
	next http.Handler,
	jwkCache *jwk.Cache,
	jwksURL string,
	log *zap.Logger,
	fallbackMu *sync.RWMutex,
	fallbackKeySet *jwk.Set,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if !strings.HasPrefix(authHeader, "Bearer ") {
			WriteError(w, r, log, apperrors.Unauthorised(apperrors.MsgUnauthorised))
			return
		}
		tokenBytes := []byte(strings.TrimPrefix(authHeader, "Bearer "))

		keySet, err := jwkCache.Get(r.Context(), jwksURL)
		if err != nil {
			// Clerk is unreachable. Attempt to use the last known-good keyset
			// so that requests with recently-issued, unexpired tokens continue
			// to work during a transient outage. A stale keyset is better than
			// a hard failure; key rotations only happen every ~24 hours.
			fallbackMu.RLock()
			fk := *fallbackKeySet
			fallbackMu.RUnlock()
			if fk != nil {
				log.Warn("RequireAuth: JWKS fetch failed; using cached keyset fallback",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.Error(err),
				)
				keySet = fk
			} else {
				log.Error("RequireAuth: JWKS fetch failed and no fallback available",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.Error(err),
				)
				WriteError(w, r, log, apperrors.Internal(err))
				return
			}
		} else {
			// Successful fetch: update the fallback so it stays fresh.
			fallbackMu.Lock()
			*fallbackKeySet = keySet
			fallbackMu.Unlock()
		}

		token, err := jwt.Parse(tokenBytes, jwt.WithKeySet(keySet), jwt.WithValidate(true))
		if err != nil {
			WriteError(w, r, log, apperrors.Unauthorised("invalid or expired token"))
			return
		}

		ctx := context.WithValue(r.Context(), contextKeyUserID, token.Subject())
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
