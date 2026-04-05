package handler

import (
	"encoding/json"
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
// The authenticated user ID is extracted from the request context (set by
// RequireAuth middleware) so that users can only submit predictions for
// themselves.
func (h *PredictionHandler) Submit(w http.ResponseWriter, r *http.Request) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised("authentication required"))
		return
	}

	var req submitPredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}

	// UserID from Clerk is a string; for now use a numeric lookup via a helper.
	// Until the user-sync layer is implemented, parse the Clerk subject as the
	// internal user ID for simplicity.
	internalUserID, err := clerkSubjectToUserID(userID)
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Unauthorised("invalid user identity"))
		return
	}

	prediction := &domain.Prediction{
		UserID:    internalUserID,
		MatchID:   req.MatchID,
		HomeScore: req.HomeScore,
		AwayScore: req.AwayScore,
	}
	if err := h.svc.Submit(r.Context(), prediction); err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusCreated, prediction)
}

// Update handles PATCH /api/v1/predictions/{id}.
func (h *PredictionHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	var req updatePredictionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		middleware.WriteError(w, r, h.log, decodeError(err))
		return
	}
	prediction, err := h.svc.Update(r.Context(), id, req.HomeScore, req.AwayScore)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, prediction)
}

// ListByUser handles GET /api/v1/predictions?user_id={id}.
func (h *PredictionHandler) ListByUser(w http.ResponseWriter, r *http.Request) {
	userIDStr := r.URL.Query().Get("user_id")
	if userIDStr == "" {
		middleware.WriteError(w, r, h.log, apperrors.Validation("user_id query parameter is required"))
		return
	}
	userID, err := parseIntParam(userIDStr)
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Validation("user_id must be a positive integer"))
		return
	}
	predictions, err := h.svc.GetByUser(r.Context(), userID)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, predictions)
}
