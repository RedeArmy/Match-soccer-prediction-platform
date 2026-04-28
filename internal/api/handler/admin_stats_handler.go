package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// AdminStatsHandler handles admin dashboard statistics endpoints.
type AdminStatsHandler struct {
	svc service.AdminReadService
	log *zap.Logger
}

// NewAdminStatsHandler constructs an AdminStatsHandler.
func NewAdminStatsHandler(svc service.AdminReadService, log *zap.Logger) *AdminStatsHandler {
	return &AdminStatsHandler{svc: svc, log: log}
}

// GetDashboardStats handles GET /admin/stats.
//
// @Summary      Dashboard statistics
// @Description  Returns aggregate counts for groups, users, and payment records.
//
//	Intended for admin dashboard home-screen widgets. Requires admin role.
//
// @Tags         admin-stats
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.DashboardStatsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/stats [get]
func (h *AdminStatsHandler) GetDashboardStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.GetDashboardStats(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}
	writeJSON(w, http.StatusOK, dashboardStatsToResponse(stats))
}
