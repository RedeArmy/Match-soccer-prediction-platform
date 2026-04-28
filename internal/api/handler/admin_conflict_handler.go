package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
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

// ConflictSummary handles GET /admin/stats/conflicts/summary.
//
// @Summary      Conflict summary
// @Description  Returns an aggregated view of all currently detected conflicts
//
//	grouped by type, with total count and average age per type.
//	Designed for dashboard alert widgets that need a lightweight
//	signal without the full conflict detail list. Requires admin role.
//
// @Tags         admin-conflicts
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.ConflictSummaryResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/stats/conflicts/summary [get]
func (h *AdminConflictHandler) ConflictSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := h.svc.ConflictSummary(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	byType := make([]ConflictTypeSummaryResponse, len(summary.ByType))
	for i, s := range summary.ByType {
		byType[i] = ConflictTypeSummaryResponse{
			Type:       string(s.Type),
			Count:      s.Count,
			AvgAgeDays: s.AvgAgeDays,
		}
	}
	writeJSON(w, http.StatusOK, ConflictSummaryResponse{
		TotalUnresolved: summary.TotalUnresolved,
		ByType:          byType,
	})
}

// ListConflicts handles GET /admin/conflicts — paginated detected conflicts.
//
// @Summary      List operational conflicts
// @Description  Returns currently detected operational inconsistencies that
//
//	require administrative attention. Conflicts are computed on demand
//	and are not persisted — they reflect the live database state.
//	Categories include groups without an owner, stale pending payments,
//	and stale pending memberships. Supports ?page and ?limit. Requires admin role.
//
// @Tags         admin-conflicts
// @Produce      json
// @Security     BearerAuth
// @Param        page   query  int  false  "Page number (default 1)"
// @Param        limit  query  int  false  "Page size (default 50, max 200)"
// @Success      200  {object}  handler.Paged[handler.ConflictResponse]
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/conflicts [get]
func (h *AdminConflictHandler) ListConflicts(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)
	conflicts, err := h.svc.ListConflicts(r.Context(), repository.Pagination{
		Limit:  p.Limit,
		Offset: p.Offset,
	})
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]ConflictResponse, len(conflicts))
	for i, c := range conflicts {
		data[i] = conflictToResponse(c)
	}
	writeJSON(w, http.StatusOK, Paged[ConflictResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}

type resolveConflictRequest struct {
	Action string `json:"action"` // "ack" (default) or "auto_fix"
	Note   string `json:"note"`
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

	action := req.Action
	if action == "" {
		action = "ack"
	}
	if err := h.svc.ResolveConflict(r.Context(), conflictType, entityID, caller.ID, action, req.Note); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
