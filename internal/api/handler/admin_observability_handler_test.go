package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/pkg/promclient"
	"github.com/rede/world-cup-quiniela/pkg/tempoclient"
)

var errTestObs = errors.New("test error")

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubPromQuerier struct {
	resp *promclient.QueryResponse
	err  error
}

func (s *stubPromQuerier) Query(_ context.Context, _ string) (*promclient.QueryResponse, error) {
	return s.resp, s.err
}

type multiPromQuerier struct {
	resps []*promclient.QueryResponse
	idx   int
	calls int
}

func (m *multiPromQuerier) Query(_ context.Context, _ string) (*promclient.QueryResponse, error) {
	m.calls++
	if m.idx >= len(m.resps) {
		return &promclient.QueryResponse{Status: "success", Data: promclient.QueryData{}}, nil
	}
	r := m.resps[m.idx]
	m.idx++
	return r, nil
}

type stubTempoQuerier struct {
	resp *tempoclient.SearchResponse
	err  error
}

func (s *stubTempoQuerier) SearchErrors(_ context.Context, _ time.Time, _ int) (*tempoclient.SearchResponse, error) {
	return s.resp, s.err
}

// obsHub wraps HubStater for observability tests (avoids redeclaring stubHubStater).
type obsHub struct {
	conns, broadcasts, dropped int64
}

func (s *obsHub) Metrics() (int64, int64, int64) {
	return s.conns, s.broadcasts, s.dropped
}

// makeVectorResp builds a minimal Prometheus vector response containing one
// sample with the given string value.
func makeVectorResp(val string) *promclient.QueryResponse {
	raw := json.RawMessage(`{"metric":{},"value":[1700000000,"` + val + `"]}`)
	return &promclient.QueryResponse{
		Status: "success",
		Data: promclient.QueryData{
			ResultType: "vector",
			Result:     []json.RawMessage{raw},
		},
	}
}

// ── MetricsSummary ────────────────────────────────────────────────────────────

