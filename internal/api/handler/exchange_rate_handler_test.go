package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

// ── router helper ─────────────────────────────────────────────────────────────

func newPublicFXRouter(svc *stubFXSvc) http.Handler {
	r := chi.NewRouter()
	h := handler.NewExchangeRateHandler(svc, zap.NewNop())
	r.Get("/exchange-rate", h.GetCurrent)
	return r
}

// ── GetCurrent ────────────────────────────────────────────────────────────────

func TestExchangeRateHandler_GetCurrent_Returns200(t *testing.T) {
	router := newPublicFXRouter(&stubFXSvc{rates: sampleRates()})
	w := do(router, http.MethodGet, "/exchange-rate", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExchangeRateHandler_GetCurrent_ServiceError_Returns500(t *testing.T) {
	router := newPublicFXRouter(&stubFXSvc{err: errors.New("cache cold and DB unreachable")})
	w := do(router, http.MethodGet, "/exchange-rate", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestExchangeRateHandler_GetCurrent_DoesNotExposeReferenceRateOrMargins(t *testing.T) {
	router := newPublicFXRouter(&stubFXSvc{rates: sampleRates()})
	w := do(router, http.MethodGet, "/exchange-rate", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Fields that must be present.
	for _, required := range []string{"buy_rate", "sell_rate", "effective_at", "stale"} {
		if _, ok := resp[required]; !ok {
			t.Errorf("response missing required field %q", required)
		}
	}
	// Fields that must NOT be exposed to the public.
	for _, hidden := range []string{"reference_rate", "buy_margin_pct", "sell_margin_pct"} {
		if _, ok := resp[hidden]; ok {
			t.Errorf("response must not expose internal field %q", hidden)
		}
	}
}

func TestExchangeRateHandler_GetCurrent_StaleFlag_Propagated(t *testing.T) {
	rates := sampleRates()
	rates.Stale = true

	router := newPublicFXRouter(&stubFXSvc{rates: rates})
	w := do(router, http.MethodGet, "/exchange-rate", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["stale"] != true {
		t.Errorf("expected stale=true in response, got %v", resp["stale"])
	}
}
