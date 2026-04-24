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
//
// @Summary      List operational conflicts
// @Description  Returns all currently detected operational inconsistencies that
//
//	require administrative attention. Conflicts are computed on demand
//	and are not persisted — they reflect the live database state.
//	Categories include groups without an owner, unpaid paid members,
//	and orphaned payment records. Requires admin role.
//
// @Tags         admin-conflicts
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.ConflictResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/conflicts [get]
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
//
// @Summary      Resolve a conflict
// @Description  Records an admin acknowledgement of the given conflict. This does
//
//	not fix the underlying data issue — it marks the conflict as
//	reviewed and records an audit log entry. The conflict will
//	reappear in ListConflicts until the root cause is corrected.
//	An optional note can be provided for the audit trail. Requires admin role.
//
// @Tags         admin-conflicts
// @Accept       json
// @Security     BearerAuth
// @Param        type  path  string                        true   "Conflict type (e.g. group_no_owner, orphaned_payment)"
// @Param        id    path  int                           true   "Entity ID affected by the conflict"
// @Param        body  body  handler.resolveConflictRequest false  "Optional resolution note"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Conflict type or entity not found"
// @Failure      422  {object}  handler.ErrorResponse  "Invalid entity ID or conflict type"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/conflicts/{type}/{id}/resolve [post]
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
