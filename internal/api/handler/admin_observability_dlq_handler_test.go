package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

func newObsDLQRouter(eventDLQ service.DLQService, notifRepo repository.NotificationDLQRepository) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminObservabilityDLQHandler(eventDLQ, notifRepo, zap.NewNop())
	r.Get("/observability/dlq", h.Stats)
	return r
}

func TestAdminObservabilityDLQ_EmptyQueues(t *testing.T) {
	eventDLQ := &stubDLQSvc{stats: []service.DLQStat{}}
	notif := &stubNotifDLQRepo{count: 0}

	w := do(newObsDLQRouter(eventDLQ, notif), http.MethodGet, "/observability/dlq", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	eventBus, _ := resp["event_bus"].(map[string]any)
	entries, _ := eventBus["entries"].([]any)
	if len(entries) != 0 {
		t.Errorf("expected empty event_bus entries")
	}

	notifications, _ := resp["notifications"].(map[string]any)
	if notifications["unresolved_count"] != float64(0) {
		t.Errorf("expected unresolved_count=0, got %v", notifications["unresolved_count"])
	}
}

func TestAdminObservabilityDLQ_WithEntries(t *testing.T) {
	ts := time.Now().Add(-5 * time.Minute)
	eventDLQ := &stubDLQSvc{stats: []service.DLQStat{
		{EventType: "match.scored", Count: 3, OldestAt: &ts},
	}}
	notif := &stubNotifDLQRepo{count: 7}

	w := do(newObsDLQRouter(eventDLQ, notif), http.MethodGet, "/observability/dlq", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)

	eventBus, _ := resp["event_bus"].(map[string]any)
	entries, _ := eventBus["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 event bus entry, got %d", len(entries))
	}
	entry, _ := entries[0].(map[string]any)
	if entry["event_type"] != "match.scored" {
		t.Errorf("unexpected event_type: %v", entry["event_type"])
	}
	if entry["count"] != float64(3) {
		t.Errorf("unexpected count: %v", entry["count"])
	}
	if entry["oldest_at"] == nil {
		t.Errorf("expected oldest_at to be present")
	}

	notifications, _ := resp["notifications"].(map[string]any)
	if notifications["unresolved_count"] != float64(7) {
		t.Errorf("expected unresolved_count=7, got %v", notifications["unresolved_count"])
	}
}

func TestAdminObservabilityDLQ_EventBusError_GracefulDegradation(t *testing.T) {
	eventDLQ := &stubDLQSvc{err: errTestObs}
	notif := &stubNotifDLQRepo{count: 5}

	w := do(newObsDLQRouter(eventDLQ, notif), http.MethodGet, "/observability/dlq", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (graceful degradation), got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	notifications, _ := resp["notifications"].(map[string]any)
	if notifications["unresolved_count"] != float64(5) {
		t.Errorf("expected notification count even on event DLQ error")
	}
}

func TestAdminObservabilityDLQ_NoOldestAt(t *testing.T) {
	eventDLQ := &stubDLQSvc{stats: []service.DLQStat{
		{EventType: "test.event", Count: 1, OldestAt: nil},
	}}
	notif := &stubNotifDLQRepo{count: 0}

	w := do(newObsDLQRouter(eventDLQ, notif), http.MethodGet, "/observability/dlq", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	eventBus, _ := resp["event_bus"].(map[string]any)
	entries, _ := eventBus["entries"].([]any)
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry")
	}
	entry, _ := entries[0].(map[string]any)
	if _, hasOldestAt := entry["oldest_at"]; hasOldestAt {
		t.Errorf("expected no oldest_at when nil")
	}
}
