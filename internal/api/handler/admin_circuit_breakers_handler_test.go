package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
)

func newCircuitBreakersRouter(reg handler.BreakerLister) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminCircuitBreakersHandler(reg, zap.NewNop())
	r.Get("/circuit-breakers", h.List)
	return r
}

func TestAdminCircuitBreakers_EmptyRegistry(t *testing.T) {
	reg := breaker.NewRegistry()
	w := do(newCircuitBreakersRouter(reg), http.MethodGet, "/circuit-breakers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries, _ := resp["breakers"].([]any)
	if len(entries) != 0 {
		t.Errorf("expected empty breakers slice, got %d entries", len(entries))
	}
}

func TestAdminCircuitBreakers_ClosedBreaker(t *testing.T) {
	reg := breaker.NewRegistry()
	b := breaker.New("test-backend", 3, 30*1e9)
	reg.Register(b)

	w := do(newCircuitBreakersRouter(reg), http.MethodGet, "/circuit-breakers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries, _ := resp["breakers"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 breaker, got %d", len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	if entry["name"] != "test-backend" {
		t.Errorf("expected name=test-backend, got %v", entry["name"])
	}
	if entry["state"] != "closed" {
		t.Errorf("expected state=closed, got %v", entry["state"])
	}
	if _, hasOpenedAt := entry["opened_at"]; hasOpenedAt {
		t.Errorf("closed breaker should not have opened_at")
	}
}

func TestAdminCircuitBreakers_OpenBreaker(t *testing.T) {
	reg := breaker.NewRegistry()
	b := breaker.New("failing-backend", 1, 30*1e9) // opens after 1 failure
	reg.Register(b)

	// Trigger the breaker open.
	_ = b.Call(func() error { return errTestObs })

	w := do(newCircuitBreakersRouter(reg), http.MethodGet, "/circuit-breakers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries, _ := resp["breakers"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 breaker, got %d", len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	if entry["state"] != "open" {
		t.Errorf("expected state=open, got %v", entry["state"])
	}
	if _, hasOpenedAt := entry["opened_at"]; !hasOpenedAt {
		t.Errorf("open breaker should have opened_at field")
	}
}

func TestAdminCircuitBreakers_MultipleBreakers(t *testing.T) {
	reg := breaker.NewRegistry()
	reg.Register(breaker.New("a", 5, 30*1e9))
	reg.Register(breaker.New("b", 5, 30*1e9))
	reg.Register(breaker.New("c", 5, 30*1e9))

	w := do(newCircuitBreakersRouter(reg), http.MethodGet, "/circuit-breakers", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	entries, _ := resp["breakers"].([]any)
	if len(entries) != 3 {
		t.Errorf("expected 3 breakers, got %d", len(entries))
	}
}
