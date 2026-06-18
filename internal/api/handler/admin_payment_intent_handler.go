package handler

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// adminIntentStore is the narrow repository interface used by AdminPaymentIntentHandler.
type adminIntentStore interface {
	ListForAdmin(ctx context.Context, f repository.PaymentIntentFilters, p repository.Pagination) ([]*domain.PaymentIntent, int, error)
	GetByID(ctx context.Context, id int64) (*domain.PaymentIntent, error)
	AdminCreditExpired(ctx context.Context, id int64, adminID, creditAmountCents int, notes string) (*domain.PaymentIntent, error)
	AdminReject(ctx context.Context, id int64, adminID int, notes string) (*domain.PaymentIntent, error)
	RequestComprobante(ctx context.Context, id int64, adminID int) (*domain.PaymentIntent, error)
}

// AdminPaymentIntentHandler serves admin endpoints for PayPal and Recurrente
// payment intent management.
type AdminPaymentIntentHandler struct {
	intentRepo adminIntentStore
	fxSvc      service.ExchangeRateService
	fileStore  storage.FileStore
	log        *zap.Logger
}

// NewAdminPaymentIntentHandler constructs an AdminPaymentIntentHandler.
func NewAdminPaymentIntentHandler(intentRepo adminIntentStore, fileStore storage.FileStore, log *zap.Logger) *AdminPaymentIntentHandler {
	return &AdminPaymentIntentHandler{intentRepo: intentRepo, fileStore: fileStore, log: log}
}

// SetExchangeRateService wires in the FX service for USD→GTQ conversion on
// manual credit of USD-denominated PayPal intents. Nil disables conversion
// (raw amount is credited without conversion).
func (h *AdminPaymentIntentHandler) SetExchangeRateService(svc service.ExchangeRateService) {
	h.fxSvc = svc
}

// paymentIntentAdminResponse is the JSON shape returned for admin list / detail.
type paymentIntentAdminResponse struct {
	ID                  int64   `json:"id"`
	Token               string  `json:"token"`
	UserID              int     `json:"user_id"`
	AmountCents         int     `json:"amount_cents"`
	Currency            string  `json:"currency"`
	Provider            string  `json:"provider"`
	Status              string  `json:"status"`
	CaptureID           *string `json:"capture_id"`
	HasComprobante      bool    `json:"has_comprobante"`
	ComprobanteRequired bool    `json:"comprobante_required"`
	UserNotes           string  `json:"user_notes"`
	ReviewedBy          *int    `json:"reviewed_by"`
	ReviewNotes         string  `json:"review_notes"`
	ExpiresAt           string  `json:"expires_at"`
	RejectedAt          *string `json:"rejected_at"`
	CreatedAt           string  `json:"created_at"`
	UpdatedAt           string  `json:"updated_at"`
}

func intentToAdminResponse(pi *domain.PaymentIntent) paymentIntentAdminResponse {
	r := paymentIntentAdminResponse{
		ID:                  pi.ID,
		Token:               pi.Token,
		UserID:              pi.UserID,
		AmountCents:         pi.AmountCents,
		Currency:            pi.Currency,
		Provider:            pi.Provider,
		Status:              string(pi.Status),
		CaptureID:           pi.CaptureID,
		HasComprobante:      pi.ComprobanteKey != nil,
		ComprobanteRequired: pi.ComprobanteRequired,
		UserNotes:           pi.UserNotes,
		ReviewedBy:          pi.ReviewedBy,
		ReviewNotes:         pi.ReviewNotes,
		ExpiresAt:           pi.ExpiresAt.Format(time.RFC3339),
		CreatedAt:           pi.CreatedAt.Format(time.RFC3339),
		UpdatedAt:           pi.UpdatedAt.Format(time.RFC3339),
	}
	if pi.RejectedAt != nil {
		s := pi.RejectedAt.Format(time.RFC3339)
		r.RejectedAt = &s
	}
	return r
}

