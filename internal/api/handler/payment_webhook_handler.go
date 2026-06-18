package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// paymentObservabilityNotifier is the narrow slice of ObservabilityNotifier
// consumed by PaymentWebhookHandler. The interface isolates the handler from
// the concrete observability package, keeping the import graph acyclic.
type paymentObservabilityNotifier interface {
	NotifyPaymentError(ctx context.Context, provider, errorCode, userID, amount string)
	NotifyBalanceCredited(ctx context.Context, userID, amountCents int, source string)
}

// PaymentWebhookHandler handles incoming payment confirmation webhooks from
// Recurrente and PayPal. Signature verification is performed by upstream
// middleware (middleware.RecurrenteWebhookAuth, middleware.PayPalWebhookAuth),
// which stamps the verified body into the request context. Handlers assert
// the presence of that sentinel before processing; a missing sentinel means
// the request reached this handler without passing through the auth middleware,
// which is treated as unauthorised.
type PaymentWebhookHandler struct {
	svc             service.WebhookPaymentService
	log             *zap.Logger
	notifier        paymentObservabilityNotifier // nil = disabled
	paymentDuration metric.Float64Histogram      // nil when not registered
}

// NewPaymentWebhookHandler constructs a PaymentWebhookHandler.
func NewPaymentWebhookHandler(svc service.WebhookPaymentService, log *zap.Logger) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{svc: svc, log: log}
}

// SetNotifier wires an ObservabilityNotifier for payment error and credit
// events. Call this before the handler serves any requests (i.e. at
// composition time in buildHandlers). Passing nil disables notifications.
func (h *PaymentWebhookHandler) SetNotifier(n paymentObservabilityNotifier) {
	h.notifier = n
}

// RegisterMetrics wires a payment processing duration histogram.  Call once
// after construction; safe to skip when metrics are disabled.
func (h *PaymentWebhookHandler) RegisterMetrics(meter metric.Meter) error {
	var err error
	h.paymentDuration, err = meter.Float64Histogram(
		"payment.processing.duration",
		metric.WithDescription("Duration of payment webhook processing by provider and status."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5,
		),
	)
	if err != nil {
		return fmt.Errorf("register payment histogram: %w", err)
	}
	return nil
}

// recurrentePaymentData holds the payment object nested inside a legacy
// payment.confirmed event (custom format, not part of the official Recurrente API).
type recurrentePaymentData struct {
	Reference   string `json:"reference"`
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	UserID      int    `json:"user_id"`
}

// recurrenteWebhookPayload is the minimal set of fields we extract from a
// legacy payment.confirmed event.
type recurrenteWebhookPayload struct {
	EventType string                `json:"event_type"`
	Data      recurrentePaymentData `json:"data"`
}

// recurrenteCheckoutMeta holds application-specific fields embedded in the
// Recurrente checkout metadata at checkout-creation time.
// Set "metadata": {"wcq_user_id": <int>, "wcq_reference": "<string>"} when
// calling the Recurrente checkout creation API.
type recurrenteCheckoutMeta struct {
	WCQUserID    int    `json:"wcq_user_id"`
	WCQReference string `json:"wcq_reference"`
}

// recurrenteCheckout is the checkout sub-object present in payment_intent.*
// and intent.* events delivered by Recurrente via Svix.
type recurrenteCheckout struct {
	ID       string                 `json:"id"`
	Metadata recurrenteCheckoutMeta `json:"metadata"`
}

// recurrenteIntentPayload covers both payment_intent.succeeded (legacy Recurrente
// API format) and intent.succeeded (unified Recurrente API format).
type recurrenteIntentPayload struct {
	ID          string             `json:"id"`
	EventType   string             `json:"event_type"`
	Type        string             `json:"type"`   // "payment" for intent.succeeded
	Status      string             `json:"status"` // "succeeded" etc.
	AmountCents int                `json:"amount_in_cents"`
	Currency    string             `json:"currency"`
	Checkout    recurrenteCheckout `json:"checkout"`
}

// HandleRecurrente handles POST /webhooks/recurrente.
//
// @Summary      Recurrente payment webhook
// @Description  Receives payment confirmation events from Recurrente via Svix. Credits the user's balance on confirmed events.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Success      204  "Event processed"
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /webhooks/recurrente [post]
func (h *PaymentWebhookHandler) HandleRecurrente(w http.ResponseWriter, r *http.Request) {
	body, ok := middleware.WebhookVerifiedBodyFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised("webhook not verified"))
		return
	}

	userID, amountCents, currency, reference, skip, err := extractRecurrenteCreditParams(body)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if skip {
		h.log.Debug("recurrente webhook: ignoring unrecognised event")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	start := time.Now()
	if err := h.svc.CreditFromRecurrente(r.Context(), userID, amountCents, currency, reference); err != nil {
		h.log.Error("recurrente webhook: failed to credit balance",
			zap.String("reference", reference),
			zap.Int("user_id", userID),
			zap.Error(err),
		)
		if h.notifier != nil {
			h.notifier.NotifyPaymentError(r.Context(), "recurrente", err.Error(),
				strconv.Itoa(userID), strconv.Itoa(amountCents))
		}
		h.recordPayment(r.Context(), "recurrente", "failed", time.Since(start))
		writeError(w, r, h.log, err)
		return
	}

	h.recordPayment(r.Context(), "recurrente", "success", time.Since(start))
	if h.notifier != nil {
		h.notifier.NotifyBalanceCredited(r.Context(), userID, amountCents, "recurrente")
	}
	w.WriteHeader(http.StatusNoContent)
}

