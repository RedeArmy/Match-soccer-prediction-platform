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
