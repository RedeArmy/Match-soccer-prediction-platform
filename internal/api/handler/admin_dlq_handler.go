package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminDLQHandler handles admin endpoints for dead-letter queue management.
type AdminDLQHandler struct {
	svc service.DLQService
	log *zap.Logger
}

// NewAdminDLQHandler constructs an AdminDLQHandler.
// svc may not be nil; pass service.NoopDLQService{} when DLQ is not supported.
func NewAdminDLQHandler(svc service.DLQService, log *zap.Logger) *AdminDLQHandler {
	return &AdminDLQHandler{svc: svc, log: log}
}

// Stats handles GET /admin/dlq — count, oldest message age, and sample payloads per event type.
func (h *AdminDLQHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.Stats(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type replayRequest struct {
	Limit *int `json:"limit"`
}

// Replay handles POST /admin/dlq/replay — re-enqueue DLQ messages.
func (h *AdminDLQHandler) Replay(w http.ResponseWriter, r *http.Request) {
	var req replayRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // limit is optional

	limit := 10
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}

	replayed, err := h.svc.Replay(r.Context(), limit)
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"replayed": replayed})
}

// Purge handles DELETE /admin/dlq — delete all DLQ entries.
func (h *AdminDLQHandler) Purge(w http.ResponseWriter, r *http.Request) {
	removed, err := h.svc.Purge(r.Context())
	if err != nil {
		middleware.WriteError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"removed": removed})
}
