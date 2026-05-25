package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// errN8nUnavailable wraps an upstream n8n communication error as an
// apperrors.Internal so writeError can serialise it as a 500.
func errN8nUnavailable(cause error) error {
	return apperrors.Internal(fmt.Errorf("n8n unavailable: %w", cause))
}

// AdminN8nHandler exposes admin-only endpoints for querying the n8n automation
// platform: workflow list and recent execution history.
type AdminN8nHandler struct {
	baseURL string
	apiKey  string
	http    *http.Client
	log     *zap.Logger
}

// NewAdminN8nHandler constructs the handler.
// baseURL is the n8n base URL (e.g. "http://n8n:5678"); apiKey is the n8n API key.
// When either is empty, the endpoints return unconfigured responses.
func NewAdminN8nHandler(baseURL, apiKey string, log *zap.Logger) *AdminN8nHandler {
	return &AdminN8nHandler{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http: &http.Client{
			Timeout:   10 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		log: log,
	}
}

func (h *AdminN8nHandler) configured() bool {
	return h.baseURL != "" && h.apiKey != ""
}

func (h *AdminN8nHandler) doGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, h.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("n8n: build request: %w", err)
	}
	req.Header.Set("X-N8N-API-KEY", h.apiKey)

	resp, err := h.http.Do(req)
	if err != nil {
		return fmt.Errorf("n8n: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("n8n: status %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("n8n: decode response: %w", err)
	}
	return nil
}

// ── Workflows ────────────────────────────────────────────────────────────────

type n8nWorkflow struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type n8nWorkflowsEnvelope struct {
	Data []n8nWorkflow `json:"data"`
}

type workflowsResponse struct {
	Configured bool          `json:"configured"`
	Workflows  []n8nWorkflow `json:"workflows"`
}

// Workflows handles GET /admin/observability/n8n/workflows.
//
// @Summary      n8n workflow list
// @Description  Returns the list of n8n workflows via the n8n REST API.
//
//	Returns configured=false when n8n is not configured
//	(WCQ_N8N_BASEURL or WCQ_N8N_APIKEY is unset).
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.workflowsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      502  {object}  handler.ErrorResponse  "n8n unreachable"
// @Router       /api/v1/admin/observability/n8n/workflows [get]
func (h *AdminN8nHandler) Workflows(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		writeJSON(w, http.StatusOK, workflowsResponse{Configured: false, Workflows: []n8nWorkflow{}})
		return
	}

	var envelope n8nWorkflowsEnvelope
	if err := h.doGet(r.Context(), "/api/v1/workflows", &envelope); err != nil {
		h.log.Warn("n8n: workflows fetch failed", append(tracing.LogFields(r.Context()), zap.Error(err))...)
		writeError(w, r, h.log, errN8nUnavailable(err))
		return
	}
	workflows := envelope.Data
	if workflows == nil {
		workflows = []n8nWorkflow{}
	}
	writeJSON(w, http.StatusOK, workflowsResponse{Configured: true, Workflows: workflows})
}

// ── Recent executions ────────────────────────────────────────────────────────

type n8nExecution struct {
	ID           json.Number `json:"id"`
	WorkflowID   string      `json:"workflowId"`
	WorkflowName string      `json:"workflowName,omitempty"`
	Finished     bool        `json:"finished"`
	Mode         string      `json:"mode"`
	Status       string      `json:"status"`
	StartedAt    string      `json:"startedAt"`
	StoppedAt    *string     `json:"stoppedAt,omitempty"`
}

type n8nExecutionsEnvelope struct {
	Data []n8nExecution `json:"data"`
}

type executionsResponse struct {
	Configured bool           `json:"configured"`
	Executions []n8nExecution `json:"executions"`
}

// RecentExecutions handles GET /admin/observability/n8n/executions/recent.
//
// @Summary      Recent n8n executions
// @Description  Returns the 20 most recent n8n workflow executions via the n8n REST API.
//
//	Returns configured=false when n8n is not configured.
//
// @Tags         admin-observability
// @Produce      json
// @Security     BearerAuth
// @Success      200  {object}  handler.executionsResponse
// @Failure      401  {object}  handler.ErrorResponse
// @Failure      403  {object}  handler.ErrorResponse
// @Failure      502  {object}  handler.ErrorResponse  "n8n unreachable"
// @Router       /api/v1/admin/observability/n8n/executions/recent [get]
func (h *AdminN8nHandler) RecentExecutions(w http.ResponseWriter, r *http.Request) {
	if !h.configured() {
		writeJSON(w, http.StatusOK, executionsResponse{Configured: false, Executions: []n8nExecution{}})
		return
	}

	var envelope n8nExecutionsEnvelope
	if err := h.doGet(r.Context(), "/api/v1/executions?limit=20", &envelope); err != nil {
		h.log.Warn("n8n: executions fetch failed", append(tracing.LogFields(r.Context()), zap.Error(err))...)
		writeError(w, r, h.log, errN8nUnavailable(err))
		return
	}
	execs := envelope.Data
	if execs == nil {
		execs = []n8nExecution{}
	}
	writeJSON(w, http.StatusOK, executionsResponse{Configured: true, Executions: execs})
}
