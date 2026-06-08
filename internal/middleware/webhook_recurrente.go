package middleware

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	svixIDHeader        = "svix-id"
	svixTimestampHeader = "svix-timestamp"
	svixSignatureHeader = "svix-signature"
	whsecPrefix         = "whsec_"

	// svixToleranceSeconds is the maximum absolute age of a Svix webhook
	// timestamp we accept. Matches the Svix SDK default (5 minutes).
	svixToleranceSeconds int64 = 300

	// webhookBodyLimit caps body reads in webhook middleware (1 MiB).
	webhookBodyLimit = 1 << 20
)

// RecurrenteWebhookAuth returns middleware that verifies Svix webhook signatures
// delivered by Recurrente.
//
// Recurrente delivers webhooks via Svix (https://docs.svix.com/). The signing
// secret is obtained from the Recurrente dashboard (Configuración → Desarrolladores
// y API → Webhooks) and has the format "whsec_<base64>". Verification follows the
// Svix algorithm:
//
//  1. Build signed_content = svix-id + "." + svix-timestamp + "." + raw-body.
//  2. Decode the base64 portion of the whsec_ secret to obtain the HMAC key.
//  3. Compute HMAC-SHA256(key, signed_content) and base64-encode the result.
//  4. Compare against every v1,<base64> token in the svix-signature header (there
//     may be several during secret rotation; any match is sufficient).
//  5. Reject requests whose svix-timestamp is more than 5 minutes old or in the
//     future by more than 5 minutes (replay-attack protection).
//
// When secret is empty the middleware logs a warning and passes all requests
// through without verification — acceptable for local development only.
// pkg/config/validation.go rejects an empty secret before the server starts in any
// non-development environment.
//
// Legacy raw (non-whsec_) secrets are also accepted to ease migration: the raw
// bytes are used directly as the HMAC key with the same Svix signed-content formula.
func RecurrenteWebhookAuth(secret string, log *zap.Logger) func(http.Handler) http.Handler {
	if secret == "" {
		log.Warn("RecurrenteWebhookAuth: WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET is not set — signature verification DISABLED; do not use in production")
	}
	key, err := parseRecurrenteSigningSecret(secret)
	if secret != "" && err != nil {
		log.Error("RecurrenteWebhookAuth: invalid signing secret format — verification DISABLED; fix WCQ_PAYMENT_RECURRENTEWEBHOOKSECRET",
			zap.Error(err))
		// Treat as bypass so startup doesn't hard-fail; config validation catches it.
		key = nil
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			serveRecurrenteWebhook(w, r, next, key, log)
		})
	}
}

// serveRecurrenteWebhook is the inner handler extracted from RecurrenteWebhookAuth
// to keep the outer function's cognitive complexity within the project lint threshold.
func serveRecurrenteWebhook(w http.ResponseWriter, r *http.Request, next http.Handler, key []byte, log *zap.Logger) {
	body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
	if err != nil {
		WriteError(w, r, log, apperrors.Internal(err))
		return
	}
	// Restore body for downstream handler.
	r.Body = io.NopCloser(bytes.NewReader(body))

	if key == nil {
		// Bypass mode (dev only): stamp sentinel so handler contract is satisfied.
		r = r.WithContext(SetWebhookVerifiedBody(r.Context(), body))
		next.ServeHTTP(w, r)
		return
	}

	msgID := r.Header.Get(svixIDHeader)
	msgTimestamp := r.Header.Get(svixTimestampHeader)
	msgSignature := r.Header.Get(svixSignatureHeader)

	if msgID == "" || msgTimestamp == "" || msgSignature == "" {
		log.Warn("recurrente webhook: missing Svix headers",
			zap.String("request_id", GetRequestID(r.Context())),
		)
		WriteError(w, r, log, apperrors.Unauthorised("missing webhook signature headers"))
		return
	}

	ts, tsErr := strconv.ParseInt(msgTimestamp, 10, 64)
	if tsErr != nil {
		WriteError(w, r, log, apperrors.Unauthorised("invalid webhook timestamp"))
		return
	}
	age := time.Now().Unix() - ts
	if age < -svixToleranceSeconds || age > svixToleranceSeconds {
		log.Warn("recurrente webhook: timestamp outside tolerance window",
			zap.Int64("age_seconds", age),
			zap.String("request_id", GetRequestID(r.Context())),
		)
		WriteError(w, r, log, apperrors.Unauthorised("webhook timestamp outside tolerance window"))
		return
	}

	if !svixSignatureValid(key, msgID, msgTimestamp, body, msgSignature) {
		log.Warn("recurrente webhook: signature mismatch",
			zap.String("svix-id", msgID),
			zap.String("request_id", GetRequestID(r.Context())),
		)
		WriteError(w, r, log, apperrors.Unauthorised("invalid webhook signature"))
		return
	}

	r = r.WithContext(SetWebhookVerifiedBody(r.Context(), body))
	next.ServeHTTP(w, r)
}

// parseRecurrenteSigningSecret derives the HMAC key from a signing secret.
// Accepts "whsec_<base64>" (Svix format) or a raw secret (legacy; key is raw bytes).
// Returns nil, nil for an empty secret (bypass mode).
func parseRecurrenteSigningSecret(secret string) ([]byte, error) {
	if secret == "" {
		return nil, nil
	}
	if strings.HasPrefix(secret, whsecPrefix) {
		b64 := secret[len(whsecPrefix):]
		key, err := base64.StdEncoding.DecodeString(b64)
		if err != nil {
			return nil, fmt.Errorf("decode whsec_ base64: %w", err)
		}
		return key, nil
	}
	return []byte(secret), nil
}

// svixSignatureValid returns true if any v1,<base64> token in the svix-signature
// header matches the HMAC computed over the Svix signed-content string.
// Constant-time byte comparison prevents timing attacks.
func svixSignatureValid(key []byte, msgID, msgTimestamp string, body []byte, sigHeader string) bool {
	// signed_content = msgID + "." + msgTimestamp + "." + rawBody
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(msgID))
	mac.Write([]byte("."))
	mac.Write([]byte(msgTimestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	for _, token := range strings.Fields(sigHeader) {
		if !strings.HasPrefix(token, "v1,") {
			continue
		}
		if hmac.Equal([]byte(token[3:]), []byte(expected)) {
			return true
		}
	}
	return false
}
