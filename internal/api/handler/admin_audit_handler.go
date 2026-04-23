package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
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
// Query params: actor_id, action, resource_type, resource_id, page, limit.
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
		middleware.WriteError(w, r, h.log, err)
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
func (h *AdminAuditHandler) ListByEntity(w http.ResponseWriter, r *http.Request) {
	resourceType := chi.URLParam(r, "type")
	if resourceType == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("resource type is required"))
		return
	}

	resourceID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil || resourceID <= 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("invalid resource id"))
		return
	}

	p := parsePagination(r)

	entries, err := h.svc.ListAuditLogsByEntity(r.Context(), resourceType, resourceID, p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
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
