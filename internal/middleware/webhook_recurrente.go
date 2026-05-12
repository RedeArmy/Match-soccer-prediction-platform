package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	recurrenteSigHeader = "X-Recurrente-Hmac-Sha256"
	// webhookBodyLimit caps body reads in webhook middleware.
	// 1 MB mirrors the io.LimitReader in the downstream handlers.
	webhookBodyLimit = 1 << 20
)

// RecurrenteWebhookAuth returns middleware that verifies the HMAC-SHA256
// signature on Recurrente webhook requests.
//
// Recurrente signs the raw request body with a shared secret using HMAC-SHA256
// and encodes the result as a lowercase hex string in the
// X-Recurrente-Hmac-Sha256 header. Requests missing the header or whose
// computed MAC does not match are rejected with 401.
//
// When secret is empty the middleware logs a warning and passes all requests
// through without verification. This is acceptable for local development only;
// pkg/config/validation.go rejects an empty secret before the server starts
// in any non-development environment.
func RecurrenteWebhookAuth(secret string, log *zap.Logger) func(http.Handler) http.Handler {
	if secret == "" {
		log.Warn("RecurrenteWebhookAuth: WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET is not set — signature verification DISABLED; do not use in production")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if secret == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
			if err != nil {
				WriteError(w, r, log, apperrors.Internal(err))
				return
			}
			// Restore the body so the downstream handler can re-read it.
			r.Body = io.NopCloser(bytes.NewReader(body))

			sig := r.Header.Get(recurrenteSigHeader)
			if sig == "" {
				log.Warn("recurrente webhook: missing signature header",
					zap.String("request_id", GetRequestID(r.Context())),
				)
				WriteError(w, r, log, apperrors.Unauthorised("missing webhook signature"))
				return
			}

			mac := hmac.New(sha256.New, []byte(secret))
			_, _ = mac.Write(body)
			expected := hex.EncodeToString(mac.Sum(nil))

			// hmac.Equal does a constant-time comparison to prevent timing attacks.
			if !hmac.Equal([]byte(sig), []byte(expected)) {
				log.Warn("recurrente webhook: signature mismatch",
					zap.String("request_id", GetRequestID(r.Context())),
				)
				WriteError(w, r, log, apperrors.Unauthorised("invalid webhook signature"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
