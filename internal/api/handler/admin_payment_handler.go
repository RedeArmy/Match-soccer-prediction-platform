package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminPaymentHandler handles admin endpoints for payment management.
type AdminPaymentHandler struct {
	svc service.PaymentService
	log *zap.Logger
}

// NewAdminPaymentHandler constructs an AdminPaymentHandler.
func NewAdminPaymentHandler(svc service.PaymentService, log *zap.Logger) *AdminPaymentHandler {
	return &AdminPaymentHandler{svc: svc, log: log}
}

// ListPending handles GET /admin/payments/pending.
func (h *AdminPaymentHandler) ListPending(w http.ResponseWriter, r *http.Request) {
	records, err := h.svc.ListPending(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	data := make([]PaymentResponse, len(records))
	for i, p := range records {
		data[i] = paymentToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}

// List handles GET /admin/payments — full list with optional filters and pagination.
func (h *AdminPaymentHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	f := repository.PaymentFilters{}
	if s := r.URL.Query().Get("status"); s != "" {
		st := domain.PaymentStatus(s)
		f.Status = &st
	}
	f.QuinielaID = parseOptionalInt(r, "quiniela_id")
	f.UserID = parseOptionalInt(r, "user_id")

	records, err := h.svc.List(r.Context(), f, p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	data := make([]PaymentResponse, len(records))
	for i, rec := range records {
		data[i] = paymentToResponse(rec)
	}
	writeJSON(w, http.StatusOK, Paged[PaymentResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}

type reviewPaymentRequest struct {
	Notes string `json:"notes"`
}

// ValidateDeposit handles POST /admin/payments/{id}/validate.
func (h *AdminPaymentHandler) ValidateDeposit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid payment id"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req reviewPaymentRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // notes is optional

	record, err := h.svc.ValidateDeposit(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, paymentToResponse(record))
}

// RejectDeposit handles POST /admin/payments/{id}/reject.
func (h *AdminPaymentHandler) RejectDeposit(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid payment id"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req reviewPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Notes == "" {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	record, err := h.svc.RejectDeposit(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, paymentToResponse(record))
}

// ListByGroup handles GET /admin/groups/{id}/payments.
func (h *AdminPaymentHandler) ListByGroup(w http.ResponseWriter, r *http.Request) {
	groupID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || groupID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid group id"))
		return
	}

	records, err := h.svc.ListByQuiniela(r.Context(), groupID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	data := make([]PaymentResponse, len(records))
	for i, p := range records {
		data[i] = paymentToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}
