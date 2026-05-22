package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

// stubHubStater satisfies handler.HubStater with configurable return values.
type stubHubStater struct {
	connections int64
	broadcasts  int64
	dropped     int64
}

func (s *stubHubStater) Metrics() (int64, int64, int64) {
	return s.connections, s.broadcasts, s.dropped
}

func newSSEStatsRouter(hub handler.HubStater) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminSSEStatsHandler(hub, zap.NewNop())
	r.Get("/notifications/sse/stats", h.Stats)
	return r
}

func TestAdminSSEStats_Returns200WithCounters(t *testing.T) {
	stub := &stubHubStater{connections: 42, broadcasts: 1000, dropped: 3}
	w := do(newSSEStatsRouter(stub), http.MethodGet, "/notifications/sse/stats", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}

	var body struct {
		Connections int64 `json:"connections"`
		Broadcasts  int64 `json:"broadcasts"`
		Dropped     int64 `json:"dropped"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Connections != 42 {
		t.Errorf("connections: got %d, want 42", body.Connections)
	}
	if body.Broadcasts != 1000 {
		t.Errorf("broadcasts: got %d, want 1000", body.Broadcasts)
	}
	if body.Dropped != 3 {
		t.Errorf("dropped: got %d, want 3", body.Dropped)
	}
}

func TestAdminSSEStats_ZeroCounters_Returns200(t *testing.T) {
	stub := &stubHubStater{}
	w := do(newSSEStatsRouter(stub), http.MethodGet, "/notifications/sse/stats", "")
	if w.Code != http.StatusOK {
		t.Errorf(fmtExpect200, w.Code)
	}
}