func TestAdminObservabilityHandler_MetricsSummary_Unconfigured(t *testing.T) {
	h := handler.NewAdminObservabilityHandler(nil, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/metrics/summary", h.MetricsSummary)

	req := withCaller(newAdminRequest(http.MethodGet, "/metrics/summary", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != false {
		t.Errorf("expected configured=false, got %v", resp["configured"])
	}
}

func TestAdminObservabilityHandler_MetricsSummary_Configured(t *testing.T) {
	prom := &multiPromQuerier{
		resps: []*promclient.QueryResponse{
			makeVectorResp("10"),
			makeVectorResp("0.01"),
			makeVectorResp("0.123"),
		},
	}

	h := handler.NewAdminObservabilityHandler(prom, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/metrics/summary", h.MetricsSummary)

	req := withCaller(newAdminRequest(http.MethodGet, "/metrics/summary", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true")
	}
	if prom.calls != 3 {
		t.Errorf("expected 3 Prometheus queries, got %d", prom.calls)
	}
	if resp["request_rate_per_sec"] != float64(10) {
		t.Errorf("unexpected request_rate_per_sec: %v", resp["request_rate_per_sec"])
	}
}

func TestAdminObservabilityHandler_MetricsSummary_PromError(t *testing.T) {
	prom := &stubPromQuerier{err: errTestObs}
	h := handler.NewAdminObservabilityHandler(prom, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/metrics/summary", h.MetricsSummary)

	req := withCaller(newAdminRequest(http.MethodGet, "/metrics/summary", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true even on error")
	}
}

func TestAdminObservabilityHandler_MetricsSummary_ErrorRateQueryFails(t *testing.T) {
	// First query (rate) succeeds; second (error rate) fails.
	calls := 0
	prom := &callCountProm{
		fn: func(_ int) (*promclient.QueryResponse, error) {
			calls++
			if calls == 2 {
				return nil, errTestObs
			}
			return makeVectorResp("5"), nil
		},
	}
	h := handler.NewAdminObservabilityHandler(prom, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/metrics/summary", h.MetricsSummary)

	req := withCaller(newAdminRequest(http.MethodGet, "/metrics/summary", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful), got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true")
	}
	// request_rate should be present, error_rate/p95 should be zero (default).
	if resp["request_rate_per_sec"] != float64(5) {
		t.Errorf("expected request_rate=5, got %v", resp["request_rate_per_sec"])
	}
}

func TestAdminObservabilityHandler_MetricsSummary_P95QueryFails(t *testing.T) {
	calls := 0
	prom := &callCountProm{
		fn: func(_ int) (*promclient.QueryResponse, error) {
			calls++
			if calls == 3 {
				return nil, errTestObs
			}
			return makeVectorResp("1"), nil
		},
	}
	h := handler.NewAdminObservabilityHandler(prom, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/metrics/summary", h.MetricsSummary)

	req := withCaller(newAdminRequest(http.MethodGet, "/metrics/summary", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful), got %d", rr.Code)
	}
}

type callCountProm struct {
	fn   func(call int) (*promclient.QueryResponse, error)
	call int
}

func (c *callCountProm) Query(_ context.Context, _ string) (*promclient.QueryResponse, error) {
	c.call++
	return c.fn(c.call)
}

// ── TracingRecentErrors ───────────────────────────────────────────────────────

func TestAdminObservabilityHandler_TracingRecentErrors_Unconfigured(t *testing.T) {
	h := handler.NewAdminObservabilityHandler(nil, nil, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/tracing/recent-errors", h.TracingRecentErrors)

	req := withCaller(newAdminRequest(http.MethodGet, "/tracing/recent-errors", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != false {
		t.Errorf("expected configured=false, got %v", resp["configured"])
	}
}

func TestAdminObservabilityHandler_TracingRecentErrors_WithResults(t *testing.T) {
	tempo := &stubTempoQuerier{resp: &tempoclient.SearchResponse{
		Traces: []tempoclient.TraceSummary{
			{TraceID: "abc123", RootServiceName: "api", RootTraceName: "GET /foo", DurationMs: 42},
		},
	}}

	h := handler.NewAdminObservabilityHandler(nil, tempo, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/tracing/recent-errors", h.TracingRecentErrors)

	req := withCaller(newAdminRequest(http.MethodGet, "/tracing/recent-errors", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true")
	}
	errs, _ := resp["errors"].([]any)
	if len(errs) != 1 {
		t.Errorf("expected 1 error trace, got %d", len(errs))
	}
}

func TestAdminObservabilityHandler_TracingRecentErrors_TempoError(t *testing.T) {
	tempo := &stubTempoQuerier{err: errTestObs}
	h := handler.NewAdminObservabilityHandler(nil, tempo, &obsHub{}, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/tracing/recent-errors", h.TracingRecentErrors)

	req := withCaller(newAdminRequest(http.MethodGet, "/tracing/recent-errors", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	errors, _ := resp["errors"].([]any)
	if len(errors) != 0 {
		t.Errorf("expected empty errors slice on tempo failure")
	}
}

// ── ActiveConnections ─────────────────────────────────────────────────────────

func TestAdminObservabilityHandler_ActiveConnections(t *testing.T) {
	hub := &obsHub{conns: 5, broadcasts: 100, dropped: 2}
	h := handler.NewAdminObservabilityHandler(nil, nil, hub, zap.NewNop())
	router := chi.NewRouter()
	router.Get("/active-connections", h.ActiveConnections)

	req := withCaller(newAdminRequest(http.MethodGet, "/active-connections", ""), adminCaller)
	rr := doReq(router, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp map[string]any
	json.NewDecoder(rr.Body).Decode(&resp)
	if resp["connections"] != float64(5) {
		t.Errorf("expected connections=5, got %v", resp["connections"])
	}
	if resp["broadcasts"] != float64(100) {
		t.Errorf("expected broadcasts=100, got %v", resp["broadcasts"])
	}
	if resp["dropped"] != float64(2) {
		t.Errorf("expected dropped=2, got %v", resp["dropped"])
	}
}
