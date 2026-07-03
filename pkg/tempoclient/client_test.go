package tempoclient_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/pkg/tempoclient"
)

func TestSearchErrors_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/search" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("tags") == "" {
			t.Error("expected tags query param")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tempoclient.SearchResponse{
			Traces: []tempoclient.TraceSummary{
				{TraceID: "abc123", RootServiceName: "api", DurationMs: 42},
			},
		})
	}))
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.SearchErrors(context.Background(), time.Now().Add(-1*time.Hour), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Errorf("expected 1 trace, got %d", len(resp.Traces))
	}
	if resp.Traces[0].TraceID != "abc123" {
		t.Errorf("unexpected traceID: %s", resp.Traces[0].TraceID)
	}
}

func TestSearchErrors_EmptyTraces(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tempoclient.SearchResponse{Traces: []tempoclient.TraceSummary{}})
	}))
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.SearchErrors(context.Background(), time.Now().Add(-5*time.Minute), 50)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 0 {
		t.Errorf("expected 0 traces, got %d", len(resp.Traces))
	}
}

func TestSearchErrors_ConnectionRefused(t *testing.T) {
	c := tempoclient.New("http://127.0.0.1:1")
	_, err := c.SearchErrors(context.Background(), time.Now().Add(-1*time.Hour), 10)
	if err == nil {
		t.Error("expected error when Tempo is unreachable")
	}
}

func TestSearchErrors_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	_, err := c.SearchErrors(context.Background(), time.Now().Add(-1*time.Hour), 10)
	if err == nil {
		t.Error("expected error on non-200 status from Tempo")
	}
}

func TestSearchErrors_InvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	_, err := c.SearchErrors(context.Background(), time.Now().Add(-1*time.Hour), 10)
	if err == nil {
		t.Error("expected error when Tempo returns invalid JSON")
	}
}

func TestSearchErrors_InvalidBaseURL(t *testing.T) {
	c := tempoclient.New("://invalid-scheme")
	_, err := c.SearchErrors(context.Background(), time.Now().Add(-1*time.Hour), 10)
	if err == nil {
		t.Error("expected error for invalid base URL")
	}
}

// ── Search (with nameFilter) ──────────────────────────────────────────────────

func makeSearchServer(t *testing.T, traces []tempoclient.TraceSummary) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tempoclient.SearchResponse{Traces: traces})
	}))
}

func TestSearch_NoFilter(t *testing.T) {
	traces := []tempoclient.TraceSummary{
		{TraceID: "t1", RootServiceName: "api", RootTraceName: "GET /foo"},
		{TraceID: "t2", RootServiceName: "worker", RootTraceName: "job.run"},
	}
	srv := makeSearchServer(t, traces)
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.Search(context.Background(), "span.status.code=ERROR", time.Now().Add(-time.Hour), 50, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 2 {
		t.Errorf("expected 2 traces without filter, got %d", len(resp.Traces))
	}
}

func TestSearch_NameFilterMatches(t *testing.T) {
	traces := []tempoclient.TraceSummary{
		{TraceID: "t1", RootServiceName: "api", RootTraceName: "POST /api/v1/paypal/create-order"},
		{TraceID: "t2", RootServiceName: "api", RootTraceName: "GET /api/v1/users/me"},
	}
	srv := makeSearchServer(t, traces)
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.Search(context.Background(), "span.status.code=ERROR", time.Now().Add(-time.Hour), 50, "paypal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Fatalf("expected 1 trace after filter, got %d", len(resp.Traces))
	}
	if resp.Traces[0].TraceID != "t1" {
		t.Errorf("expected traceID t1, got %s", resp.Traces[0].TraceID)
	}
}

func TestSearch_NameFilterCaseInsensitive(t *testing.T) {
	traces := []tempoclient.TraceSummary{
		{TraceID: "t1", RootServiceName: "API", RootTraceName: "POST /Paypal"},
	}
	srv := makeSearchServer(t, traces)
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.Search(context.Background(), "span.status.code=ERROR", time.Now().Add(-time.Hour), 50, "paypal")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Errorf("expected case-insensitive match, got %d traces", len(resp.Traces))
	}
}

func TestSearch_NameFilterByService(t *testing.T) {
	traces := []tempoclient.TraceSummary{
		{TraceID: "t1", RootServiceName: "worker", RootTraceName: "job.cleanup"},
		{TraceID: "t2", RootServiceName: "api", RootTraceName: "GET /health"},
	}
	srv := makeSearchServer(t, traces)
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.Search(context.Background(), "span.status.code=ERROR", time.Now().Add(-time.Hour), 50, "worker")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Fatalf("expected 1 trace matching service name, got %d", len(resp.Traces))
	}
	if resp.Traces[0].TraceID != "t1" {
		t.Errorf("expected traceID t1, got %s", resp.Traces[0].TraceID)
	}
}

func TestSearch_NameFilterNoMatch(t *testing.T) {
	traces := []tempoclient.TraceSummary{
		{TraceID: "t1", RootServiceName: "api", RootTraceName: "GET /foo"},
	}
	srv := makeSearchServer(t, traces)
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	resp, err := c.Search(context.Background(), "span.status.code=ERROR", time.Now().Add(-time.Hour), 50, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Traces) != 0 {
		t.Errorf("expected 0 traces when filter matches nothing, got %d", len(resp.Traces))
	}
}

// TestSearch_ResponseBodyOverLimit verifies the client stops reading a Tempo
// response after maxResponseBytes rather than buffering an unbounded body —
// a misconfigured or compromised Tempo backend (its URL is operator-supplied
// config) must not be able to exhaust memory.
func TestSearch_ResponseBodyOverLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		pad := make([]byte, 10<<20+1024)
		for i := range pad {
			pad[i] = ' '
		}
		w.Write(pad)
		w.Write([]byte(`{"traces":[]}`))
	}))
	defer srv.Close()

	c := tempoclient.New(srv.URL)
	_, err := c.SearchErrors(context.Background(), time.Now().Add(-time.Hour), 50)
	if err == nil {
		t.Error("expected decode error when response body exceeds the size cap")
	}
}
