package handler

import (
	"encoding/json"
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
func (h *AdminSystemParamHandler) ListAll(w http.ResponseWriter, r *http.Request) {
	params, err := h.svc.GetAll(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	data := make([]SystemParamResponse, len(params))
	for i, p := range params {
		data[i] = systemParamToResponse(p)
	}
	writeJSON(w, http.StatusOK, data)
}

// Get handles GET /admin/system-params/{key}.
func (h *AdminSystemParamHandler) Get(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("param key is required"))
		return
	}

	param, err := h.svc.Get(r.Context(), key)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, systemParamToResponse(param))
}

type setParamRequest struct {
	Value string `json:"value"`
}

// Set handles PATCH /admin/system-params/{key}.
func (h *AdminSystemParamHandler) Set(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	if key == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("param key is required"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req setParamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	param, err := h.svc.Set(r.Context(), key, req.Value, caller.ID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, systemParamToResponse(param))
}

type bulkSetParamRequest struct {
	Params map[string]string `json:"params"`
}

// BulkSet handles POST /admin/system-params/bulk.
func (h *AdminSystemParamHandler) BulkSet(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var req bulkSetParamRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}
	if len(req.Params) == 0 {
		middleware.WriteError(w, r, h.log, apperrors.Validation("params map must not be empty"))
		return
	}

	if err := h.svc.BulkSet(r.Context(), req.Params, caller.ID); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
