package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// AdminTiebreakerHandler handles admin endpoints for tiebreaker data.
type AdminTiebreakerHandler struct {
	svc service.AdminReadService
	log *zap.Logger
}

// NewAdminTiebreakerHandler constructs an AdminTiebreakerHandler.
func NewAdminTiebreakerHandler(svc service.AdminReadService, log *zap.Logger) *AdminTiebreakerHandler {
	return &AdminTiebreakerHandler{svc: svc, log: log}
}

// ListSubmissions handles GET /admin/tiebreaker/submissions — all tiebreaker predictions.
//
// @Summary      List tiebreaker submissions
// @Description  Returns all tiebreaker numeric predictions from every user, with
//
//	the submitter's display name resolved. Used to review submissions
//	before confirming the official result. Requires admin role.
//
// @Tags         admin-tiebreaker
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query     int  false  "Max records per page (default 50, max 200)"
// @Param        page   query     int  false  "Page number (default 1)"
// @Success      200    {object}  handler.Paged[handler.TiebreakerSubmissionResponse]
// @Failure      401    {object}  handler.ErrorResponse
// @Failure      403    {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500    {object}  handler.ErrorResponse
// @Router       /api/v1/admin/tiebreaker/submissions [get]
func (h *AdminTiebreakerHandler) ListSubmissions(w http.ResponseWriter, r *http.Request) {
	p := parsePagination(r)

	views, err := h.svc.ListTiebreakerSubmissions(r.Context(), p)
	if err != nil {
		middleware.WriteError(w, r, h.log, err)
		return
	}

	data := make([]TiebreakerSubmissionResponse, len(views))
	for i, v := range views {
		data[i] = TiebreakerSubmissionResponse{
			ID:         v.Submission.ID,
			UserID:     v.Submission.UserID,
			UserName:   v.UserName,
			Prediction: v.Submission.Prediction,
			CreatedAt:  v.Submission.CreatedAt.Format(timeFormat),
			UpdatedAt:  v.Submission.UpdatedAt.Format(timeFormat),
		}
	}
	writeJSON(w, http.StatusOK, Paged[TiebreakerSubmissionResponse]{
		Data: data,
		Page: PageMeta{Limit: p.Limit, Offset: p.Offset},
	})
}
