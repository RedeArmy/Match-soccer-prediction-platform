package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

func newN8nRouter(baseURL, apiKey string) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminN8nHandler(baseURL, apiKey, zap.NewNop())
	r.Get("/n8n/workflows", h.Workflows)
	r.Get("/n8n/executions/recent", h.RecentExecutions)
	return r
}

// ── Unconfigured ──────────────────────────────────────────────────────────────

func TestAdminN8nHandler_Workflows_Unconfigured(t *testing.T) {
	w := do(newN8nRouter("", ""), http.MethodGet, "/n8n/workflows", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["configured"] != false {
		t.Errorf("expected configured=false, got %v", resp["configured"])
	}
}

func TestAdminN8nHandler_Executions_Unconfigured(t *testing.T) {
	w := do(newN8nRouter("", ""), http.MethodGet, "/n8n/executions/recent", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["configured"] != false {
		t.Errorf("expected configured=false, got %v", resp["configured"])
	}
}

// ── Configured with fake n8n server ──────────────────────────────────────────

func TestAdminN8nHandler_Workflows_Configured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-N8N-API-KEY") != "test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":"1","name":"dlq-overflow","active":true,"createdAt":"2024-01-01T00:00:00Z","updatedAt":"2024-01-01T00:00:00Z"}]}`))
	}))
	defer srv.Close()

	w := do(newN8nRouter(srv.URL, "test-key"), http.MethodGet, "/n8n/workflows", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true")
	}
	workflows, _ := resp["workflows"].([]any)
	if len(workflows) != 1 {
		t.Errorf("expected 1 workflow, got %d", len(workflows))
	}
}

func TestAdminN8nHandler_Executions_Configured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"data":[{"id":42,"workflowId":"1","finished":true,"mode":"webhook","status":"success","startedAt":"2024-01-01T00:00:00Z"}]}`))
	}))
	defer srv.Close()

	w := do(newN8nRouter(srv.URL, "key"), http.MethodGet, "/n8n/executions/recent", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["configured"] != true {
		t.Errorf("expected configured=true")
	}
	execs, _ := resp["executions"].([]any)
	if len(execs) != 1 {
		t.Errorf("expected 1 execution, got %d", len(execs))
	}
}

func TestAdminN8nHandler_Workflows_N8nDown(t *testing.T) {
	// Point at a URL that will refuse the connection.
	w := do(newN8nRouter("http://127.0.0.1:1", "key"), http.MethodGet, "/n8n/workflows", "")
	// Should return 500 (internal error from apperrors.Internal wrapping connection refused).
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when n8n is down, got %d", w.Code)
	}
}

func TestAdminN8nHandler_Executions_N8nDown(t *testing.T) {
	w := do(newN8nRouter("http://127.0.0.1:1", "key"), http.MethodGet, "/n8n/executions/recent", "")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when n8n is down, got %d", w.Code)
	}
}