const errMsgParseRecurrente = "could not parse Recurrente webhook payload"

// extractRecurrenteCreditParams parses a Recurrente webhook body and extracts the
// parameters needed to credit a user's balance. Supports three event formats:
//
//   - "payment.confirmed"       – legacy custom format; uses data.user_id + data.amount_cents.
//   - "payment_intent.succeeded" – official Recurrente API; uses checkout.metadata.wcq_user_id
//     and amount_in_cents. Requires the checkout to have been created with
//     metadata: {"wcq_user_id": <int>, "wcq_reference": "<string>"}.
//   - "intent.succeeded"        – unified Recurrente API (type="payment", status="succeeded");
//     same metadata convention as payment_intent.succeeded.
//
// Returns skip=true for unknown event types (caller should respond 204 and return).
func extractRecurrenteCreditParams(body []byte) (userID, amountCents int, currency, reference string, skip bool, err error) {
	// Peek at event_type to route the format.
	var probe struct {
		EventType string `json:"event_type"`
		Type      string `json:"type"`
		Status    string `json:"status"`
	}
	if json.Unmarshal(body, &probe) != nil {
		return 0, 0, "", "", false, apperrors.Validation(errMsgParseRecurrente)
	}

	switch probe.EventType {
	case "payment.confirmed":
		var p recurrenteWebhookPayload
		if json.Unmarshal(body, &p) != nil {
			return 0, 0, "", "", false, apperrors.Validation(errMsgParseRecurrente)
		}
		d := p.Data
		if d.UserID <= 0 || d.AmountCents <= 0 || d.Reference == "" {
			return 0, 0, "", "", false, apperrors.Validation("missing required fields in Recurrente webhook data")
		}
		return d.UserID, d.AmountCents, d.Currency, d.Reference, false, nil

	case "payment_intent.succeeded":
		var p recurrenteIntentPayload
		if json.Unmarshal(body, &p) != nil {
			return 0, 0, "", "", false, apperrors.Validation(errMsgParseRecurrente)
		}
		return extractFromIntent(p)

	case "intent.succeeded":
		// Only handle payment intents that reached succeeded status.
		if probe.Type != "payment" || probe.Status != "succeeded" {
			return 0, 0, "", "", true, nil
		}
		var p recurrenteIntentPayload
		if json.Unmarshal(body, &p) != nil {
			return 0, 0, "", "", false, apperrors.Validation(errMsgParseRecurrente)
		}
		return extractFromIntent(p)

	default:
		return 0, 0, "", "", true, nil
	}
}

// extractFromIntent extracts credit parameters from a payment_intent.succeeded or
// intent.succeeded payload. User identity is resolved via checkout metadata fields
// (wcq_user_id / wcq_reference) that must be embedded at checkout-creation time.
func extractFromIntent(p recurrenteIntentPayload) (userID, amountCents int, currency, reference string, skip bool, err error) {
	if p.AmountCents <= 0 {
		return 0, 0, "", "", false, apperrors.Validation("missing amount_in_cents in Recurrente intent event")
	}
	if p.Checkout.Metadata.WCQUserID <= 0 {
		return 0, 0, "", "", false, apperrors.Validation("missing wcq_user_id in Recurrente checkout metadata")
	}
	ref := p.Checkout.Metadata.WCQReference
	if ref == "" {
		if p.Checkout.ID != "" {
			ref = "checkout:" + p.Checkout.ID
		} else {
			ref = "intent:" + p.ID
		}
	}
	return p.Checkout.Metadata.WCQUserID, p.AmountCents, p.Currency, ref, false, nil
}

// paypalCaptureAmount holds the monetary amount declared on a PayPal capture event.
// Used for defence-in-depth cross-checking against the server-authoritative intent amount.
type paypalCaptureAmount struct {
	Value        string `json:"value"`
	CurrencyCode string `json:"currency_code"`
}

// paypalCaptureResource is the resource object embedded in a PayPal
// PAYMENT.CAPTURE.COMPLETED event.
type paypalCaptureResource struct {
	// ID is the PayPal capture ID — used as the idempotency key.
	ID string `json:"id"`
	// CustomID is the server-generated payment intent token. It is set by
	// the frontend when creating the PayPal order using the token returned
	// by POST /api/v1/payment-intents. The token is unguessable and binds
	// the order to a specific user — it cannot be substituted.
	CustomID string              `json:"custom_id"`
	Amount   paypalCaptureAmount `json:"amount"`
}

