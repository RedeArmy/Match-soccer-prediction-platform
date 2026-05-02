package handler

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

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

// Stats handles GET /admin/dlq - count, oldest message age, and sample payloads per event type.
//
// @Summary      Dead-letter queue stats
// @Description  Returns the count, oldest entry age, and a sample of messages for
//
//	each known event type in the dead-letter queue. Non-zero counts
//	indicate scoring failures that require manual replay. This endpoint
//	returns empty stats (not an error) when the Redis DLQ is not
//	configured. Requires admin role.
//
// @Tags         admin-dlq
// @Produce      json
// @Security     BearerAuth
// @Success      200  {array}   service.DLQStat
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/dlq [get]
func (h *AdminDLQHandler) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.svc.Stats(r.Context())
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

type replayRequest struct {
	Limit *int `json:"limit"`
}

// Replay handles POST /admin/dlq/replay - re-enqueue DLQ messages.
//
// @Summary      Replay dead-letter queue
// @Description  Re-enqueues up to limit entries from all DLQ keys back onto their
//
//	original event streams. Default limit is 10. Use this to recover
//	from transient scoring failures after the underlying issue has been
//	resolved. Requires admin role.
//
// @Tags         admin-dlq
// @Accept       json
// @Produce      json
// @Security     BearerAuth
// @Param        body  body      handler.replayRequest  false  "Optional replay limit (default 10)"
// @Success      200   {object}  object{replayed=int}
// @Failure      401   {object}  handler.ErrorResponse
// @Failure      403   {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500   {object}  handler.ErrorResponse
// @Router       /api/v1/admin/dlq/replay [post]
func (h *AdminDLQHandler) Replay(w http.ResponseWriter, r *http.Request) {
	var req replayRequest
	_ = json.NewDecoder(r.Body).Decode(&req) // limit is optional

	limit := 10
	if req.Limit != nil && *req.Limit > 0 {
		limit = *req.Limit
	}

	replayed, err := h.svc.Replay(r.Context(), limit)
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"replayed": replayed})
}

// Purge handles DELETE /admin/dlq - delete all DLQ entries.
//
// @Summary      Purge dead-letter queue
// @Description  Permanently deletes all entries from all DLQ keys. Use with
//
//	caution: purged events cannot be replayed. Prefer Replay when
//	events can be re-processed. Requires admin role.
//
// @Tags         admin-dlq
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  object{removed=int}
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse  "Caller is not an admin"
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/dlq [delete]
func (h *AdminDLQHandler) Purge(w http.ResponseWriter, r *http.Request) {
	removed, err := h.svc.Purge(r.Context())
	if err != nil {
		writeError(w, r, h.log, apperrors.Internal(err))
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"removed": removed})
}
