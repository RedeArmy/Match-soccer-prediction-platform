package handler

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/go-chi/chi/v5"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
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
type PaymentIntentHandler struct {
	svc        service.PaymentIntentCreator
	recurrente RecurrenteCheckoutCreator
	fileStore  storage.FileStore
	maxUpload  int64
	appBaseURL string
	log        *zap.Logger
}

// NewPaymentIntentHandler constructs a PaymentIntentHandler.
func NewPaymentIntentHandler(svc service.PaymentIntentCreator, log *zap.Logger) *PaymentIntentHandler {
	return &PaymentIntentHandler{svc: svc, log: log}
}

// WithRecurrente wires in the Recurrente checkout creator and the app base URL.
func (h *PaymentIntentHandler) WithRecurrente(r RecurrenteCheckoutCreator, appBaseURL string) {
	h.recurrente = r
	h.appBaseURL = appBaseURL
}

// WithFileStore wires in the file store for comprobante uploads.
// maxUploadBytes caps the accepted file size (0 uses domain default).
func (h *PaymentIntentHandler) WithFileStore(fs storage.FileStore, maxUploadBytes int64) {
	h.fileStore = fs
	if maxUploadBytes > 0 {
		h.maxUpload = maxUploadBytes
	} else {
		h.maxUpload = 10 << 20 // 10 MB default
	}
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
// It first persists a payment_intent record so the admin panel can track the
// checkout lifecycle even when the webhook is delayed or never arrives.
func (h *PaymentIntentHandler) createRecurrenteCheckout(w http.ResponseWriter, r *http.Request, userID, amountCents int, currency string) {
	if h.recurrente == nil {
		h.log.Warn("recurrente checkout requested but client is not configured (WCQ_PAYMENT_RECURRENTEAPIKEY)")
		writeError(w, r, h.log, apperrors.Validation("Recurrente payments are not available"))
		return
	}

	if h.appBaseURL == "" || strings.Contains(h.appBaseURL, "localhost") || strings.Contains(h.appBaseURL, "127.0.0.1") {
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

	// Persist the intent before creating the external checkout so that the
	// admin panel immediately shows this pending payment.
	intent, err := h.svc.CreateForRecurrente(r.Context(), userID, amountCents, currency, ref)
	if err != nil {
		h.log.Error("recurrente: failed to persist payment intent",
			zap.Int("user_id", userID),
			zap.Error(err))
		writeError(w, r, h.log, err)
		return
	}

	successURL := fmt.Sprintf("%s/balance?deposit=success&amount=%d&currency=%s",
		h.appBaseURL, amountCents, url.QueryEscape(currency))
	cancelURL := h.appBaseURL + "/balance/deposit"

	h.log.Debug("recurrente: creating checkout",
		zap.String("success_url", successURL),
		zap.String("cancel_url", cancelURL))

	redirectURL, err := h.recurrente.CreateCheckout(r.Context(), userID, amountCents, currency, ref, successURL, cancelURL)
	if err != nil {
		h.log.Error("recurrente: failed to create checkout",
			zap.Int("user_id", userID),
			zap.Int("amount_cents", amountCents),
			zap.Error(err))
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("create recurrente checkout: %w", err)))
		return
	}

	writeJSON(w, http.StatusCreated, PaymentIntentResponse{
		Token:       intent.Token,
		AmountCents: intent.AmountCents,
		Currency:    intent.Currency,
		ExpiresAt:   intent.ExpiresAt,
		RedirectURL: redirectURL,
	})
}

// ListMy handles GET /api/v1/payment-intents/my.
// Returns the caller's non-captured payment intents so the balance page can
// surface actionable pending items.
func (h *PaymentIntentHandler) ListMy(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	intents, err := h.svc.ListMyPending(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := make([]paymentIntentSummary, len(intents))
	for i, pi := range intents {
		resp[i] = intentToSummary(pi)
	}
	writeJSON(w, http.StatusOK, resp)
}

// UploadComprobante handles POST /api/v1/payment-intents/{token}/comprobante.
// Accepts a multipart/form-data upload with a single "file" field.
func (h *PaymentIntentHandler) UploadComprobante(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, r, h.log, apperrors.Validation("token is required"))
		return
	}

	if h.fileStore == nil {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("file store not configured")))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeError(w, r, h.log, apperrors.Validation("could not parse multipart form"))
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, r, h.log, apperrors.Validation("file field is required"))
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	// Read fully so we can detect size and stream to the store.
	full, err := io.ReadAll(io.LimitReader(file, h.maxUpload+1))
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("read file: %w", err)))
		return
	}
	if int64(len(full)) > h.maxUpload {
		writeError(w, r, h.log, apperrors.RequestBodyTooLarge())
		return
	}

	// Look up the intent to verify ownership and capture provider/created_at for the key.
	pending, err := h.svc.ListMyPending(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	var foundIntent *domain.PaymentIntent
	for _, pi := range pending {
		if pi.Token == token {
			foundIntent = pi
			break
		}
	}
	if foundIntent == nil {
		writeError(w, r, h.log, apperrors.NotFound("payment intent not found or not pending"))
		return
	}

	// Key format: comprobantes/voucher_{userID}_{provider}_{intentCreatedAt}
	// e.g. comprobantes/voucher_42_recurrente_2026-06-18T15-04-05
	ts := foundIntent.CreatedAt.UTC().Format("2006-01-02T15-04-05")
	storageKey := fmt.Sprintf("comprobantes/voucher_%d_%s_%s", foundIntent.UserID, foundIntent.Provider, ts)

	if err := h.fileStore.Put(r.Context(), storageKey, contentType, bytes.NewReader(full), int64(len(full))); err != nil {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("store comprobante: %w", err)))
		return
	}

	if err := h.svc.SetComprobanteByToken(r.Context(), token, storageKey, contentType, len(full)); err != nil {
		_ = h.fileStore.Delete(r.Context(), storageKey) // best-effort cleanup
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "uploaded"})
}

