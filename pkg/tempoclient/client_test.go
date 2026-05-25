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