// List handles GET /api/v1/admin/payment-intents.
// Accepts optional ?provider=paypal|recurrente&status=pending|captured|expired|rejected&page=1&limit=15
func (h *AdminPaymentIntentHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	f := repository.PaymentIntentFilters{}
	if prov := r.URL.Query().Get("provider"); prov != "" {
		f.Provider = &prov
	}
	if st := r.URL.Query().Get("status"); st != "" {
		s := domain.PaymentIntentStatus(st)
		f.Status = &s
	}

	intents, total, err := h.intentRepo.ListForAdmin(r.Context(), f, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]paymentIntentAdminResponse, len(intents))
	for i, pi := range intents {
		data[i] = intentToAdminResponse(pi)
	}
	type pagedWithTotal struct {
		Data  []paymentIntentAdminResponse `json:"data"`
		Page  PageMeta                     `json:"page"`
		Total int                          `json:"total"`
	}
	writeJSON(w, http.StatusOK, pagedWithTotal{
		Data:  data,
		Page:  PageMeta{Limit: p.Limit, Offset: p.Offset},
		Total: total,
	})
}

type adminCreditIntentRequest struct {
	Notes string `json:"notes"`
}

// Credit handles POST /api/v1/admin/payment-intents/{id}/credit.
// Manually credits the intent amount to the user's balance. For USD intents
// the amount is converted to GTQ at the current buy rate.
func (h *AdminPaymentIntentHandler) Credit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidIntentID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req adminCreditIntentRequest
	_ = decodeJSONOptional(r, &req)

	// Load the intent to determine the credit amount.
	intent, err := h.intentRepo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if intent == nil {
		writeError(w, r, h.log, apperrors.NotFound("payment intent not found"))
		return
	}

	creditAmountCents := intent.AmountCents
	if intent.Currency == "USD" && h.fxSvc != nil {
		gtq, _, convErr := h.fxSvc.ConvertUSDToGTQ(r.Context(), int64(intent.AmountCents))
		if convErr != nil {
			h.log.Warn("admin credit: USD→GTQ conversion failed, crediting raw amount",
				zap.Error(convErr),
				zap.Int64("intent_id", id),
			)
		} else {
			creditAmountCents = int(gtq)
		}
	}

	updated, err := h.intentRepo.AdminCreditExpired(r.Context(), id, caller.ID, creditAmountCents, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, intentToAdminResponse(updated))
}

type adminRejectIntentRequest struct {
	Notes string `json:"notes"`
}

// Reject handles POST /api/v1/admin/payment-intents/{id}/reject.
func (h *AdminPaymentIntentHandler) Reject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidIntentID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[adminRejectIntentRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Notes == "" {
		writeError(w, r, h.log, apperrors.Validation("notes are required for rejection"))
		return
	}

	updated, err := h.intentRepo.AdminReject(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, intentToAdminResponse(updated))
}

// RequestComprobante handles POST /api/v1/admin/payment-intents/{id}/request-comprobante.
// Sets comprobante_required=true on a pending intent so the user is prompted to
// upload a receipt on their balance page.
func (h *AdminPaymentIntentHandler) RequestComprobante(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidIntentID))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	updated, err := h.intentRepo.RequestComprobante(r.Context(), id, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, intentToAdminResponse(updated))
}

// DownloadComprobante handles GET /api/v1/admin/payment-intents/{id}/comprobante.
func (h *AdminPaymentIntentHandler) DownloadComprobante(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidIntentID))
		return
	}

	intent, err := h.intentRepo.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if intent == nil || intent.ComprobanteKey == nil {
		writeError(w, r, h.log, apperrors.NotFound("comprobante not found"))
		return
	}

	if h.fileStore == nil {
		writeError(w, r, h.log, apperrors.Internal(fmt.Errorf("file store not configured")))
		return
	}

	rc, ct, err := h.fileStore.Get(r.Context(), *intent.ComprobanteKey)
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}
	defer func() { _ = rc.Close() }()

	if ct != "" {
		w.Header().Set("Content-Type", ct)
	} else if intent.ComprobanteContentType != nil {
		w.Header().Set("Content-Type", *intent.ComprobanteContentType)
	}
	w.Header().Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, rc); err != nil {
		h.log.Warn("admin comprobante: stream interrupted", zap.Int64("intent_id", id), zap.Error(err))
	}
}