// paymentIntentSummary is the JSON shape returned for user-facing intents.
type paymentIntentSummary struct {
	ID                  int64  `json:"id"`
	Token               string `json:"token"`
	AmountCents         int    `json:"amount_cents"`
	Currency            string `json:"currency"`
	Provider            string `json:"provider"`
	Status              string `json:"status"`
	HasComprobante      bool   `json:"has_comprobante"`
	ComprobanteRequired bool   `json:"comprobante_required"`
	UserNotes           string `json:"user_notes"`
	ExpiresAt           string `json:"expires_at"`
	CreatedAt           string `json:"created_at"`
}

func intentToSummary(pi *domain.PaymentIntent) paymentIntentSummary {
	return paymentIntentSummary{
		ID:                  pi.ID,
		Token:               pi.Token,
		AmountCents:         pi.AmountCents,
		Currency:            pi.Currency,
		Provider:            pi.Provider,
		Status:              string(pi.Status),
		HasComprobante:      pi.ComprobanteKey != nil,
		ComprobanteRequired: pi.ComprobanteRequired,
		UserNotes:           pi.UserNotes,
		ExpiresAt:           pi.ExpiresAt.Format(time.RFC3339),
		CreatedAt:           pi.CreatedAt.Format(time.RFC3339),
	}
}

// ListMyAll handles GET /api/v1/payment-intents/my/all.
// Returns all payment intents for the caller, ordered newest first.
// Used on the balance page to render the full colored payment history.
func (h *PaymentIntentHandler) ListMyAll(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	intents, err := h.svc.ListMyAll(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := make([]paymentIntentSummary, len(intents))
	for i, pi := range intents {
		resp[i] = intentToSummary(pi)
	}
	writeJSON(w, http.StatusOK, resp)
}

// ResubmitForReview handles POST /api/v1/payment-intents/{token}/resubmit.
// Accepts multipart/form-data with a required "notes" field and an optional "file".
// Transitions a rejected intent to under_review so the admin can re-evaluate.
func (h *PaymentIntentHandler) ResubmitForReview(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	token := chi.URLParam(r, "token")
	if token == "" {
		writeError(w, r, h.log, apperrors.Validation("token is required"))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	if err := r.ParseMultipartForm(4 << 20); err != nil {
		writeError(w, r, h.log, apperrors.Validation("could not parse form"))
		return
	}

	notes := r.FormValue("notes")
	if notes == "" {
		writeError(w, r, h.log, apperrors.Validation("notes are required"))
		return
	}

	// Look up the intent (which may be rejected or under_review) to obtain the
	// provider and creation timestamp needed for the storage key.
	allIntents, err := h.svc.ListMyAll(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	var resubmitIntent *domain.PaymentIntent
	for _, pi := range allIntents {
		if pi.Token == token {
			resubmitIntent = pi
			break
		}
	}
	if resubmitIntent == nil {
		writeError(w, r, h.log, apperrors.NotFound("payment intent not found"))
		return
	}

	var (
		comprobanteKey  *string
		comprobanteType *string
		comprobanteSize *int
	)

	file, header, fileErr := r.FormFile("file")
	if fileErr == nil {
		defer file.Close()

		if h.fileStore == nil {
			writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("file store not configured")))
			return
		}

		ct := header.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/octet-stream"
		}

		full, err := io.ReadAll(io.LimitReader(file, h.maxUpload+1))
		if err != nil {
			writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("read file: %w", err)))
			return
		}
		if int64(len(full)) > h.maxUpload {
			writeError(w, r, h.log, apperrors.RequestBodyTooLarge())
			return
		}

		// Key format: comprobantes/voucher_{userID}_{provider}_{intentCreatedAt}_review
		// The _review suffix distinguishes re-submissions from the initial upload.
		ts := resubmitIntent.CreatedAt.UTC().Format("2006-01-02T15-04-05")
		storageKey := fmt.Sprintf("comprobantes/voucher_%d_%s_%s_review", resubmitIntent.UserID, resubmitIntent.Provider, ts)
		if err := h.fileStore.Put(r.Context(), storageKey, ct, bytes.NewReader(full), int64(len(full))); err != nil {
			writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("store comprobante: %w", err)))
			return
		}

		sz := len(full)
		comprobanteKey = &storageKey
		comprobanteType = &ct
		comprobanteSize = &sz
	}

	updated, err := h.svc.ResubmitForReview(r.Context(), caller.ID, token, comprobanteKey, comprobanteType, comprobanteSize, notes)
	if err != nil {
		if comprobanteKey != nil {
			_ = h.fileStore.Delete(r.Context(), *comprobanteKey)
		}
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, intentToSummary(updated))
}

// generateCheckoutReference returns a 16-byte hex string used as wcq_reference.
func generateCheckoutReference() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate checkout reference: %w", err)
	}
	return hex.EncodeToString(b), nil
}
