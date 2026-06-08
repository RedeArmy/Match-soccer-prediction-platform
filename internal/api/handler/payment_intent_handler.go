package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// RecurrenteCheckoutCreator opens a hosted Recurrente checkout session and
// returns the URL the user must be redirected to.
type RecurrenteCheckoutCreator interface {
	CreateCheckout(ctx context.Context, userID int, amountCents int, currency, reference, successURL, cancelURL string) (checkoutURL string, err error)
}

// PaymentIntentHandler handles creation of server-side payment intents.
// For PayPal the handler mints an opaque token used as custom_id on the
// PayPal order; for Recurrente it creates a hosted checkout session and
// returns the redirect URL directly.
type PaymentIntentHandler struct {
	svc        service.PaymentIntentCreator
	recurrente RecurrenteCheckoutCreator // nil when Recurrente is not configured
	appBaseURL string                    // e.g. "https://app.quinielamundial.gt"
	log        *zap.Logger
}

// NewPaymentIntentHandler constructs a PaymentIntentHandler.
func NewPaymentIntentHandler(svc service.PaymentIntentCreator, log *zap.Logger) *PaymentIntentHandler {
	return &PaymentIntentHandler{svc: svc, log: log}
}

// WithRecurrente wires in the Recurrente checkout creator and the app base URL
// used to build success/cancel redirect URLs. Call once at composition time.
func (h *PaymentIntentHandler) WithRecurrente(r RecurrenteCheckoutCreator, appBaseURL string) {
	h.recurrente = r
	h.appBaseURL = appBaseURL
}

type createPaymentIntentRequest struct {
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
	Provider    string `json:"provider"` // "recurrente" | "paypal"; empty treated as "paypal"
}

// PaymentIntentResponse is the JSON body returned for a successfully created intent.
type PaymentIntentResponse struct {
	Token       string    `json:"token"`
	AmountCents int       `json:"amount_cents"`
	Currency    string    `json:"currency"`
	ExpiresAt   time.Time `json:"expires_at"`
	RedirectURL string    `json:"redirect_url,omitempty"`
}

// Create handles POST /api/v1/payment-intents.
//
// @Summary      Create payment intent
// @Description  For PayPal: mints an opaque single-use token to pass as PayPal custom_id.
// @Description  For Recurrente: creates a hosted checkout session and returns a redirect URL.
// @Tags         payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.createPaymentIntentRequest  true  "Amount, currency and provider"
// @Success      201   {object}  handler.PaymentIntentResponse
// @Failure      400   {object}  handler.ErrorResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Router       /api/v1/payment-intents [post]
func (h *PaymentIntentHandler) Create(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[createPaymentIntentRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if req.Provider == "recurrente" {
		h.createRecurrenteCheckout(w, r, caller.ID, req.AmountCents, req.Currency)
		return
	}

	// Default (PayPal and anything unrecognised): mint an intent token.
	intent, err := h.svc.Create(r.Context(), caller.ID, req.AmountCents, req.Currency)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusCreated, PaymentIntentResponse{
		Token:       intent.Token,
		AmountCents: intent.AmountCents,
		Currency:    intent.Currency,
		ExpiresAt:   intent.ExpiresAt,
	})
}

// createRecurrenteCheckout handles the Recurrente branch of Create.
func (h *PaymentIntentHandler) createRecurrenteCheckout(w http.ResponseWriter, r *http.Request, userID, amountCents int, currency string) {
	if h.recurrente == nil {
		h.log.Warn("recurrente checkout requested but client is not configured (WCQ_PAYMENT_RECURRENTEAPIKEY)")
		writeError(w, r, h.log, apperrors.Validation("Recurrente payments are not available"))
		return
	}

	if strings.Contains(h.appBaseURL, "localhost") || strings.Contains(h.appBaseURL, "127.0.0.1") {
		writeError(w, r, h.log, apperrors.Validation(
			"Recurrente requires a public URL for redirects — set WCQ_SERVER_APPBASEURL to an ngrok or production URL",
		))
		return
	}

	ref, err := generateCheckoutReference()
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}

	successURL := h.appBaseURL + "/balance?deposit=success"
	cancelURL := h.appBaseURL + "/balance/deposit"

	h.log.Debug("recurrente: creating checkout",
		zap.String("success_url", successURL),
		zap.String("cancel_url", cancelURL))

	redirectURL, err := h.recurrente.CreateCheckout(r.Context(), userID, amountCents, currency, ref, successURL, cancelURL)
	if err != nil {
		h.log.Error("recurrente: failed to create checkout",
			zap.Int("user_id", userID),
			zap.Int("amount_cents", amountCents),
			zap.String("success_url", successURL),
			zap.String("cancel_url", cancelURL),
			zap.Error(err))
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("create recurrente checkout: %w", err)))
		return
	}

	writeJSON(w, http.StatusCreated, PaymentIntentResponse{
		AmountCents: amountCents,
		Currency:    currency,
		ExpiresAt:   time.Now().Add(30 * time.Minute),
		RedirectURL: redirectURL,
	})
}

// generateCheckoutReference returns a 16-byte hex string used as wcq_reference
// in the Recurrente checkout metadata so the webhook can be correlated.
func generateCheckoutReference() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate checkout reference: %w", err)
	}
	return hex.EncodeToString(b), nil
}
