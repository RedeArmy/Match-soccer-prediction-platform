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
//
// @Summary      List pending payments
// @Description  Returns all payment records with status=pending that are awaiting
//
//	admin review. Requires admin role.
//
// @Tags         admin-payments
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.PaymentResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/payments/pending [get]
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
//
// @Summary      List all payments
// @Description  Returns a paginated list of all payment records. Supports optional
//
//	filtering by status, quiniela, and user. Requires admin role.
//
// @Tags         admin-payments
// @Produce      json
// @Security     BearerAuth
// @Param        status      query     string  false  "Filter by status (pending, confirmed, rejected, refunded)"
// @Param        quiniela_id query     int     false  "Filter by group ID"
// @Param        user_id     query     int     false  "Filter by user ID"
// @Param        limit       query     int     false  "Max records per page (default 50, max 200)"
// @Param        page        query     int     false  "Page number (default 1)"
// @Success      200         {object}  handler.Paged[handler.PaymentResponse]
// @Failure      401         {object}  handler.ErrorResponse
// @Failure      403         {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500         {object}  handler.ErrorResponse
// @Router       /api/v1/admin/payments [get]
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
//
// @Summary      Validate a payment
// @Description  Confirms a pending payment deposit. Marks the payment as confirmed
//
//	and unlocks the user's membership (paid=true). Optional notes are
//	recorded for the audit trail. Requires admin role.
//
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          false  "Payment ID"
// @Param        body  body      handler.reviewPaymentRequest false  "Optional review notes"
// @Success      200   {object}  handler.PaymentResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404   {object}  handler.ErrorResponse  "Payment not found"
// @Failure      422   {object}  handler.ErrorResponse  "Invalid payment ID"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/payments/{id}/validate [post]
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
//
// @Summary      Reject a payment
// @Description  Marks a pending payment deposit as rejected. The rejection reason
//
//	must be provided in the notes field. Requires admin role.
//
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                          true  "Payment ID"
// @Param        body  body      handler.reviewPaymentRequest true  "Rejection notes (required)"
// @Success      200   {object}  handler.PaymentResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404   {object}  handler.ErrorResponse  "Payment not found"
// @Failure      422   {object}  handler.ErrorResponse  "notes are required"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/payments/{id}/reject [post]
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

	req, err := decodeJSON[reviewPaymentRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	if req.Notes == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("notes are required"))
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
//
// @Summary      List payments for a group
// @Description  Returns all payment records for the given group, regardless of
//
//	status. Requires admin role.
//
// @Tags         admin-groups
// @Produce      json
// @Security     BearerAuth
// @Param        id   path      int  true  "Group ID"
// @Success      200  {array}   handler.PaymentResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid group ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/groups/{id}/payments [get]
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
