package handler

import (
	"net/http"

	"go.uber.org/zap"
)

// HubStater is the minimal interface the SSE stats handler requires from the
// hub. The concrete *hub.Hub satisfies this interface; tests may supply a stub.
type HubStater interface {
	Metrics() (connections, broadcasts, dropped int64)
}

// AdminSSEStatsHandler exposes real-time SSE hub counters to admin operators.
// The returned values are per-replica: in a multi-replica deployment, clients
// should sum across all instances (or rely on Prometheus aggregation).
type AdminSSEStatsHandler struct {
	hub HubStater
	log *zap.Logger
}

// NewAdminSSEStatsHandler constructs the handler.
func NewAdminSSEStatsHandler(hub HubStater, log *zap.Logger) *AdminSSEStatsHandler {
	return &AdminSSEStatsHandler{hub: hub, log: log}
}

type sseStatsResponse struct {
	Connections int64 `json:"connections"`
	Broadcasts  int64 `json:"broadcasts"`
	Dropped     int64 `json:"dropped"`
}

// Stats returns the current SSE hub counters for this replica.
//
//	@Summary		SSE hub stats
//	@Description	Returns per-replica SSE connection count, cumulative broadcasts, and dropped events. Aggregate across replicas for cluster-wide totals.
//	@Tags			admin,notifications
//	@Produce		json
//	@Success		200	{object}	sseStatsResponse
//	@Failure		500	{object}	handler.ErrorResponse
//	@Security		BearerAuth
//	@Router			/api/v1/admin/notifications/sse/stats [get]
func (h *AdminSSEStatsHandler) Stats(w http.ResponseWriter, r *http.Request) {
	connections, broadcasts, dropped := h.hub.Metrics()
	writeJSON(w, http.StatusOK, sseStatsResponse{
		Connections: connections,
		Broadcasts:  broadcasts,
		Dropped:     dropped,
	})
}
