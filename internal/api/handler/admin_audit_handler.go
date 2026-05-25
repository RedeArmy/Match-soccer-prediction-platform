package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminAuditHandler handles admin endpoints for the audit log.
type AdminAuditHandler struct {
	svc service.AuditReader
	log *zap.Logger
}

// NewAdminAuditHandler constructs an AdminAuditHandler.
func NewAdminAuditHandler(svc service.AuditReader, log *zap.Logger) *AdminAuditHandler {
	return &AdminAuditHandler{svc: svc, log: log}
}

// List handles GET /admin/audit-log with optional filters and cursor pagination.
//
// @Summary      List audit log entries
// @Description  Returns a cursor-paginated list of audit log entries. Supports
//
//	filtering by actor, action string, and affected resource. Pass the
//	next_cursor value from the previous response as ?cursor= to fetch
//	the next page. All admin actions (bans, deletions, ownership
//	transfers, payment reviews) appear in this log. Requires admin role.
//
// @Tags         admin-audit-log
// @Produce      json
// @Security     BearerAuth
// @Param        actor_id       query     int     false  "Filter by actor (admin) user ID"
// @Param        action         query     string  false  "Filter by action string (e.g. admin_user.banned)"
// @Param        resource_type  query     string  false  "Filter by resource type (e.g. membership, payment_record)"
// @Param        resource_id    query     int     false  "Filter by resource ID"
// @Param        created_after  query     string  false  "Return entries created at or after this RFC3339 timestamp"
// @Param        created_before query     string  false  "Return entries created at or before this RFC3339 timestamp"
// @Param        limit          query     int     false  "Max records per page (default 50, max 200)"
// @Param        cursor         query     string  false  "Opaque cursor from previous response next_cursor field"
// @Success      200            {object}  handler.CursorPaged[handler.AuditLogResponse]
// @Failure      401            {object}  handler.ErrorResponse
// @Failure      403            {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422            {object}  handler.ErrorResponse  "Invalid cursor token"
// @Failure      500            {object}  handler.ErrorResponse
// @Router       /api/v1/admin/audit-log [get]
func (h *AdminAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parseCursorPage(r)

	f := repository.AuditLogFilters{
		ActorID:    parseOptionalInt(r, "actor_id"),
		ResourceID: parseOptionalInt(r, "resource_id"),
	}
	if q := r.URL.Query().Get("action"); q != "" {
		f.Action = &q
	}
	if q := r.URL.Query().Get("resource_type"); q != "" {
		f.ResourceType = &q
	}
	if q := r.URL.Query().Get("created_after"); q != "" {
		t, err := time.Parse(time.RFC3339, q)
		if err != nil {
			writeError(w, r, h.log, apperrors.Validation("created_after must be RFC3339"))
			return
		}
		f.CreatedAfter = &t
	}
	if q := r.URL.Query().Get("created_before"); q != "" {
		t, err := time.Parse(time.RFC3339, q)
		if err != nil {
			writeError(w, r, h.log, apperrors.Validation("created_before must be RFC3339"))
			return
		}
		f.CreatedBefore = &t
	}

	entries, nextCursor, err := h.svc.ListAuditLogs(r.Context(), f, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]AuditLogResponse, len(entries))
	for i, e := range entries {
		data[i] = auditLogToResponse(e)
	}
	writeJSON(w, http.StatusOK, CursorPaged[AuditLogResponse]{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}

// ListByEntity handles GET /admin/audit-log/entity/{type}/{id}.
//
// @Summary      List audit log by entity
// @Description  Returns cursor-paginated audit log entries scoped to a single
//
//	resource (identified by its type and ID). Pass the next_cursor value
//	from the previous response as ?cursor= to fetch the next page.
//	Requires admin role.
//
// @Tags         admin-audit-log
// @Produce      json
// @Security     BearerAuth
// @Param        type    path      string  true   "Resource type (e.g. membership, payment_record, user)"
// @Param        id      path      int     true   "Resource ID"
// @Param        limit   query     int     false  "Max records per page (default 50, max 200)"
// @Param        cursor  query     string  false  "Opaque cursor from previous response next_cursor field"
// @Success      200     {object}  handler.CursorPaged[handler.AuditLogResponse]
// @Failure      401     {object}  handler.ErrorResponse
// @Failure      403     {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422     {object}  handler.ErrorResponse  "Invalid resource ID or cursor token"
// @Failure      500     {object}  handler.ErrorResponse
// @Router       /api/v1/admin/audit-log/entity/{type}/{id} [get]
func (h *AdminAuditHandler) ListByEntity(w http.ResponseWriter, r *http.Request) {
	resourceType := chi.URLParam(r, "type")
	if resourceType == "" {
		writeError(w, r, h.log, apperrors.Validation("resource type is required"))
		return
	}

	resourceID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || resourceID <= 0 {
		writeError(w, r, h.log, apperrors.Validation("invalid resource id"))
		return
	}

	p := parseCursorPage(r)

	entries, nextCursor, err := h.svc.ListAuditLogsByEntity(r.Context(), resourceType, resourceID, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]AuditLogResponse, len(entries))
	for i, e := range entries {
		data[i] = auditLogToResponse(e)
	}
	writeJSON(w, http.StatusOK, CursorPaged[AuditLogResponse]{
		Data:       data,
		NextCursor: nextCursor,
		HasMore:    nextCursor != "",
	})
}
