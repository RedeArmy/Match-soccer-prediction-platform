package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// PredictionHandler handles HTTP requests for the /api/v1/predictions resource.
type PredictionHandler struct {
	svc service.PredictionService
	log *zap.Logger
}

// NewPredictionHandler constructs a PredictionHandler.
func NewPredictionHandler(svc service.PredictionService, log *zap.Logger) *PredictionHandler {
	return &PredictionHandler{svc: svc, log: log}
}

// submitPredictionRequest is the JSON body accepted by POST /api/v1/predictions.
type submitPredictionRequest struct {
	MatchID   int `json:"match_id"`
	HomeScore int `json:"home_score"`
	AwayScore int `json:"away_score"`
}

// updatePredictionRequest is the JSON body accepted by PATCH /api/v1/predictions/{id}.
type updatePredictionRequest struct {
	HomeScore int `json:"home_score"`
	AwayScore int `json:"away_score"`
}

// Submit handles POST /api/v1/predictions.
//
// @Summary      Submit a prediction
// @Description  Submits a score forecast for a scheduled match. The user identity
//
//	is taken from the Bearer token; predictions cannot be submitted on behalf
//	of other users.
//
// @Tags         predictions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.submitPredictionRequest  true  "Prediction"
// @Success      201   {object}  handler.PredictionResponse
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      409   {object}  handler.ErrorResponse  "Prediction already exists for this match"
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/predictions [post]
func (h *PredictionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[submitPredictionRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	prediction := &domain.Prediction{
		UserID:    caller.ID,
		MatchID:   req.MatchID,
		HomeScore: req.HomeScore,
		AwayScore: req.AwayScore,
	}
	if err := h.svc.Submit(r.Context(), prediction); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, predToResponse(prediction))
}

// Update handles PATCH /api/v1/predictions/{id}.
//
// @Summary      Update a prediction
// @Description  Updates the score forecast for an existing prediction. Only
//
//	permitted while the match is still scheduled.
//
// @Tags         predictions
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        id    path      int                              true  "Prediction ID"
// @Param        body  body      handler.updatePredictionRequest  true  "Updated scores"
// @Success      200   {object}  handler.PredictionResponse
// @Failure      404   {object}  handler.ErrorResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/predictions/{id} [patch]
func (h *PredictionHandler) Update(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	req, err := decodeJSON[updatePredictionRequest](r)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	prediction, err := h.svc.Update(r.Context(), caller.ID, id, req.HomeScore, req.AwayScore)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, predToResponse(prediction))
}

// GetMine handles GET /api/v1/predictions/me[?quiniela_id={id}].
//
// @Summary      List my predictions
// @Description  Returns all predictions submitted by the authenticated caller.
//
//	When quiniela_id is provided, results are scoped to predictions
//	belonging to active members of that quiniela. A caller who is not
//	an active member of the quiniela receives an empty array (not a 403),
//	consistent with the empty-collection-on-no-results contract.
//
// @Tags         predictions
// @Produce      json
// @Security     BearerAuth
// @Param        quiniela_id  query     int  false  "Scope results to this quiniela's active members"
// @Success      200          {array}   handler.PredictionResponse
// @Failure      401          {object}  handler.ErrorResponse  "Missing or invalid auth token"
// @Failure      422          {object}  handler.ErrorResponse  "Invalid quiniela_id"
// @Failure      500          {object}  handler.ErrorResponse
// @Router       /api/v1/predictions/me [get]
func (h *PredictionHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	var (
		predictions []*domain.Prediction
		err         error
	)
	if quinielaIDStr := r.URL.Query().Get("quiniela_id"); quinielaIDStr != "" {
		quinielaID, qErr := parseIntParam(quinielaIDStr)
		if qErr != nil {
			middleware.WriteError(w, r, h.log, apperrors.Validation("quiniela_id must be a positive integer"))
			return
		}
		predictions, err = h.svc.GetByUserAndQuiniela(r.Context(), caller.ID, quinielaID)
	} else {
		predictions, err = h.svc.GetByUser(r.Context(), caller.ID)
	}
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	out := make([]PredictionResponse, len(predictions))
	for i, p := range predictions {
		out[i] = predToResponse(p)
	}
	writeJSON(w, http.StatusOK, out)
}