// paypalWebhookPayload is the minimal set of fields we extract from a PayPal
// PAYMENT.CAPTURE.COMPLETED event.
type paypalWebhookPayload struct {
	EventType string                `json:"event_type"`
	Resource  paypalCaptureResource `json:"resource"`
}

// HandlePayPal handles POST /webhooks/paypal.
//
// @Summary      PayPal payment webhook
// @Description  Receives PAYMENT.CAPTURE.COMPLETED events from PayPal. Credits the user's balance.
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Success      204  "Event processed"
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /webhooks/paypal [post]
func (h *PaymentWebhookHandler) HandlePayPal(w http.ResponseWriter, r *http.Request) {
	body, ok := middleware.WebhookVerifiedBodyFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised("webhook not verified"))
		return
	}

	var payload paypalWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, r, h.log, apperrors.Validation("could not parse PayPal webhook payload"))
		return
	}

	if payload.EventType != "PAYMENT.CAPTURE.COMPLETED" {
		h.log.Debug("paypal webhook: ignoring event", zap.String("event_type", payload.EventType))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	intentToken := payload.Resource.CustomID
	captureID := payload.Resource.ID
	if intentToken == "" {
		writeError(w, r, h.log, apperrors.Validation("missing intent token in PayPal custom_id"))
		return
	}
	if captureID == "" {
		writeError(w, r, h.log, apperrors.Validation("missing capture ID in PayPal resource"))
		return
	}

	// Parse the webhook-declared amount for defence-in-depth cross-checking in
	// the service layer. A parse failure is non-fatal: the server-authoritative
	// intent amount is used for the actual credit regardless; we pass 0 to
	// signal "amount unavailable" and skip the mismatch warning.
	webhookAmountCents, err := paypalAmountToCents(payload.Resource.Amount.Value)
	if err != nil {
		h.log.Warn("paypal webhook: cannot parse amount field, skipping amount cross-check",
			zap.String("amount_value", payload.Resource.Amount.Value),
			zap.Error(err),
		)
		webhookAmountCents = 0
	}

	start := time.Now()
	if err := h.svc.ResolveAndCreditPayPalIntent(r.Context(), intentToken, captureID, webhookAmountCents); err != nil {
		// A NotFound error means the payment intent has expired or was never
		// created for this token. Returning a non-2xx here causes PayPal to
		// retry indefinitely, which is useless — the intent cannot be recovered
		// automatically. Acknowledge with 204 to stop the retry loop and emit a
		// warning so the payment can be credited manually if required.
		if errors.Is(err, apperrors.ErrNotFound) {
			h.log.Warn("paypal webhook: intent expired or not found — acknowledging to stop retries; manual credit may be required",
				zap.String("capture_id", captureID),
				zap.String("intent_token", intentToken),
				zap.Error(err),
			)
			if h.notifier != nil {
				h.notifier.NotifyPaymentError(r.Context(), "paypal", "intent_expired",
					"", strconv.Itoa(webhookAmountCents))
			}
			h.recordPayment(r.Context(), "paypal", "expired", time.Since(start))
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.log.Error("paypal webhook: failed to resolve intent and credit balance",
			zap.String("capture_id", captureID),
			zap.String("intent_token", intentToken),
			zap.Error(err),
		)
		if h.notifier != nil {
			h.notifier.NotifyPaymentError(r.Context(), "paypal", err.Error(),
				"", strconv.Itoa(webhookAmountCents))
		}
		h.recordPayment(r.Context(), "paypal", "failed", time.Since(start))
		writeError(w, r, h.log, err)
		return
	}

	h.recordPayment(r.Context(), "paypal", "success", time.Since(start))
	if h.notifier != nil && webhookAmountCents > 0 {
		h.notifier.NotifyBalanceCredited(r.Context(), 0, webhookAmountCents, "paypal")
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *PaymentWebhookHandler) recordPayment(ctx context.Context, provider, status string, elapsed time.Duration) {
	if h.paymentDuration == nil {
		return
	}
	h.paymentDuration.Record(ctx, elapsed.Seconds(),
		metric.WithAttributes(
			attribute.String("provider", provider),
			attribute.String("status", status),
		),
	)
}

// paypalAmountToCents converts a PayPal decimal amount string (e.g. "12.50",
// "1.5", "100") to integer cents. The fractional part is right-padded to two
// digits so that "1.5" → 150 rather than the naive 105.
func paypalAmountToCents(value string) (int, error) {
	if value == "" {
		return 0, fmt.Errorf("empty amount value")
	}
	parts := strings.SplitN(value, ".", 2)
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid major amount %q: %w", value, err)
	}
	cents := major * 100
	if len(parts) == 2 {
		frac := parts[1]
		for len(frac) < 2 {
			frac += "0"
		}
		fracCents, err := strconv.Atoi(frac[:2])
		if err != nil {
			return 0, fmt.Errorf("invalid fractional amount %q: %w", value, err)
		}
		cents += fracCents
	}
	return cents, nil
}
