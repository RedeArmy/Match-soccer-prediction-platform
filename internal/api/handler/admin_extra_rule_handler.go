package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminExtraRuleHandler handles admin endpoints for match extras (bonus
// predictions) point configuration.
type AdminExtraRuleHandler struct {
	svc service.ExtraRuleService
	log *zap.Logger
}

// NewAdminExtraRuleHandler constructs an AdminExtraRuleHandler.
func NewAdminExtraRuleHandler(svc service.ExtraRuleService, log *zap.Logger) *AdminExtraRuleHandler {
	return &AdminExtraRuleHandler{svc: svc, log: log}
}

// ExtraRuleResponse is the JSON representation of an ExtraRule.
type ExtraRuleResponse struct {
	ID        int    `json:"id"`
	ExtraType string `json:"extra_type"`
	Points    int    `json:"points"`
	IsActive  bool   `json:"is_active"`
	UpdatedAt string `json:"updated_at"`
}

type updateExtraRuleRequest struct {
	Points   int  `json:"points"`
	IsActive bool `json:"is_active"`
}

func extraRuleToResponse(r *domain.ExtraRule) ExtraRuleResponse {
	return ExtraRuleResponse{
		ID:        r.ID,
		ExtraType: string(r.ExtraType),
		Points:    r.Points,
		IsActive:  r.IsActive,
		UpdatedAt: r.UpdatedAt.Format(timeFormat),
	}
}

// List handles GET /admin/extra-rules.
//
// @Summary      List extra rules
// @Description  Returns the point configuration for both match extras
//
//	(first_scorer, halftime_result). Requires admin role.
//
// @Tags         admin-extra-rules
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.ExtraRuleResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/extra-rules [get]
func (h *AdminExtraRuleHandler) List(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]ExtraRuleResponse, len(rules))
	for i, rule := range rules {
		data[i] = extraRuleToResponse(rule)
	}
	writeJSON(w, http.StatusOK, data)
}

// GetByType handles GET /admin/extra-rules/{extraType}.
//
// @Summary      Get extra rule for a type
// @Description  Returns the point configuration for the requested extra type.
// @Tags         admin-extra-rules
// @Produce      json
// @Security     BearerAuth
// @Param        extraType  path      string  true  "Extra type (first_scorer, halftime_result)"
// @Success      200        {object}  handler.ExtraRuleResponse
// @Failure      401        {object}  handler.ErrorResponse
// @Failure      403        {object}  handler.ErrorResponse
// @Failure      404        {object}  handler.ErrorResponse  "Type not found"
// @Failure      500        {object}  handler.ErrorResponse
// @Router       /api/v1/admin/extra-rules/{extraType} [get]
func (h *AdminExtraRuleHandler) GetByType(w http.ResponseWriter, r *http.Request) {
	extraType := domain.ExtraType(chi.URLParam(r, "extraType"))
	rule, err := h.svc.GetByType(r.Context(), extraType)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, extraRuleToResponse(rule))
}

// Update handles PATCH /admin/extra-rules/{extraType}.
//
// @Summary      Update extra rule for a type
// @Description  Sets a new point value and active flag for the requested extra
//
//	type. Changes take effect on the next ScoreExtras call for a match
//	— no restart required. Requires admin role.
//
// @Tags         admin-extra-rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        extraType  path      string                        true  "Extra type"
// @Param        body       body      handler.updateExtraRuleRequest true  "New point value"
// @Success      200        {object}  handler.ExtraRuleResponse
// @Failure      400        {object}  handler.ErrorResponse  "Invalid JSON"
// @Failure      401        {object}  handler.ErrorResponse
// @Failure      403        {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404        {object}  handler.ErrorResponse  "Type not found"
// @Failure      422        {object}  handler.ErrorResponse  "Validation error"
// @Failure      500        {object}  handler.ErrorResponse
// @Router       /api/v1/admin/extra-rules/{extraType} [patch]
func (h *AdminExtraRuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	extraType := domain.ExtraType(chi.URLParam(r, "extraType"))

	var req updateExtraRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid request body"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	updated, err := h.svc.Update(r.Context(), extraType, domain.ExtraRuleInput{
		Points:   req.Points,
		IsActive: req.IsActive,
	}, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, extraRuleToResponse(updated))
}
