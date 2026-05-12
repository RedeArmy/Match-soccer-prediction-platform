package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// BalanceHandler handles user balance and ledger endpoints.
type BalanceHandler struct {
	svc service.BalanceService
	log *zap.Logger
}

// NewBalanceHandler constructs a BalanceHandler.
func NewBalanceHandler(svc service.BalanceService, log *zap.Logger) *BalanceHandler {
	return &BalanceHandler{svc: svc, log: log}
}

// GetBalance handles GET /api/v1/users/me/balance.
//
// @Summary      Get balance
// @Description  Returns the authenticated user's current balance, reserved amount, and available balance.
// @Tags         balance
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.BalanceResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Router       /api/v1/users/me/balance [get]
func (h *BalanceHandler) GetBalance(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	balanceCents, reservedCents, err := h.svc.GetBalance(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, balanceToResponse(balanceCents, reservedCents))
}

// GetLedger handles GET /api/v1/users/me/balance/ledger.
//
// @Summary      Get balance ledger
// @Description  Returns the authenticated user's balance transaction history, ordered by most recent first.
// @Tags         balance
// @Produce      json
// @Security     BearerAuth
// @Param        limit   query  int  false  "Max records per page (default 50)"
// @Param        offset  query  int  false  "Offset for pagination"
// @Success      200  {array}  handler.LedgerEntryResponse
// @Failure      401  {object} handler.ErrorResponse
// @Router       /api/v1/users/me/balance/ledger [get]
func (h *BalanceHandler) GetLedger(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	p := parsePagination(r)
	if p.Limit <= 0 {
		p = repository.Pagination{Limit: 50}
	}
	entries, err := h.svc.GetLedger(r.Context(), caller.ID, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]LedgerEntryResponse, len(entries))
	for i, e := range entries {
		data[i] = ledgerEntryToResponse(e)
	}
	writeJSON(w, http.StatusOK, data)
}
