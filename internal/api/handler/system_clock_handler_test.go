package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/pkg/clock"
)

func TestSystemClockHandler_GetClock_ReturnsNowAsRFC3339(t *testing.T) {
	pinned := time.Date(2026, 7, 1, 20, 0, 0, 0, time.UTC)
	h := handler.NewSystemClockHandler(clock.Frozen{T: pinned}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/clock", nil)
	h.GetClock(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var body struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	got, err := time.Parse(time.RFC3339, body.Now)
	if err != nil {
		t.Fatalf("body.now %q is not RFC3339: %v", body.Now, err)
	}
	if !got.Equal(pinned) {
		t.Errorf("body.now = %v, want %v", got, pinned)
	}
}

func TestSystemClockHandler_GetClock_ContentTypeJSON(t *testing.T) {
	h := handler.NewSystemClockHandler(clock.Real{}, zap.NewNop())

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/clock", nil)
	h.GetClock(w, r)

	ct := w.Header().Get("Content-Type")
	if ct == "" {
		t.Error("Content-Type header is missing")
	}
}

func TestSystemClockHandler_GetClock_RealClock_InRange(t *testing.T) {
	h := handler.NewSystemClockHandler(clock.Real{}, zap.NewNop())

	before := time.Now().UTC().Truncate(time.Second)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/v1/system/clock", nil)
	h.GetClock(w, r)
	after := time.Now().UTC().Add(time.Second)

	var body struct {
		Now string `json:"now"`
	}
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	got, err := time.Parse(time.RFC3339, body.Now)
	if err != nil {
		t.Fatalf("body.now %q is not RFC3339: %v", body.Now, err)
	}
	if got.Before(before) || got.After(after) {
		t.Errorf("real-clock response %v not between %v and %v", got, before, after)
	}
}
