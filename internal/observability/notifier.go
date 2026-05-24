// Package observability provides the Notifier, which ships
// structured JSON events to n8n webhook endpoints for operational alerting.
//
// Every public method is non-blocking: the HTTP POST runs in a detached
// goroutine so that the caller's critical path (payment processing, DLQ
// polling, circuit-breaker record) is never delayed by n8n network latency
// or unavailability.
//
// When BaseURL is empty, every method is a silent no-op — wire the notifier
// unconditionally; no per-call guard is needed at the call site.
package observability

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	pathDLQOverflow      = "/webhook/dlq-overflow"
	pathCircuitBreaker   = "/webhook/circuit-breaker"
	pathPaymentError     = "/webhook/payment-error"
	pathOutboxLag        = "/webhook/outbox-lag"
	pathPayoutApproved   = "/webhook/payout-approved"
	pathTransferUploaded = "/webhook/transfer-uploaded"
	pathBalanceCredited  = "/webhook/balance-credited"

	defaultWebhookTimeout = 5 * time.Second
)

// NotifierConfig bundles the constructor parameters for Notifier.
type NotifierConfig struct {
	// BaseURL is the n8n base URL, e.g. "http://n8n:5678".
	// When empty, all methods are no-ops and no HTTP connections are made.
	// Set via WCQ_N8N_BASEURL.
	BaseURL string
	// Secret is the HMAC-SHA256 signing key stamped in X-Signature headers.
	// When empty, requests are sent unsigned; a warning is emitted at
	// construction so the misconfiguration is visible in startup logs.
	// Set via WCQ_N8N_WEBHOOKSECRET.
	Secret string
	Log    *zap.Logger
}

// Notifier ships structured JSON events to n8n webhooks.
// The zero value is not usable; construct with New.
type Notifier struct {
	baseURL    string
	secret     string
	httpClient *http.Client
	log        *zap.Logger
}

// New constructs a Notifier from cfg.
// When cfg.BaseURL is empty the returned notifier is disabled — all methods
// are no-ops and no HTTP client or connections are created.
func New(cfg NotifierConfig) *Notifier {
	if cfg.BaseURL != "" && cfg.Secret == "" {
		cfg.Log.Warn("observability notifier: WCQ_N8N_WEBHOOKSECRET not configured — webhook requests will be unsigned; set the secret to authenticate deliveries")
	}
	return &Notifier{
		baseURL: cfg.BaseURL,
		secret:  cfg.Secret,
		httpClient: &http.Client{
			Timeout:   defaultWebhookTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		log: cfg.Log,
	}
}

// Enabled reports whether this notifier will actually send HTTP requests.
// When false, all methods are no-ops.
func (n *Notifier) Enabled() bool { return n.baseURL != "" }

// ── Payload types ────────────────────────────────────────────────────────────

// DLQOverflowPayload is the body posted to /webhook/dlq-overflow.
type DLQOverflowPayload struct {
	DLQDepth  int64  `json:"dlq_depth"`
	Threshold int64  `json:"threshold"`
	Timestamp string `json:"timestamp"`
	TraceID   string `json:"trace_id,omitempty"`
}

// CircuitBreakerPayload is the body posted to /webhook/circuit-breaker.
type CircuitBreakerPayload struct {
	Backend    string `json:"backend"`
	State      string `json:"state"`
	LastTripAt string `json:"last_trip_at"`
	TraceID    string `json:"trace_id,omitempty"`
}

// PaymentErrorPayload is the body posted to /webhook/payment-error.
type PaymentErrorPayload struct {
	Provider  string `json:"provider"`
	ErrorCode string `json:"error_code"`
	UserID    string `json:"user_id,omitempty"`
	Amount    string `json:"amount,omitempty"`
	TraceID   string `json:"trace_id,omitempty"`
}

// OutboxLagPayload is the body posted to /webhook/outbox-lag.
type OutboxLagPayload struct {
	LagSeconds   float64 `json:"lag_seconds"`
	PendingCount int64   `json:"pending_count"`
	Timestamp    string  `json:"timestamp"`
}

// PayoutApprovedPayload is the body posted to /webhook/payout-approved.
type PayoutApprovedPayload struct {
	UserID      int    `json:"user_id"`
	AmountCents int    `json:"amount_cents"`
	Method      string `json:"method"`
	AdminID     string `json:"admin_id"`
	TraceID     string `json:"trace_id,omitempty"`
}

// TransferUploadedPayload is the body posted to /webhook/transfer-uploaded.
type TransferUploadedPayload struct {
	UserID             int    `json:"user_id"`
	FileURL            string `json:"file_url"`
	AmountClaimedCents int    `json:"amount_claimed_cents"`
	Timestamp          string `json:"timestamp"`
}

// BalanceCreditedPayload is the body posted to /webhook/balance-credited.
type BalanceCreditedPayload struct {
	UserID      int    `json:"user_id"`
	AmountCents int    `json:"amount_cents"`
	Source      string `json:"source"`
	TraceID     string `json:"trace_id,omitempty"`
}

// ── Notify methods ───────────────────────────────────────────────────────────

// NotifyDLQOverflow fires a non-blocking POST to /webhook/dlq-overflow.
// Call whenever the notification DLQ unresolved count exceeds the alert
// threshold so n8n can page the on-call team.
func (n *Notifier) NotifyDLQOverflow(ctx context.Context, depth, threshold int64) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathDLQOverflow, DLQOverflowPayload{
		DLQDepth:  depth,
		Threshold: threshold,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		TraceID:   extractTraceID(ctx),
	})
}

