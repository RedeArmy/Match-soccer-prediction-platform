// Package middleware provides HTTP middleware for the chi router.
//
// Each file in this package implements a single, self-contained concern.
// Middleware is applied in the Routes() method of internal/api/Server and
// must be stateless and safe for concurrent use by multiple goroutines.
//
// Custom middleware in this package supplements - rather than replaces - the
// middleware provided by go-chi/chi/v5/middleware. The RequestID concern is
// delegated to chi; application-specific concerns (JWT validation, structured
// zap logging, trusted IP extraction, Clerk authentication) are implemented
// here to keep business context out of the chi package.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/auth"
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

// RequireAuth returns a middleware that validates the Bearer JWT in the
// Authorization header using the given IdentityProvider.
//
// On success, the provider's subject (e.g. Clerk user_id) is stored in the
// request context and is retrievable via UserIDFromContext.
//
// Error mapping:
//   - auth.ErrProviderUnavailable → 503 Internal (the provider is down, not the
//     caller's fault; transient outage should not expose credentials issues)
//   - auth.ErrInvalidToken or any other error → 401 Unauthorised
//
// Swapping identity providers (Clerk → Auth0 → custom) requires only
// constructing a different IdentityProvider implementation at the wiring
// layer; this middleware is provider-agnostic.
func RequireAuth(provider auth.IdentityProvider, log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				WriteError(w, r, log, apperrors.Unauthorised(apperrors.MsgUnauthorised))
				return
			}
			rawToken := strings.TrimPrefix(authHeader, "Bearer ")

			subject, err := provider.ValidateToken(r.Context(), rawToken)
			if err != nil {
				if errors.Is(err, auth.ErrProviderUnavailable) {
					log.Error("RequireAuth: identity provider unavailable",
						zap.String("request_id", GetRequestID(r.Context())),
						zap.Error(err),
					)
					WriteError(w, r, log, apperrors.Internal(err))
				} else {
					WriteError(w, r, log, apperrors.Unauthorised("invalid or expired token"))
				}
				return
			}

			ctx := context.WithValue(r.Context(), contextKeyUserID, subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
