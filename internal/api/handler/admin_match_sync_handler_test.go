package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// ── stub ──────────────────────────────────────────────────────────────────────

type stubMatchSyncer struct {
	linkErr       error
	unlinkErr     error
	pollLiveCount int
	pollErr       error
	reconcileDiff []service.SyncDiff
	reconcileErr  error
}

func (s *stubMatchSyncer) LinkExternal(_ context.Context, _ int, _ string, _ int64) error {
	return s.linkErr
}
func (s *stubMatchSyncer) UnlinkExternal(_ context.Context, _ int) error { return s.unlinkErr }
func (s *stubMatchSyncer) PollAndApply(_ context.Context) (int, error) {
	return s.pollLiveCount, s.pollErr
}
func (s *stubMatchSyncer) ReconcileDate(_ context.Context, _, _ int) ([]service.SyncDiff, error) {
	return s.reconcileDiff, s.reconcileErr
}

func newSyncHandler(svc *stubMatchSyncer) *handler.AdminMatchSyncHandler {
	return handler.NewAdminMatchSyncHandler(svc, zap.NewNop())
}

func chiRoute(h http.Handler, pattern, method, url string) *httptest.ResponseRecorder {
	r := chi.NewRouter()
	r.Method(method, pattern, h)
	req := httptest.NewRequest(method, url, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ── LinkExternal ──────────────────────────────────────────────────────────────

func TestAdminMatchSync_LinkExternal_Success(t *testing.T) {
	svc := &stubMatchSyncer{}
	h := newSyncHandler(svc)

	body, _ := json.Marshal(map[string]any{"provider": "api-football", "external_id": 867234})
	r := chi.NewRouter()
	r.Post("/admin/matches/{id}/external-link", h.LinkExternal)
	req := httptest.NewRequest(http.MethodPost, "/admin/matches/42/external-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200 — body: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["match_id"] != float64(42) {
		t.Errorf("match_id: got %v, want 42", resp["match_id"])
	}
}

func TestAdminMatchSync_LinkExternal_ValidationError_Returns422(t *testing.T) {
	svc := &stubMatchSyncer{linkErr: apperrors.Validation("unsupported provider")}
	h := newSyncHandler(svc)

	body, _ := json.Marshal(map[string]any{"provider": "unknown", "external_id": 1})
	r := chi.NewRouter()
	r.Post("/admin/matches/{id}/external-link", h.LinkExternal)
	req := httptest.NewRequest(http.MethodPost, "/admin/matches/1/external-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("status: got %d, want 422", w.Code)
	}
}

func TestAdminMatchSync_LinkExternal_NotFound_Returns404(t *testing.T) {
	svc := &stubMatchSyncer{linkErr: apperrors.NotFound("match not found")}
	h := newSyncHandler(svc)

	body, _ := json.Marshal(map[string]any{"provider": "api-football", "external_id": 99})
	r := chi.NewRouter()
	r.Post("/admin/matches/{id}/external-link", h.LinkExternal)
	req := httptest.NewRequest(http.MethodPost, "/admin/matches/999/external-link", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

// ── UnlinkExternal ────────────────────────────────────────────────────────────

func TestAdminMatchSync_UnlinkExternal_Success_Returns204(t *testing.T) {
	h := newSyncHandler(&stubMatchSyncer{})

	r := chi.NewRouter()
	r.Delete("/admin/matches/{id}/external-link", h.UnlinkExternal)
	req := httptest.NewRequest(http.MethodDelete, "/admin/matches/5/external-link", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status: got %d, want 204", w.Code)
	}
}

// ── TriggerPoll ───────────────────────────────────────────────────────────────

func TestAdminMatchSync_TriggerPoll_Success_ReturnsLiveCount(t *testing.T) {
	h := newSyncHandler(&stubMatchSyncer{pollLiveCount: 3})

	w := chiRoute(http.HandlerFunc(h.TriggerPoll), "/poll", http.MethodPost, "/poll")
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["live_count"] != float64(3) {
		t.Errorf("live_count: got %v, want 3", resp["live_count"])
	}
}

func TestAdminMatchSync_TriggerPoll_ProviderError_Returns500(t *testing.T) {
	h := newSyncHandler(&stubMatchSyncer{pollErr: errors.New("provider unavailable")})

	w := chiRoute(http.HandlerFunc(h.TriggerPoll), "/poll", http.MethodPost, "/poll")
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

// ── Reconcile ─────────────────────────────────────────────────────────────────

func TestAdminMatchSync_Reconcile_NoDiffs_ReturnsEmptyArray(t *testing.T) {
	h := newSyncHandler(&stubMatchSyncer{reconcileDiff: nil})

	w := chiRoute(http.HandlerFunc(h.Reconcile), "/reconcile", http.MethodGet, "/reconcile")
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var items []any
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 0 {
		t.Errorf("expected empty array, got %d items", len(items))
	}
}

func TestAdminMatchSync_Reconcile_WithDiffs_ReturnsItems(t *testing.T) {
	home, away := 2, 1
	diffs := []service.SyncDiff{
		{MatchID: 7, InternalHome: &home, InternalAway: &away, ExternalHome: 2, ExternalAway: 1},
	}
	h := newSyncHandler(&stubMatchSyncer{reconcileDiff: diffs})

	w := chiRoute(http.HandlerFunc(h.Reconcile), "/reconcile", http.MethodGet, "/reconcile?league_id=1&season=2026")
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var items []map[string]any
	_ = json.NewDecoder(w.Body).Decode(&items)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	if items[0]["match_id"] != float64(7) {
		t.Errorf("match_id: got %v, want 7", items[0]["match_id"])
	}
}
