package middleware

import "context"

type webhookVerifiedBodyKey struct{}

// SetWebhookVerifiedBody stamps ctx with the signature-verified request body.
// Called by RecurrenteWebhookAuth and PayPalWebhookAuth after successful
// verification (or, in bypass mode, after the body is buffered). Handlers call
// WebhookVerifiedBodyFromContext to assert that the middleware ran.
func SetWebhookVerifiedBody(ctx context.Context, body []byte) context.Context {
	return context.WithValue(ctx, webhookVerifiedBodyKey{}, body)
}

// WebhookVerifiedBodyFromContext returns the verified body and true when the
// webhook middleware stamped the context. Returns nil, false otherwise — which
// signals that the middleware was bypassed and the handler should reject the
// request.
func WebhookVerifiedBodyFromContext(ctx context.Context) ([]byte, bool) {
	v, ok := ctx.Value(webhookVerifiedBodyKey{}).([]byte)
	return v, ok
}