// NotifyCircuitBreakerOpen fires a non-blocking POST to /webhook/circuit-breaker.
// backend is the breaker name (e.g. "s3", "gdrive", "onedrive", "email").
// state is the new state string ("open", "half-open", "closed").
// lastTripAt is the time the circuit transitioned to open.
func (n *Notifier) NotifyCircuitBreakerOpen(ctx context.Context, backend, state string, lastTripAt time.Time) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathCircuitBreaker, CircuitBreakerPayload{
		Backend:    backend,
		State:      state,
		LastTripAt: lastTripAt.UTC().Format(time.RFC3339),
		TraceID:    extractTraceID(ctx),
	})
}

// NotifyPaymentError fires a non-blocking POST to /webhook/payment-error.
// provider is "recurrente" or "paypal". errorCode is a short error token.
// userID and amount are string-encoded to avoid implicit type assumptions
// (the payment webhook layer works with strconv-decoded values).
func (n *Notifier) NotifyPaymentError(ctx context.Context, provider, errorCode, userID, amount string) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathPaymentError, PaymentErrorPayload{
		Provider:  provider,
		ErrorCode: errorCode,
		UserID:    userID,
		Amount:    amount,
		TraceID:   extractTraceID(ctx),
	})
}

// NotifyOutboxLag fires a non-blocking POST to /webhook/outbox-lag when the
// outbox processing lag exceeds the monitoring threshold (typically 30 s).
func (n *Notifier) NotifyOutboxLag(ctx context.Context, lagSeconds float64, pendingCount int64) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathOutboxLag, OutboxLagPayload{
		LagSeconds:   lagSeconds,
		PendingCount: pendingCount,
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	})
}

// NotifyPayoutApproved fires a non-blocking POST to /webhook/payout-approved.
// Triggered when an admin approves a withdrawal request.
func (n *Notifier) NotifyPayoutApproved(ctx context.Context, userID, amountCents int, method, adminID string) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathPayoutApproved, PayoutApprovedPayload{
		UserID:      userID,
		AmountCents: amountCents,
		Method:      method,
		AdminID:     adminID,
		TraceID:     extractTraceID(ctx),
	})
}

// NotifyTransferUploaded fires a non-blocking POST to /webhook/transfer-uploaded.
// Triggered when a user uploads a bank transfer proof. fileURL is the storage
// key; amountClaimedCents is the amount declared in the form.
func (n *Notifier) NotifyTransferUploaded(ctx context.Context, userID, amountClaimedCents int, fileURL string) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathTransferUploaded, TransferUploadedPayload{
		UserID:             userID,
		FileURL:            fileURL,
		AmountClaimedCents: amountClaimedCents,
		Timestamp:          time.Now().UTC().Format(time.RFC3339),
	})
}

// NotifyBalanceCredited fires a non-blocking POST to /webhook/balance-credited.
// source describes the credit origin: "recurrente", "paypal", "manual", "win".
func (n *Notifier) NotifyBalanceCredited(ctx context.Context, userID, amountCents int, source string) {
	if !n.Enabled() {
		return
	}
	n.fire(ctx, pathBalanceCredited, BalanceCreditedPayload{
		UserID:      userID,
		AmountCents: amountCents,
		Source:      source,
		TraceID:     extractTraceID(ctx),
	})
}

// ── Internal helpers ─────────────────────────────────────────────────────────

// fire marshals payload and dispatches the webhook in a background goroutine.
// The goroutine uses a fresh context with a fixed timeout so it is not
// cancelled when the originating request context expires.
func (n *Notifier) fire(ctx context.Context, path string, payload any) {
	b, err := json.Marshal(payload)
	if err != nil {
		n.log.Warn("observability notifier: marshal failed",
			zap.String("path", path),
			zap.Error(err),
		)
		return
	}

	// Capture log fields from the live context before launching the goroutine.
	logFields := []zap.Field{zap.String("webhook_path", path)}

	go func() { //nolint:gosec // G118: intentional fire-and-forget; goroutine must outlive the request context
		// Background context: the HTTP call must survive the request context.
		postCtx, cancel := context.WithTimeout(context.Background(), defaultWebhookTimeout)
		defer cancel()

		url := n.baseURL + path
		req, err := http.NewRequestWithContext(postCtx, http.MethodPost, url, bytes.NewReader(b))
		if err != nil {
			n.log.Warn("observability notifier: failed to build request",
				append(logFields, zap.Error(err))...)
			return
		}
		req.Header.Set("Content-Type", "application/json")

		if n.secret != "" {
			mac := hmac.New(sha256.New, []byte(n.secret))
			mac.Write(b)
			req.Header.Set("X-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
		}

		resp, err := n.httpClient.Do(req)
		if err != nil {
			n.log.Warn("observability notifier: request failed",
				append(logFields, zap.Error(err))...)
			return
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode >= 300 {
			n.log.Warn("observability notifier: unexpected response status",
				append(logFields, zap.Int("status_code", resp.StatusCode))...)
		}
	}()

	_ = ctx // ctx trace_id already captured in payload; parameter kept for future use
}

// extractTraceID returns the W3C trace ID from ctx, or empty string when
// tracing is disabled or no active span is present.
func extractTraceID(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}
