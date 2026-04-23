package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminConflictHandler handles admin endpoints for operational conflict management.
type AdminConflictHandler struct {
	svc service.ConflictService
	log *zap.Logger
}

// NewAdminConflictHandler constructs an AdminConflictHandler.
func NewAdminConflictHandler(svc service.ConflictService, log *zap.Logger) *AdminConflictHandler {
	return &AdminConflictHandler{svc: svc, log: log}
}

// ListConflicts handles GET /admin/conflicts — all currently detected conflicts.
func (h *AdminConflictHandler) ListConflicts(w http.ResponseWriter, r *http.Request) {
	conflicts, err := h.svc.ListConflicts(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]ConflictResponse, len(conflicts))
	for i, c := range conflicts {
		data[i] = conflictToResponse(c)
	}
	writeJSON(w, http.StatusOK, data)
}

type resolveConflictRequest struct {
	Note string `json:"note"`
}

// ResolveConflict handles POST /admin/conflicts/{type}/{id}/resolve.
func (h *AdminConflictHandler) ResolveConflict(w http.ResponseWriter, r *http.Request) {
	conflictType := chi.URLParam(r, "type")
	if conflictType == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("conflict type is required"))
		return
	}

	entityID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || entityID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid entity id"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req resolveConflictRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // note is optional

	if err := h.svc.ResolveConflict(r.Context(), conflictType, entityID, caller.ID, req.Note); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
