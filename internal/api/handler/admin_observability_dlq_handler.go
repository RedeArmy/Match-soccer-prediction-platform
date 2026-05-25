package handler

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// AdminObservabilityDLQHandler provides a unified read-only view of both the
// event-bus DLQ (Redis Streams) and the notification DLQ (PostgreSQL), intended
// for the observability dashboard.
type AdminObservabilityDLQHandler struct {
	eventDLQ     service.DLQService
	notifDLQRepo repository.NotificationDLQRepository
	log          *zap.Logger
}

// NewAdminObservabilityDLQHandler constructs the handler.
func NewAdminObservabilityDLQHandler(eventDLQ service.DLQService, notifDLQRepo repository.NotificationDLQRepository, log *zap.Logger) *AdminObservabilityDLQHandler {
	return &AdminObservabilityDLQHandler{eventDLQ: eventDLQ, notifDLQRepo: notifDLQRepo, log: log}
}

type eventBusDLQEntry struct {
	EventType string  `json:"event_type"`
	Count     int64   `json:"count"`
	OldestAt  *string `json:"oldest_at,omitempty"`
}

type unifiedDLQResponse struct {
	EventBus struct {
		Entries []eventBusDLQEntry `json:"entries"`
	} `json:"event_bus"`
	Notifications struct {
		UnresolvedCount int64 `json:"unresolved_count"`
	} `json:"notifications"`
}

// Stats handles GET /admin/observability/dlq.
//
// @Summary      Unified DLQ stats
// @Description  Returns a combined view of the event-bus DLQ (Redis Streams) and the
//
//	notification DLQ (PostgreSQL). Provides at-a-glance health of both
//	pipeline failure queues without requiring separate admin panel pages.
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.unifiedDLQResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/observability/dlq [get]
func (h *AdminObservabilityDLQHandler) Stats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	eventStats, err := h.eventDLQ.Stats(ctx)
	if err != nil {
		h.log.Warn("observability/dlq: event bus stats failed", zap.Error(err))
		eventStats = []service.DLQStat{}
	}

	notifCount, err := h.notifDLQRepo.CountUnresolved(ctx)
	if err != nil {
		h.log.Warn("observability/dlq: notification DLQ count failed", zap.Error(err))
	}

	resp := unifiedDLQResponse{}
	resp.EventBus.Entries = make([]eventBusDLQEntry, len(eventStats))
	for i, s := range eventStats {
		entry := eventBusDLQEntry{
			EventType: s.EventType,
			Count:     s.Count,
		}
		if s.OldestAt != nil {
			ts := s.OldestAt.UTC().Format("2006-01-02T15:04:05Z")
			entry.OldestAt = &ts
		}
		resp.EventBus.Entries[i] = entry
	}
	resp.Notifications.UnresolvedCount = notifCount

	writeJSON(w, http.StatusOK, resp)
}
