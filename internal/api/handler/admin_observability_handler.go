package handler

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/promclient"
	"github.com/rede/world-cup-quiniela/pkg/tempoclient"
)

// PromQuerier is the interface the metrics-summary handler requires from a
// Prometheus client. The concrete *promclient.Client satisfies this; tests may
// supply a stub.
type PromQuerier interface {
	Query(ctx context.Context, query string) (*promclient.QueryResponse, error)
}

// TempoQuerier is the interface the tracing handler requires from a Tempo
// client. The concrete *tempoclient.Client satisfies this; tests may supply a stub.
type TempoQuerier interface {
	SearchErrors(ctx context.Context, since time.Time, limit int) (*tempoclient.SearchResponse, error)
}

// AdminObservabilityHandler exposes admin-only observability endpoints:
// metrics summary (Prometheus), recent error traces (Tempo), and active SSE
// connection count (Hub). All data is read-only; no mutations occur.
type AdminObservabilityHandler struct {
	prom PromQuerier
	temp TempoQuerier
	hub  HubStater
	log  *zap.Logger
}

// NewAdminObservabilityHandler constructs the handler.
// prom and temp may be nil when the respective backends are not configured;
// the handler returns graceful "unconfigured" responses in that case.
// The concrete *promclient.Client and *tempoclient.Client satisfy PromQuerier
// and TempoQuerier respectively; pass nil to disable the backend.
func NewAdminObservabilityHandler(prom PromQuerier, temp TempoQuerier, hub HubStater, log *zap.Logger) *AdminObservabilityHandler {
	return &AdminObservabilityHandler{prom: prom, temp: temp, hub: hub, log: log}
}

// ── Metrics summary ──────────────────────────────────────────────────────────

type metricsSummaryResponse struct {
	Configured  bool    `json:"configured"`
	RequestRate float64 `json:"request_rate_per_sec"`
	ErrorRate   float64 `json:"error_rate_5xx"`
	P95LatencyS float64 `json:"p95_latency_seconds"`
}

// MetricsSummary handles GET /admin/observability/metrics/summary.
//
// @Summary      Observability metrics summary
// @Description  Returns the current HTTP request rate, 5xx error rate, and p95 latency
//
//	queried live from Prometheus. Returns configured=false when Prometheus is
//	not configured (WCQ_OBSERVABILITY_PROMETHEUSURL is unset).
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.metricsSummaryResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/observability/metrics/summary [get]
func (h *AdminObservabilityHandler) MetricsSummary(w http.ResponseWriter, r *http.Request) {
	if h.prom == nil {
		writeJSON(w, http.StatusOK, metricsSummaryResponse{Configured: false})
		return
	}
	ctx := r.Context()

	rateQR, err := h.prom.Query(ctx, `sum(rate(http_server_request_duration_seconds_count[5m]))`)
	if err != nil {
		h.log.Warn("metrics/summary: request rate query failed", zap.Error(err))
		writeJSON(w, http.StatusOK, metricsSummaryResponse{Configured: true})
		return
	}
	requestRate, _ := promclient.FirstFloat(rateQR)

	errQR, err := h.prom.Query(ctx,
		`sum(rate(http_server_request_duration_seconds_count{http_status_code=~"5.."}[5m])) / `+
			`sum(rate(http_server_request_duration_seconds_count[5m]))`)
	if err != nil {
		h.log.Warn("metrics/summary: error rate query failed", zap.Error(err))
		writeJSON(w, http.StatusOK, metricsSummaryResponse{Configured: true, RequestRate: requestRate})
		return
	}
	errorRate, _ := promclient.FirstFloat(errQR)

	latQR, err := h.prom.Query(ctx,
		`histogram_quantile(0.95, sum by (le) (rate(http_server_request_duration_seconds_bucket[5m])))`)
	if err != nil {
		h.log.Warn("metrics/summary: p95 latency query failed", zap.Error(err))
		writeJSON(w, http.StatusOK, metricsSummaryResponse{Configured: true, RequestRate: requestRate, ErrorRate: errorRate})
		return
	}
	p95, _ := promclient.FirstFloat(latQR)

	writeJSON(w, http.StatusOK, metricsSummaryResponse{
		Configured:  true,
		RequestRate: requestRate,
		ErrorRate:   errorRate,
		P95LatencyS: p95,
	})
}

// ── Tracing recent errors ────────────────────────────────────────────────────

type tracingErrorEntry struct {
	TraceID         string `json:"trace_id"`
	RootServiceName string `json:"root_service_name"`
	RootTraceName   string `json:"root_trace_name"`
	StartTimeNs     string `json:"start_time_unix_nano"`
	DurationMs      uint32 `json:"duration_ms"`
}

type tracingRecentErrorsResponse struct {
	Configured bool                `json:"configured"`
	Errors     []tracingErrorEntry `json:"errors"`
}

// TracingRecentErrors handles GET /admin/observability/tracing/recent-errors.
//
// @Summary      Recent error traces
// @Description  Returns up to 50 traces containing error spans from the last 15 minutes,
//
//	queried live from Grafana Tempo. Returns configured=false when Tempo is
//	not configured (WCQ_OBSERVABILITY_TEMPOURL is unset).
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.tracingRecentErrorsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      500  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/observability/tracing/recent-errors [get]
func (h *AdminObservabilityHandler) TracingRecentErrors(w http.ResponseWriter, r *http.Request) {
	if h.temp == nil {
		writeJSON(w, http.StatusOK, tracingRecentErrorsResponse{Configured: false, Errors: []tracingErrorEntry{}})
		return
	}
	since := time.Now().Add(-15 * time.Minute)
	sr, err := h.temp.SearchErrors(r.Context(), since, 50)
	if err != nil {
		h.log.Warn("tracing/recent-errors: tempo search failed", zap.Error(err))
		writeJSON(w, http.StatusOK, tracingRecentErrorsResponse{Configured: true, Errors: []tracingErrorEntry{}})
		return
	}
	entries := make([]tracingErrorEntry, len(sr.Traces))
	for i, t := range sr.Traces {
		entries[i] = tracingErrorEntry{
			TraceID:         t.TraceID,
			RootServiceName: t.RootServiceName,
			RootTraceName:   t.RootTraceName,
			StartTimeNs:     t.StartTimeUnixNano,
			DurationMs:      t.DurationMs,
		}
	}
	writeJSON(w, http.StatusOK, tracingRecentErrorsResponse{Configured: true, Errors: entries})
}

// ── Active connections ───────────────────────────────────────────────────────

type activeConnectionsResponse struct {
	Connections int64 `json:"connections"`
	Broadcasts  int64 `json:"broadcasts"`
	Dropped     int64 `json:"dropped"`
}

// ActiveConnections handles GET /admin/observability/active-connections.
//
// @Summary      Active SSE connections
// @Description  Returns per-replica SSE connection count, cumulative broadcasts, and
//
//	dropped events from the notification hub. Aggregate across replicas in
//	Prometheus for cluster-wide totals.
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.activeConnectionsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Router       /api/v1/admin/observability/active-connections [get]
func (h *AdminObservabilityHandler) ActiveConnections(w http.ResponseWriter, _ *http.Request) {
	connections, broadcasts, dropped := h.hub.Metrics()
	writeJSON(w, http.StatusOK, activeConnectionsResponse{
		Connections: connections,
		Broadcasts:  broadcasts,
		Dropped:     dropped,
	})
}
