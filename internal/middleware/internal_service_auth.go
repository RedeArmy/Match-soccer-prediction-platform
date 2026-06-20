package middleware

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// InternalServiceAuth returns a middleware that authenticates service-to-service
// requests (e.g. from n8n scheduled workflows) using a static shared secret
// passed in the "Authorization: Bearer <key>" header.
//
// Unlike RequireAuth (which validates Clerk JWTs), this middleware checks the
// token against a pre-shared key configured via WCQ_N8N_INTERNALKEY. Apply it
// to /api/internal/* routes that internal automation must reach without a user
// session.
//
// Security properties:
//   - Token comparison is constant-time (crypto/subtle) to prevent timing attacks.
//   - An empty key is treated as misconfiguration: the middleware returns 503 on
//     every request rather than silently admitting all callers, so the operator
//     learns of the problem at the first call rather than at a security review.
//   - The route is still covered by the L1 IP rate limiter registered on the root
//     router; no additional rate limiter is applied here because n8n's scheduler
//     fires at most once every 24 hours per workflow.
func InternalServiceAuth(key string, log *zap.Logger) func(http.Handler) http.Handler {
	if key == "" {
		log.Warn("internal service auth: WCQ_N8N_INTERNALKEY is not set — " +
			"/api/internal/* routes will return 503 until the key is configured")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if key == "" {
				WriteError(w, r, log, apperrors.Internal(
					fmt.Errorf("internal service authentication is not configured; set WCQ_N8N_INTERNALKEY"),
				))
				return
			}

			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				WriteError(w, r, log, apperrors.Unauthorised(apperrors.MsgUnauthorised))
				return
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")

			if subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
				WriteError(w, r, log, apperrors.Unauthorised("invalid service credential"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
