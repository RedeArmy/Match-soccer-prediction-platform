package handler

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const msgParamKeyRequired = "param key is required"

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
		writeError(w, r, h.log, apperrors.Validation(msgParamKeyRequired))
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
		writeError(w, r, h.log, apperrors.Validation(msgParamKeyRequired))
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
	// DryRun, when true, validates all params and returns the projected
	// old→new diffs without persisting any change. Useful for previewing
	// the impact of a bulk update before committing.
	DryRun bool `json:"dry_run"`
	// ConfirmSensitiveChanges must be true when the request modifies any
	// scoring.* or payment.* parameter in a live (non-dry-run) BulkSet.
	// Omitting it on a request that touches sensitive keys returns 422.
	ConfirmSensitiveChanges bool `json:"confirm_sensitive_changes"`
}

// BulkSet handles POST /admin/system-params/bulk.
//
// # Dry-run mode
//
// When dry_run=true the request validates all params and returns a
// BulkPreviewResponse showing the projected old→new diff for each key
// (HTTP 200). No parameters are written. Use this before a live call
// to review the impact, especially for sensitive parameters.
//
// # Sensitive-parameter confirmation
//
// Params in the scoring.* and payment.* categories affect all future match
// scoring and user-facing financial limits respectively. A live BulkSet
// (dry_run=false) that includes any such key returns 422 unless the request
// also sets confirm_sensitive_changes=true. The error message names the
// affected keys so the operator can review before re-submitting.
//
// @Summary      Bulk-update system parameters
// @Description  Updates multiple system parameters in a single atomic operation.
//
//	Each key-value pair is upserted. The params map must not be empty.
//	Set dry_run=true to preview changes. Set confirm_sensitive_changes=true
//	when the update includes scoring.* or payment.* keys. Requires admin role.
//
// @Tags         admin-system-params
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body  handler.bulkSetParamRequest  true  "Params map; optional dry_run and confirm_sensitive_changes flags"
// @Success      200   {object}  handler.BulkPreviewResponse  "dry_run=true: projected diffs (no write)"
// @Success      204   "Live update applied"
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422   {object}  handler.ErrorResponse  "Validation error or sensitive keys without confirmation"
// @Failure      500   {object}  handler.ErrorResponse
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

	if req.DryRun {
		diffs, err := h.svc.BulkPreview(r.Context(), req.Params)
		if err != nil {
			writeError(w, r, h.log, err)
			return
		}
		resp := BulkPreviewResponse{Diffs: make([]ParamDiffResponse, len(diffs))}
		for i, d := range diffs {
			resp.Diffs[i] = paramDiffToResponse(d)
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}

	if !req.ConfirmSensitiveChanges {
		if keys := sensitiveKeysIn(req.Params); len(keys) > 0 {
			writeError(w, r, h.log, apperrors.Validation(
				fmt.Sprintf("sensitive params require confirm_sensitive_changes=true: %s",
					strings.Join(keys, ", ")),
			))
			return
		}
	}

	if err := h.svc.BulkSet(r.Context(), req.Params, caller.ID); err != nil {
		writeError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sensitiveKeysIn returns the sorted list of keys in params that belong to a
// sensitive category (scoring.*, payment.*). Returns nil when none match.
func sensitiveKeysIn(params map[string]string) []string {
	var keys []string
	for k := range params {
		if domain.IsSensitiveParamKey(k) {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Strings(keys)
	return keys
}

// Reset handles POST /admin/system-params/{key}/reset.
//
// @Summary      Reset a system parameter to its migration default
// @Description  Restores the operational value of the given parameter to the
//
//	immutable default_value set by the seeding migration. Useful for
//	reverting operator overrides without re-deploying or re-running
//	migrations. The change takes effect immediately. Requires admin role.
//
// @Tags         admin-system-params
// @Produce      json
// @Security     BearerAuth
// @Param        key  path      string  true  "Parameter key"
// @Success      200  {object}  handler.SystemParamResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404  {object}  handler.ErrorResponse  "Parameter not found"
// @Failure      422  {object}  handler.ErrorResponse  "Key is required"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params/{key}/reset [post]
func (h *AdminSystemParamHandler) Reset(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, r, h.log, apperrors.Validation(msgParamKeyRequired))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	param, err := h.svc.ResetToDefault(r.Context(), key, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, systemParamToResponse(param))
}

// History handles GET /admin/system-params/{key}/history.
//
// @Summary      System parameter mutation history
// @Description  Returns the mutation history for the given parameter key in
//
//	reverse-chronological order (most recent change first). Each entry
//	records the old value, the new value, the actor, and the action type
//	("set" or "reset"). Requires admin role.
//
// @Tags         admin-system-params
// @Produce      json
// @Security     BearerAuth
// @Param        key     path      string  true   "Parameter key"
// @Param        limit   query     int     false  "Page size (default 20, max 100)"
// @Param        cursor  query     string  false  "Opaque pagination cursor from prior response"
// @Success      200  {object}  handler.CursorPaged[handler.SystemParamHistoryResponse]
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      422  {object}  handler.ErrorResponse  "Key is required"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/system-params/{key}/history [get]
func (h *AdminSystemParamHandler) History(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		writeError(w, r, h.log, apperrors.Validation(msgParamKeyRequired))
		return
	}

	entries, next, err := h.svc.GetHistory(r.Context(), key, parseCursorPage(r))
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	data := make([]SystemParamHistoryResponse, len(entries))
	for i, e := range entries {
		data[i] = systemParamHistoryToResponse(e)
	}
	writeJSON(w, http.StatusOK, CursorPaged[SystemParamHistoryResponse]{Data: data, NextCursor: next})
}
