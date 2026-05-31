package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// withdrawalObservabilityNotifier is the narrow slice of ObservabilityNotifier
// consumed by WithdrawalHandler.
type withdrawalObservabilityNotifier interface {
	NotifyPayoutApproved(ctx context.Context, userID, amountCents int, method, adminID string)
}

// WithdrawalHandler handles user withdrawal request endpoints.
type WithdrawalHandler struct {
	svc      service.WithdrawalService
	log      *zap.Logger
	notifier withdrawalObservabilityNotifier // nil = disabled
}

// NewWithdrawalHandler constructs a WithdrawalHandler.
func NewWithdrawalHandler(svc service.WithdrawalService, log *zap.Logger) *WithdrawalHandler {
	return &WithdrawalHandler{svc: svc, log: log}
}

// SetNotifier wires an ObservabilityNotifier for payout-approved events.
// Call at composition time (buildHandlers) before any requests are served.
func (h *WithdrawalHandler) SetNotifier(n withdrawalObservabilityNotifier) {
	h.notifier = n
}

type createWithdrawalRequest struct {
	AmountCents   int               `json:"amount_cents"`
	Currency      string            `json:"currency"`
	Method        string            `json:"method"`
	PayoutDetails map[string]string `json:"payout_details"`
}

// Create handles POST /api/v1/withdrawals.
//
// @Summary      Request withdrawal
// @Description  Creates a withdrawal request and reserves the amount from the user's available balance.
//
//	method must be one of: bank_gt (Guatemalan bank transfer), paypal.
//	currency defaults to GTQ when omitted.
//	All monetary values are in minor currency units (e.g. centavos for GTQ).
//
// @Tags         balance
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.createWithdrawalRequest  true  "Withdrawal details"
// @Success      201  {object}  handler.WithdrawalResponse
// @Failure      400  {object}  handler.ErrorResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      409  {object}  handler.ErrorResponse  "Insufficient balance"
// @Failure      422  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/withdrawals [post]
func (h *WithdrawalHandler) Create(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[createWithdrawalRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.AmountCents <= 0 {
		writeError(w, r, h.log, apperrors.Validation("amount_cents must be positive"))
		return
	}
	method := domain.WithdrawalMethod(req.Method)
	if method != domain.WithdrawalMethodBankGT && method != domain.WithdrawalMethodPayPal {
		writeError(w, r, h.log, apperrors.Validation("method must be bank_gt or paypal"))
		return
	}
	if err := domain.ValidatePayoutDetails(method, req.PayoutDetails); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	currency := req.Currency
	if currency == "" {
		currency = "GTQ"
	}
	if err := domain.ValidateWithdrawalCurrency(currency); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	result, err := h.svc.Create(r.Context(), caller.ID, req.AmountCents, currency, method, req.PayoutDetails)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, withdrawalToResponse(result))
}

// ListMine handles GET /api/v1/withdrawals.
//
// @Summary      List my withdrawals
// @Description  Returns all withdrawal requests submitted by the authenticated user.
// @Tags         balance
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.WithdrawalResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/withdrawals [get]
func (h *WithdrawalHandler) ListMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	requests, err := h.svc.ListByUser(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]WithdrawalResponse, len(requests))
	for i, req := range requests {
		data[i] = withdrawalToResponse(req)
	}
	writeJSON(w, http.StatusOK, data)
}

// AdminListPending handles GET /admin/withdrawals/pending.
//
// @Summary      List pending withdrawals
// @Description  Returns all withdrawal requests awaiting review. Requires admin role.
// @Tags         admin-payments
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.WithdrawalResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/withdrawals/pending [get]
func (h *WithdrawalHandler) AdminListPending(w http.ResponseWriter, r *http.Request) {
	requests, err := h.svc.ListPending(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]WithdrawalResponse, len(requests))
	for i, req := range requests {
		// Mask sensitive payout_details in list responses: a single paginated
		// payload would otherwise expose every user's raw bank account numbers
		// or PayPal emails.  The processing admin who clicks into a specific
		// withdrawal sees the full details via the approve/reject/process response.
		data[i] = withdrawalToResponseMasked(req)
	}
	writeJSON(w, http.StatusOK, data)
}

type reviewWithdrawalRequest struct {
	Notes string `json:"notes"`
}

// AdminApprove handles POST /admin/withdrawals/{id}/approve.
//
// @Summary      Approve withdrawal
// @Description  Advances a pending withdrawal to approved status. Requires admin role.
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                               true  "Request ID"
// @Param        body  body      handler.reviewWithdrawalRequest   false "Optional notes"
// @Success      200  {object}  handler.WithdrawalResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      409  {object}  handler.ErrorResponse  "Withdrawal is not in pending status"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid withdrawal ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/withdrawals/{id}/approve [post]
func (h *WithdrawalHandler) AdminApprove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidWithdrawalID))
		return
	}
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	var req reviewWithdrawalRequest
	if err := decodeJSONOptional(r, &req); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	result, err := h.svc.ApproveRequest(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if h.notifier != nil {
		h.notifier.NotifyPayoutApproved(r.Context(), result.UserID, result.AmountCents,
			string(result.Method), strconv.Itoa(caller.ID))
	}
	writeJSON(w, http.StatusOK, withdrawalToResponse(result))
}

// AdminReject handles POST /admin/withdrawals/{id}/reject.
//
// @Summary      Reject withdrawal
// @Description  Rejects a pending withdrawal and releases the reserved balance. Requires admin role.
// @Tags         admin-payments
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                               true  "Request ID"
// @Param        body  body      handler.reviewWithdrawalRequest   true  "Rejection notes"
// @Success      200  {object}  handler.WithdrawalResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      409  {object}  handler.ErrorResponse  "Withdrawal is not in pending status"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid withdrawal ID or notes required"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/withdrawals/{id}/reject [post]
func (h *WithdrawalHandler) AdminReject(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidWithdrawalID))
		return
	}
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, moneyJSONBodyLimit)
	req, err := decodeJSON[reviewWithdrawalRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if req.Notes == "" {
		writeError(w, r, h.log, apperrors.Validation("notes are required"))
		return
	}

	result, err := h.svc.RejectRequest(r.Context(), id, caller.ID, req.Notes)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, withdrawalToResponse(result))
}

// AdminProcess handles POST /admin/withdrawals/{id}/process.
//
// @Summary      Process withdrawal
// @Description  Marks an approved withdrawal as processed and commits the balance deduction. Requires admin role.
// @Tags         admin-payments
// @Produce      json
// @Security     BearerAuth
// @Param        id  path      int  true  "Request ID"
// @Success      200  {object}  handler.WithdrawalResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      404  {object}  handler.ErrorResponse
// @Failure      409  {object}  handler.ErrorResponse  "Withdrawal not approved or insufficient reserved balance"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid withdrawal ID"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/withdrawals/{id}/process [post]
func (h *WithdrawalHandler) AdminProcess(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation(msgInvalidWithdrawalID))
		return
	}
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	result, err := h.svc.ProcessWithdrawal(r.Context(), id, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, withdrawalToResponse(result))
}
