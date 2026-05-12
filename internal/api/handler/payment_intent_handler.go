package handler

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PaymentIntentHandler handles creation of server-side payment intents used
// as opaque PayPal custom_id values. The token prevents user-ID substitution:
// only the server that generated the token can resolve it to a user.
type PaymentIntentHandler struct {
	svc service.PaymentIntentService
	log *zap.Logger
}

// NewPaymentIntentHandler constructs a PaymentIntentHandler.
func NewPaymentIntentHandler(svc service.PaymentIntentService, log *zap.Logger) *PaymentIntentHandler {
	return &PaymentIntentHandler{svc: svc, log: log}
}

type createPaymentIntentRequest struct {
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// PaymentIntentResponse is the JSON body returned for a successfully created intent.
type PaymentIntentResponse struct {
	Token       string    `json:"token"`
	AmountCents int       `json:"amount_cents"`
	Currency    string    `json:"currency"`
	ExpiresAt   time.Time `json:"expires_at"`
}

// Create handles POST /api/v1/payment-intents.
//
// @Summary      Create payment intent
// @Description  Generates an opaque single-use payment intent token to be passed as
//
//	PayPal custom_id when the client creates a PayPal order. The token binds
//	the order to the calling user, preventing user-ID substitution attacks.
//
// @Tags         payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.createPaymentIntentRequest  true  "Amount and currency"
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
