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

// AdminScoringRuleHandler handles admin endpoints for per-phase scoring
// configuration.
type AdminScoringRuleHandler struct {
	svc service.ScoringRuleService
	log *zap.Logger
}

// NewAdminScoringRuleHandler constructs an AdminScoringRuleHandler.
func NewAdminScoringRuleHandler(svc service.ScoringRuleService, log *zap.Logger) *AdminScoringRuleHandler {
	return &AdminScoringRuleHandler{svc: svc, log: log}
}

// ScoringRuleResponse is the JSON representation of a ScoringRule.
type ScoringRuleResponse struct {
	ID             int    `json:"id"`
	Phase          string `json:"phase"`
	ExactScore     int    `json:"exact_score"`
	CorrectOutcome int    `json:"correct_outcome"`
	GoalDifference int    `json:"goal_difference"`
	ExtraTimeBonus int    `json:"extra_time_bonus"`
	PenaltiesBonus int    `json:"penalties_bonus"`
	IsActive       bool   `json:"is_active"`
	UpdatedAt      string `json:"updated_at"`
}

type updateScoringRuleRequest struct {
	ExactScore     int  `json:"exact_score"`
	CorrectOutcome int  `json:"correct_outcome"`
	GoalDifference int  `json:"goal_difference"`
	ExtraTimeBonus int  `json:"extra_time_bonus"`
	PenaltiesBonus int  `json:"penalties_bonus"`
	IsActive       bool `json:"is_active"`
}

func scoringRuleToResponse(r *domain.ScoringRule) ScoringRuleResponse {
	return ScoringRuleResponse{
		ID:             r.ID,
		Phase:          string(r.Phase),
		ExactScore:     r.ExactScore,
		CorrectOutcome: r.CorrectOutcome,
		GoalDifference: r.GoalDifference,
		ExtraTimeBonus: r.ExtraTimeBonus,
		PenaltiesBonus: r.PenaltiesBonus,
		IsActive:       r.IsActive,
		UpdatedAt:      r.UpdatedAt.Format(timeFormat),
	}
}

// List handles GET /admin/scoring-rules.
//
// @Summary      List scoring rules
// @Description  Returns the point configuration for all seven tournament phases,
//
//	ordered by progression (group_stage → final). Knockout phases
//	carry higher point values to reward correct predictions on
//	high-stakes fixtures. Requires admin role.
//
// @Tags         admin-scoring-rules
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   handler.ScoringRuleResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/scoring-rules [get]
func (h *AdminScoringRuleHandler) List(w http.ResponseWriter, r *http.Request) {
	rules, err := h.svc.List(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	data := make([]ScoringRuleResponse, len(rules))
	for i, rule := range rules {
		data[i] = scoringRuleToResponse(rule)
	}
	writeJSON(w, http.StatusOK, data)
}

// GetByPhase handles GET /admin/scoring-rules/{phase}.
//
// @Summary      Get scoring rule for a phase
// @Description  Returns the point configuration for the requested tournament phase.
// @Tags         admin-scoring-rules
// @Produce      json
// @Security     BearerAuth
// @Param        phase  path      string  true  "Tournament phase (e.g. group_stage, round_of_16, final)"
// @Success      200    {object}  handler.ScoringRuleResponse
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse
// @Failure      404    {object}  handler.ErrorResponse  "Phase not found"
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/admin/scoring-rules/{phase} [get]
func (h *AdminScoringRuleHandler) GetByPhase(w http.ResponseWriter, r *http.Request) {
	phase := domain.MatchPhase(chi.URLParam(r, "phase"))
	rule, err := h.svc.GetByPhase(r.Context(), phase)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, scoringRuleToResponse(rule))
}

// Update handles PATCH /admin/scoring-rules/{phase}.
//
// @Summary      Update scoring rule for a phase
// @Description  Sets new point values for the requested tournament phase. Changes
//
//	take effect on the next ScoreMatch call for a match of that phase —
//	no restart required. All three point values must be non-negative, and
//	exact_score must exceed correct_outcome to preserve the scoring
//	hierarchy. Requires admin role.
//
// @Tags         admin-scoring-rules
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        phase  path      string                           true  "Tournament phase"
// @Param        body   body      handler.updateScoringRuleRequest true  "New point values"
// @Success      200    {object}  handler.ScoringRuleResponse
// @Failure      400    {object}  handler.ErrorResponse  "Invalid JSON"
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      404    {object}  handler.ErrorResponse  "Phase not found"
// @Failure      422    {object}  handler.ErrorResponse  "Validation error"
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/admin/scoring-rules/{phase} [patch]
func (h *AdminScoringRuleHandler) Update(w http.ResponseWriter, r *http.Request) {
	phase := domain.MatchPhase(chi.URLParam(r, "phase"))

	var req updateScoringRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, r, h.log, apperrors.Validation("invalid request body"))
		return
	}

	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	updated, err := h.svc.Update(r.Context(), phase, service.ScoringRuleInput{
		ExactScore:     req.ExactScore,
		CorrectOutcome: req.CorrectOutcome,
		GoalDifference: req.GoalDifference,
		ExtraTimeBonus: req.ExtraTimeBonus,
		PenaltiesBonus: req.PenaltiesBonus,
		IsActive:       req.IsActive,
	}, caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, scoringRuleToResponse(updated))
}
