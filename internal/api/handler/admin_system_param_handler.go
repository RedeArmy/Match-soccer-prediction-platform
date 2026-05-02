package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminSystemParamHandler handles admin endpoints for system parameter management.
type AdminSystemParamHandler struct {
	svc service.SystemParamService
	log *zap.Logger
}

// NewAdminSystemParamHandler constructs an AdminSystemParamHandler.
func NewAdminSystemParamHandler(svc service.SystemParamService, log *zap.Logger) *AdminSystemParamHandler {
	return &AdminSystemParamHandler{svc: svc, log: log}
}

// ListAll handles GET /admin/system-params.
//
// @Summary      List system parameters
// @Description  Returns all runtime-configurable system parameters. Parameters
//
//	control scoring rules, group thresholds, and prediction deadlines.
//	Changes take effect immediately without a server restart. Requires admin role.
//
// @Tags         admin-system-params
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.SystemParamResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params [get]
func (h *AdminSystemParamHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	params, err := h.svc.GetAll(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]SystemParamResponse, len(params))
	for i, p := range params {
		data[i] = systemParamToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}

// Get handles GET /admin/system-params/{key}.
//
// @Summary      Get a system parameter
// @Description  Returns a single runtime-configurable system parameter by its key.
//
//	Requires admin role.
//
// @Tags         admin-system-params
// @Produce      json
// @Security     BearerAuth
// @Param        key  path      string  true  "Parameter key (e.g. scoring.exact_score)"
// @Success      200  {object}  handler.SystemParamResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Parameter not found"
// @Failure      422  {object}  handler.ErrorResponse  "Key is required"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params/{key} [get]
func (h *AdminSystemParamHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, r, h.log, apperrors.Validation("param key is required"))
		return
	}

	param, err := h.svc.Get(r.Context(), key)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, systemParamToResponse(param))
}

type setParamRequest struct {
	Value string `json:"value"`
}

// Set handles PATCH /admin/system-params/{key}.
//
// @Summary      Update a system parameter
// @Description  Sets a new value for the given runtime-configurable parameter.
//
//	The change takes effect immediately (in-memory cache is invalidated).
//	The new value is type-validated against the parameter's declared type
//	before being persisted. Requires admin role.
//
// @Tags         admin-system-params
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        key   path      string                    true  "Parameter key"
// @Param        body  body      handler.setParamRequest   true  "New value"
// @Success      200   {object}  handler.SystemParamResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404   {object}  handler.ErrorResponse  "Parameter not found"
// @Failure      422   {object}  handler.ErrorResponse  "Value fails type validation"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params/{key} [patch]
func (h *AdminSystemParamHandler) Set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, r, h.log, apperrors.Validation("param key is required"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[setParamRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	param, err := h.svc.Set(r.Context(), key, req.Value, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, systemParamToResponse(param))
}

type bulkSetParamRequest struct {
	Params map[string]string `json:"params"`
}

// BulkSet handles POST /admin/system-params/bulk.
//
// @Summary      Bulk-update system parameters
// @Description  Updates multiple system parameters in a single atomic operation.
//
//	Each key-value pair is upserted. The params map must not be empty.
//	Requires admin role.
//
// @Tags         admin-system-params
// @Accept       json
// @Security     BearerAuth
// @Param        body  body  handler.bulkSetParamRequest  true  "Map of key -> value pairs"
// @Success      204
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "params map is empty"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params/bulk [post]
func (h *AdminSystemParamHandler) BulkSet(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[bulkSetParamRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	if len(req.Params) == 0 {
		writeError(w, r, h.log, apperrors.Validation("params map must not be empty"))
		return
	}

	if err := h.svc.BulkSet(r.Context(), req.Params, caller.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
