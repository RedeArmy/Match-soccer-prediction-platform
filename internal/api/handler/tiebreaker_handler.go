package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// TiebreakerHandler handles HTTP requests for the global tiebreaker resource.
// Admin operations (SetQuestion, ConfirmResult) are mounted at /api/v1/tiebreaker
// and guarded by RequireRole(admin) middleware. Member operations (Submit, GetMine)
// are mounted under /api/v1/groups/{id}/tiebreaker.
type TiebreakerHandler struct {
	svc service.TiebreakerService
	log *zap.Logger
}

// NewTiebreakerHandler constructs a TiebreakerHandler.
func NewTiebreakerHandler(svc service.TiebreakerService, log *zap.Logger) *TiebreakerHandler {
	return &TiebreakerHandler{svc: svc, log: log}
}

type setQuestionRequest struct {
	Question string `json:"question"`
}

type submitTiebreakerRequest struct {
	Prediction int `json:"prediction"`
}

type confirmResultRequest struct {
	Result int `json:"result"`
}

// SetQuestion handles PATCH /api/v1/tiebreaker/question.
// Only the system administrator may call this (enforced by RequireRole middleware).
func (h *TiebreakerHandler) SetQuestion(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[setQuestionRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	cfg, err := h.svc.SetQuestion(r.Context(), req.Question)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, tiebreakerConfigToResponse(cfg))
}

// Submit handles POST /api/v1/groups/{id}/tiebreaker.
// Active members submit or update their numeric prediction.
func (h *TiebreakerHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	req, err := decodeJSON[submitTiebreakerRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	tb, err := h.svc.Submit(r.Context(), id, caller.ID, req.Prediction)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, tiebreakerToResponse(tb))
}

// GetMine handles GET /api/v1/groups/{id}/tiebreaker.
// Returns the global tiebreaker question and the caller's own prediction.
func (h *TiebreakerHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	view, err := h.svc.GetMine(r.Context(), id, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, tiebreakerViewToResponse(view))
}

// ConfirmResult handles PATCH /api/v1/tiebreaker/result.
// Only the system administrator may call this (enforced by RequireRole middleware).
func (h *TiebreakerHandler) ConfirmResult(w http.ResponseWriter, r *http.Request) {
	_, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[confirmResultRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	if err := h.svc.ConfirmResult(r.Context(), req.Result); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
