package handler

import (
	"net/http"
	"strconv"

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

// List handles GET /admin/audit-log with optional filters and pagination.
//
// @Summary      List audit log entries
// @Description  Returns a paginated list of audit log entries. Supports filtering
//
//	by actor, action string, and affected resource. All admin actions
//	(bans, deletions, ownership transfers, payment reviews) appear in
//	this log. Requires admin role.
//
// @Tags         admin-audit-log
// @Produce      json
// @Security     BearerAuth
// @Param        actor_id       query     int     false  "Filter by actor (admin) user ID"
// @Param        action         query     string  false  "Filter by action string (e.g. admin_user.banned)"
// @Param        resource_type  query     string  false  "Filter by resource type (e.g. membership, payment_record)"
// @Param        resource_id    query     int     false  "Filter by resource ID"
// @Param        limit          query     int     false  "Max records per page (default 50, max 200)"
// @Param        page           query     int     false  "Page number (default 1)"
// @Success      200            {object}  handler.Paged[handler.AuditLogResponse]
// @Failure      401            {object}  handler.ErrorResponse
// @Failure      403            {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500            {object}  handler.ErrorResponse
// @Router       /api/v1/admin/audit-log [get]
func (h *AdminAuditHandler) List(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

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

	entries, err := h.svc.ListAuditLogs(r.Context(), f, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]AuditLogResponse, len(entries))
	for i, e := range entries {
		data[i] = auditLogToResponse(e)
	}
	writeJSON(w, http.StatusOK, Paged[AuditLogResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}

// ListByEntity handles GET /admin/audit-log/entity/{type}/{id}.
//
// @Summary      List audit log by entity
// @Description  Returns paginated audit log entries scoped to a single resource
//
//	(identified by its type and ID). Useful for reviewing the full
//	history of a specific membership, payment, or user. Requires admin role.
//
// @Tags         admin-audit-log
// @Produce      json
// @Security     BearerAuth
// @Param        type   path      string  true   "Resource type (e.g. membership, payment_record, user)"
// @Param        id     path      int     true   "Resource ID"
// @Param        limit  query     int     false  "Max records per page (default 50, max 200)"
// @Param        page   query     int     false  "Page number (default 1)"
// @Success      200    {object}  handler.Paged[handler.AuditLogResponse]
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422    {object}  handler.ErrorResponse  "Invalid resource ID"
// @Failure      500    {object}  handler.ErrorResponse
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

	p := parsePagination(r)

	entries, err := h.svc.ListAuditLogsByEntity(r.Context(), resourceType, resourceID, p)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]AuditLogResponse, len(entries))
	for i, e := range entries {
		data[i] = auditLogToResponse(e)
	}
	writeJSON(w, http.StatusOK, Paged[AuditLogResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}
