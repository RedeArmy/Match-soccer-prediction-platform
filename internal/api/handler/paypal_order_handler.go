package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PayPalOrderHandler creates PayPal checkout orders server-side.
// The client secret never leaves the backend; the frontend receives only the order ID.
type PayPalOrderHandler struct {
	clientID     string
	clientSecret string
	baseURL      string
	svc          service.PaymentIntentCreator
	client       *http.Client
	log          *zap.Logger
}

// NewPayPalOrderHandler constructs a PayPalOrderHandler.
// baseURL defaults to sandbox when empty and the environment is not production.
func NewPayPalOrderHandler(clientID, clientSecret, baseURL string, isDev bool, svc service.PaymentIntentCreator, log *zap.Logger) *PayPalOrderHandler {
	if baseURL == "" {
		if isDev {
			baseURL = "https://api-m.sandbox.paypal.com"
		} else {
			baseURL = "https://api-m.paypal.com"
		}
	}
	return &PayPalOrderHandler{
		clientID:     clientID,
		clientSecret: clientSecret,
		baseURL:      baseURL,
		svc:          svc,
		client:       &http.Client{Timeout: 10 * time.Second},
		log:          log,
	}
}

type createPayPalOrderRequest struct {
	AmountCents int    `json:"amount_cents"`
	Currency    string `json:"currency"`
}

type createPayPalOrderResponse struct {
	ID string `json:"id"`
}

// Create handles POST /api/v1/paypal/create-order.
// It creates a payment intent in the database (minting the opaque token used as
// PayPal custom_id), then creates the v2/checkout/orders record at PayPal.
// The frontend only needs to send amount_cents + currency; everything else is
// resolved server-side so the PayPal client secret never reaches the browser.
func (h *PayPalOrderHandler) Create(w http.ResponseWriter, r *http.Request) {
	if h.clientID == "" || h.clientSecret == "" {
		h.log.Error("PayPal credentials not configured")
		writeJSON(w, http.StatusServiceUnavailable,
			map[string]string{"error": "PayPal no está configurado en el servidor"})
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised("authentication is required"))
		return
	}

	var req createPayPalOrderRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil || req.AmountCents <= 0 || req.Currency == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Cuerpo de solicitud inválido"})
		return
	}

	// Mint the payment intent — its Token becomes PayPal's custom_id so the
	// webhook handler can credit the correct user on capture.
	intent, err := h.svc.Create(r.Context(), caller.ID, req.AmountCents, req.Currency)
	if err != nil {
		h.log.Error("PayPal: create payment intent failed", zap.Error(err))
		writeError(w, r, h.log, err)
		return
	}

	accessToken, err := h.getAccessToken(r.Context())
	if err != nil {
		h.log.Error("PayPal access token error", zap.Error(err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "No se pudo autenticar con PayPal"})
		return
	}

	amountUSD := fmt.Sprintf("%.2f", float64(req.AmountCents)/100.0)
	orderID, err := h.createOrder(r.Context(), accessToken, amountUSD, intent.Token)
	if err != nil {
		h.log.Error("PayPal create order error", zap.Error(err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "PayPal rechazó la orden"})
		return
	}

	writeJSON(w, http.StatusCreated, createPayPalOrderResponse{ID: orderID})
}

func (h *PayPalOrderHandler) getAccessToken(ctx context.Context) (string, error) {
	creds := base64.StdEncoding.EncodeToString([]byte(h.clientID + ":" + h.clientSecret))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/v1/oauth2/token",
		strings.NewReader("grant_type=client_credentials"),
	)
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal token request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("paypal auth %d: %s", resp.StatusCode, body)
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("paypal token decode: %w", err)
	}
	return out.AccessToken, nil
}

func (h *PayPalOrderHandler) createOrder(ctx context.Context, token, amountUSD, customID string) (string, error) {
	payload := fmt.Sprintf(
		`{"intent":"CAPTURE","purchase_units":[{"amount":{"currency_code":"USD","value":%q},"custom_id":%q}]}`,
		amountUSD, customID,
	)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		h.baseURL+"/v2/checkout/orders",
		strings.NewReader(payload),
	)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("paypal order request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("paypal order %d: %s", resp.StatusCode, body)
	}
	var out struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("paypal order decode: %w", err)
	}
	return out.ID, nil
}
