package handler

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// AdminNotificationDLQHandler exposes read and resolve operations on the
// notification_dlq table.  The DLQWorker handles automatic replay; these
// endpoints give operators visibility and a manual escape hatch for entries
// that exhaust retry attempts.
type AdminNotificationDLQHandler struct {
	repo repository.NotificationDLQRepository
	log  *zap.Logger
}

// NewAdminNotificationDLQHandler constructs the handler.
func NewAdminNotificationDLQHandler(repo repository.NotificationDLQRepository, log *zap.Logger) *AdminNotificationDLQHandler {
	return &AdminNotificationDLQHandler{repo: repo, log: log}
}

// notifDLQEntryResponse is the JSON shape for a single DLQ entry.
type notifDLQEntryResponse struct {
	ID          int64   `json:"id"`
	Channel     string  `json:"channel"`
	UserID      *int    `json:"user_id,omitempty"`
	EventType   string  `json:"event_type"`
	ErrorDetail string  `json:"error_detail"`
	Attempts    int     `json:"attempts"`
	CreatedAt   string  `json:"created_at"`
	Resolved    bool    `json:"resolved"`
}

// NotifDLQStatsResponse is the JSON body for GET /admin/notification-dlq.
type NotifDLQStatsResponse struct {
	UnresolvedCount int64                   `json:"unresolved_count"`
	Recent          []notifDLQEntryResponse `json:"recent"`
}

func notifDLQEntryToResponse(e *domain.NotificationDLQEntry) notifDLQEntryResponse {
	return notifDLQEntryResponse{
		ID:          e.ID,
		Channel:     e.Channel,
		UserID:      e.UserID,
		EventType:   e.EventType,
		ErrorDetail: e.ErrorDetail,
		Attempts:    e.Attempts,
		CreatedAt:   e.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Resolved:    e.ResolvedAt != nil,
	}
}

// Stats handles GET /admin/notification-dlq.
//
// @Summary      Notification delivery DLQ stats
// @Description  Returns the count of unresolved notification delivery failures
//
//	(notification_dlq table) and a sample of the most recent entries.
//	This is the PostgreSQL-backed delivery DLQ; the Redis Streams DLQ
//	(event processing failures) is exposed at GET /admin/dlq.
//
// @Tags         admin-notification-dlq
// @Produce      json
// @Security     BearerAuth
// @Param        limit  query  int  false  "Max recent entries to return (1–100, default 20)"
// @Success      200  {object}  handler.NotifDLQStatsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-dlq [get]
func (h *AdminNotificationDLQHandler) Stats(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n >= 1 && n <= 100 {
			limit = n
		}
	}

	total, err := h.repo.CountUnresolved(r.Context())
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	recent, err := h.repo.ListRecent(r.Context(), limit)
	if err != nil {
		writeError(w, r, h.log, err)
		return
	}

	resp := NotifDLQStatsResponse{
		UnresolvedCount: total,
		Recent:          make([]notifDLQEntryResponse, len(recent)),
	}
	for i, e := range recent {
		resp.Recent[i] = notifDLQEntryToResponse(e)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Resolve handles POST /admin/notification-dlq/{id}/resolve.
//
// @Summary      Manually resolve a notification DLQ entry
// @Description  Marks a notification delivery failure as resolved, removing it
//
//	from future DLQWorker replay batches.  Use for entries that have
//	exhausted retry attempts or that cannot be re-delivered (e.g. the
//	target user no longer exists).  This is a write-off operation; it
//	does not re-deliver the notification.
//
// @Tags         admin-notification-dlq
// @Security     BearerAuth
// @Param        id   path      int  true  "DLQ entry ID"
// @Success      204
// @Failure      400  {object}  handler.ErrorResponse  "Invalid ID"
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/notification-dlq/{id}/resolve [post]
func (h *AdminNotificationDLQHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, r, h.log, apperrors.Validation("id must be a positive integer"))
		return
	}

	if err := h.repo.MarkResolved(r.Context(), id); err != nil {
		writeError(w, r, h.log, err)
		return
	}

	h.log.Info("admin: notification DLQ entry manually resolved",
		zap.Int64("dlq_id", id),
	)
	w.WriteHeader(http.StatusNoContent)
}
