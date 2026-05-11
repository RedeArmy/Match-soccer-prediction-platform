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
// PredictedWinMethod is optional; when provided for a knockout match it must be
// one of: "normal", "extra_time", "penalties".
type submitPredictionRequest struct {
	MatchID            int     `json:"match_id"`
	HomeScore          int     `json:"home_score"`
	AwayScore          int     `json:"away_score"`
	PredictedWinMethod *string `json:"predicted_win_method"`
}

// updatePredictionRequest is the JSON body accepted by PATCH /api/v1/predictions/{id}.
type updatePredictionRequest struct {
	HomeScore          int     `json:"home_score"`
	AwayScore          int     `json:"away_score"`
	PredictedWinMethod *string `json:"predicted_win_method"`
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
// @Success      201   {object}  handler.PredictionResponse  "Created (first submission)"
// @Success      200   {object}  handler.PredictionResponse  "OK (idempotent replay — prediction unchanged)"
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      422   {object}  handler.ErrorResponse
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/predictions [post]
func (h *PredictionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	req, err := decodeJSON[submitPredictionRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	var predictedWM *domain.WinMethod
	if req.PredictedWinMethod != nil {
		wm, err := domain.ParseWinMethod(*req.PredictedWinMethod)
		if err != nil {
			writeError(w, r, h.log, err)
			return
		}
		predictedWM = &wm
	}
	prediction := &domain.Prediction{
		UserID:             caller.ID,
		MatchID:            req.MatchID,
		HomeScore:          req.HomeScore,
		AwayScore:          req.AwayScore,
		PredictedWinMethod: predictedWM,
	}
	created, err := h.svc.Submit(r.Context(), prediction)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	status := http.StatusCreated
	if !created {
		status = http.StatusOK
	}
	writeJSON(w, status, predToResponse(prediction))
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
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}
	id, err := pathID(r, "id")
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	req, err := decodeJSON[updatePredictionRequest](r)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}
	var predictedWM *domain.WinMethod
	if req.PredictedWinMethod != nil {
		wm, err := domain.ParseWinMethod(*req.PredictedWinMethod)
		if err != nil {
			writeError(w, r, h.log, err)
			return
		}
		predictedWM = &wm
	}
	prediction, err := h.svc.Update(r.Context(), caller.ID, id, req.HomeScore, req.AwayScore, predictedWM)
	if err != nil {
		writeError(w, r, h.log, err)
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
//	Supports optional pagination via ?limit and ?offset (defaults to unbounded).
//
// @Tags         predictions
// @Produce      json
// @Security     BearerAuth
// @Param        quiniela_id  query     int  false  "Scope results to this quiniela's active members"
// @Param        limit        query     int  false  "Maximum number of results to return (0 = unbounded)"
// @Param        offset       query     int  false  "Number of results to skip (default: 0)"
// @Success      200          {object}  handler.Paged[handler.PredictionResponse]
// @Failure      401          {object}  handler.ErrorResponse  "Missing or invalid auth token"
// @Failure      422          {object}  handler.ErrorResponse  "Invalid quiniela_id"
// @Failure      500          {object}  handler.ErrorResponse
// @Router       /api/v1/predictions/me [get]
func (h *PredictionHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	limit, offset := parsePaginationParams(r)

	var (
		predictions []*domain.Prediction
		err         error
	)
	if quinielaIDStr := r.URL.Query().Get("quiniela_id"); quinielaIDStr != "" {
		quinielaID, qErr := parseIntParam(quinielaIDStr)
		if qErr != nil {
			writeError(w, r, h.log, apperrors.Validation("quiniela_id must be a positive integer"))
			return
		}
		predictions, err = h.svc.GetByUserAndQuiniela(r.Context(), caller.ID, quinielaID)
	} else {
		predictions, err = h.svc.GetByUser(r.Context(), caller.ID)
	}
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	// Apply in-memory pagination (service layer does not support pagination yet).
	pagedPredictions := applySlicePagination(predictions, limit, offset)

	out := make([]PredictionResponse, len(pagedPredictions))
	for i, p := range pagedPredictions {
		out[i] = predToResponse(p)
	}

	writeJSON(w, http.StatusOK, Paged[PredictionResponse]{
		Data: out,
		Page: PageMeta{Limit: limit, Offset: offset},
	})
}
