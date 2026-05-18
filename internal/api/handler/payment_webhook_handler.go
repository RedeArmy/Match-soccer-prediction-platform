package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PaymentWebhookHandler handles incoming payment confirmation webhooks from
// Recurrente and PayPal. Signature verification is performed by upstream
// middleware (middleware.RecurrenteWebhookAuth, middleware.PayPalWebhookAuth),
// which stamps the verified body into the request context. Handlers assert
// the presence of that sentinel before processing; a missing sentinel means
// the request reached this handler without passing through the auth middleware,
// which is treated as unauthorised.
type PaymentWebhookHandler struct {
	svc service.WebhookPaymentService
	log *zap.Logger
}

// NewPaymentWebhookHandler constructs a PaymentWebhookHandler.
func NewPaymentWebhookHandler(svc service.WebhookPaymentService, log *zap.Logger) *PaymentWebhookHandler {
	return &PaymentWebhookHandler{svc: svc, log: log}
}

// recurrenteWebhookPayload is the minimal set of fields we extract from a
// Recurrente payment-confirmed event.
type recurrenteWebhookPayload struct {
	// EventType should be "payment.confirmed" for us to credit the balance.
	EventType string `json:"event_type"`
	// Data contains the payment object.
	Data struct {
		Reference   string `json:"reference"`
		AmountCents int    `json:"amount_cents"`
		Currency    string `json:"currency"`
		// UserID is the metadata field we embed when creating the payment.
		UserID int `json:"user_id"`
	} `json:"data"`
}

// HandleRecurrente handles POST /webhooks/recurrente.
//
// @Summary      Recurrente payment webhook
// @Description  Receives payment confirmation events from Recurrente. Credits the user's balance on confirmed events.
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

	var payload recurrenteWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		writeError(w, r, h.log, apperrors.Validation("could not parse Recurrente webhook payload"))
		return
	}

	if payload.EventType != "payment.confirmed" {
		h.log.Debug("recurrente webhook: ignoring event", zap.String("event_type", payload.EventType))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	d := payload.Data
	if d.UserID <= 0 || d.AmountCents <= 0 || d.Reference == "" {
		writeError(w, r, h.log, apperrors.Validation("missing required fields in Recurrente webhook data"))
		return
	}

	if err := h.svc.CreditFromRecurrente(r.Context(), d.UserID, d.AmountCents, d.Currency, d.Reference); err != nil {
		h.log.Error("recurrente webhook: failed to credit balance",
			zap.String("reference", d.Reference),
			zap.Int("user_id", d.UserID),
			zap.Error(err),
		)
		writeError(w, r, h.log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// paypalWebhookPayload is the minimal set of fields we extract from a PayPal
// PAYMENT.CAPTURE.COMPLETED event.
type paypalWebhookPayload struct {
	EventType string `json:"event_type"`
	Resource  struct {
		// ID is the PayPal capture ID — used as the idempotency key.
		ID string `json:"id"`
		// CustomID is the server-generated payment intent token. It is set by
		// the frontend when creating the PayPal order using the token returned
		// by POST /api/v1/payment-intents. The token is unguessable and binds
		// the order to a specific user — it cannot be substituted.
		CustomID string `json:"custom_id"`
		// Amount carries the declared capture amount for defence-in-depth cross-checking.
		Amount struct {
			Value        string `json:"value"`
			CurrencyCode string `json:"currency_code"`
		} `json:"amount"`
	} `json:"resource"`
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

	if err := h.svc.ResolveAndCreditPayPalIntent(r.Context(), intentToken, captureID, webhookAmountCents); err != nil {
		h.log.Error("paypal webhook: failed to resolve intent and credit balance",
			zap.String("capture_id", captureID),
			zap.String("intent_token", intentToken),
			zap.Error(err),
		)
		writeError(w, r, h.log, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
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
