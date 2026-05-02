package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// UserStatsHandler handles HTTP requests for user statistics endpoints.
type UserStatsHandler struct {
	svc service.MyStatsGetter
	log *zap.Logger
}

// NewUserStatsHandler constructs a UserStatsHandler.
func NewUserStatsHandler(svc service.MyStatsGetter, log *zap.Logger) *UserStatsHandler {
	return &UserStatsHandler{svc: svc, log: log}
}

// GetMyStats handles GET /api/v1/users/me/stats.
//
// Returns the authenticated user's prediction performance profile: prediction
// volume, accuracy rate, points by tournament phase, current streak, and
// longest streak. Requires a resolved user in context (ResolveUser middleware).
//
// @Summary      My prediction statistics
// @Description  Returns aggregated prediction stats for the authenticated user:
//
//	total/scored/correct/exact predictions, points by phase, accuracy %,
//	current streak, and longest streak.
//
// @Tags         users
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.UserStatsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/users/me/stats [get]
func (h *UserStatsHandler) GetMyStats(w http.ResponseWriter, r *http.Request) {
	caller, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, r, h.log, apperrors.Unauthorised(msgAuthRequired))
		return
	}

	stats, err := h.svc.GetMyStats(r.Context(), caller.ID)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	writeJSON(w, http.StatusOK, userStatsToResponse(stats))
}
