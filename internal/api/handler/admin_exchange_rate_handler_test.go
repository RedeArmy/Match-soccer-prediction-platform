package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubFXSvc struct {
	rates *domain.ExchangeRates
	err   error
}

func (s *stubFXSvc) RefreshRate(_ context.Context) (*domain.ExchangeRates, error) {
	return s.rates, s.err
}
func (s *stubFXSvc) GetCurrentRates(_ context.Context) (*domain.ExchangeRates, error) {
	return s.rates, s.err
}
func (s *stubFXSvc) OverrideRate(_ context.Context, _ decimal.Decimal, _ string, _ int) (*domain.ExchangeRates, error) {
	return s.rates, s.err
}
func (s *stubFXSvc) ConvertUSDToGTQ(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return 0, decimal.Zero, s.err
}
func (s *stubFXSvc) ConvertGTQToUSD(_ context.Context, _ int64) (int64, decimal.Decimal, error) {
	return 0, decimal.Zero, s.err
}
func (s *stubFXSvc) WarmCache(_ context.Context) error { return s.err }

type stubFXHistoryRepo struct {
	records []*domain.ExchangeRateRecord
	err     error
}

func (r *stubFXHistoryRepo) Save(_ context.Context, _ *domain.ExchangeRateRecord) error {
	return r.err
}
func (r *stubFXHistoryRepo) GetLatest(_ context.Context) (*domain.ExchangeRateRecord, error) {
	if r.err != nil {
		return nil, r.err
	}
	if len(r.records) == 0 {
		return nil, nil
	}
	return r.records[0], nil
}
func (r *stubFXHistoryRepo) GetHistory(_ context.Context, _, _ int) ([]*domain.ExchangeRateRecord, error) {
	return r.records, r.err
}
func (r *stubFXHistoryRepo) GetOverrides(_ context.Context, _ int) ([]*domain.ExchangeRateRecord, error) {
	return r.records, r.err
}

type stubFXAudit struct{}

func (s *stubFXAudit) Log(_ context.Context, _ *int, _ *domain.UserRole, _ string, _ *string, _ *int, _ map[string]any) {
}

// ── router helpers ────────────────────────────────────────────────────────────

func newAdminFXRouter(svc service.ExchangeRateService, repo repository.ExchangeRateRepository) http.Handler {
	r := chi.NewRouter()
	h := handler.NewAdminExchangeRateHandler(svc, repo, &stubFXAudit{}, zap.NewNop())
	r.Get("/exchange-rate/current", h.GetCurrent)
	r.Get("/exchange-rate/history", h.GetHistory)
	r.Post("/exchange-rate/override", h.Override)
	r.Post("/exchange-rate/refresh", h.Refresh)
	return r
}

func sampleRates() *domain.ExchangeRates {
	return &domain.ExchangeRates{
		ReferenceRate: decimal.RequireFromString("7.7500"),
		BuyRate:       decimal.RequireFromString("7.63375"),
		SellRate:      decimal.RequireFromString("7.905"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		EffectiveAt:   time.Now(),
	}
}

// ── GetCurrent ────────────────────────────────────────────────────────────────

func TestAdminExchangeRateHandler_GetCurrent_Returns200WithRates(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	w := do(router, http.MethodGet, "/exchange-rate/current", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp handler.ExchangeRatesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ReferenceRate != "7.75" {
		t.Errorf("reference_rate: want 7.75, got %s", resp.ReferenceRate)
	}
	if resp.Source != "banguat" {
		t.Errorf("source: want banguat, got %s", resp.Source)
	}
}

func TestAdminExchangeRateHandler_GetCurrent_ServiceError_Returns500(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{err: errors.New("cache cold, DB unreachable")}, &stubFXHistoryRepo{})
	w := do(router, http.MethodGet, "/exchange-rate/current", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetHistory ────────────────────────────────────────────────────────────────

func TestAdminExchangeRateHandler_GetHistory_Returns200WithArray(t *testing.T) {
	records := []*domain.ExchangeRateRecord{
		{
			ID:            1,
			ReferenceRate: decimal.RequireFromString("7.75"),
			BuyRate:       decimal.RequireFromString("7.63375"),
			SellRate:      decimal.RequireFromString("7.905"),
			BuyMarginPct:  decimal.RequireFromString("0.015"),
			SellMarginPct: decimal.RequireFromString("0.020"),
			Source:        "banguat",
			EffectiveAt:   time.Now(),
			FetchedAt:     time.Now(),
		},
	}
	router := newAdminFXRouter(&stubFXSvc{}, &stubFXHistoryRepo{records: records})
	w := do(router, http.MethodGet, "/exchange-rate/history", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp []handler.ExchangeRateRecordResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp) != 1 {
		t.Errorf("expected 1 record, got %d", len(resp))
	}
	if resp[0].Source != "banguat" {
		t.Errorf("source: want banguat, got %s", resp[0].Source)
	}
}

func TestAdminExchangeRateHandler_GetHistory_EmptyTable_Returns200WithEmptyArray(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{}, &stubFXHistoryRepo{records: nil})
	w := do(router, http.MethodGet, "/exchange-rate/history", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp []handler.ExchangeRateRecordResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if len(resp) != 0 {
		t.Errorf("expected empty array, got %d records", len(resp))
	}
}

func TestAdminExchangeRateHandler_GetHistory_RepoError_Returns500(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{}, &stubFXHistoryRepo{err: errors.New("db timeout")})
	w := do(router, http.MethodGet, "/exchange-rate/history", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Override ──────────────────────────────────────────────────────────────────

func TestAdminExchangeRateHandler_Override_Success_Returns200(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"7.75","reason":"adjusted after bank holiday closure"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAdminExchangeRateHandler_Override_MissingReason_Returns400(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"7.75","reason":""}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 for empty reason, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_ShortReason_Returns400(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"7.75","reason":"too short"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 for reason shorter than 10 chars, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_InvalidRate_Returns400(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"not-a-number","reason":"adjusted for weekend trading window"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 for invalid rate, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_RateOutOfRange_Returns400(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"25.0","reason":"rate is above the maximum allowed value"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 for rate > 20, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_NoCaller_Returns401(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"7.75","reason":"adjusted for weekend trading window"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	// Deliberately omit withCaller — no user in context.
	w := doReq(router, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 without caller, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_InvalidJSON_Returns400(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", "{bad json")
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 400 for malformed JSON, got %d", w.Code)
	}
}

func TestAdminExchangeRateHandler_Override_ServiceError_Returns500(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{err: errors.New("db: connection reset")}, &stubFXHistoryRepo{})
	body := `{"reference_rate":"7.75","reason":"adjusted for weekend trading window"}`
	req := newAdminRequestJSON(http.MethodPost, "/exchange-rate/override", body)
	req = withCaller(req, adminCaller)
	w := doReq(router, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", w.Code)
	}
}

// ── Refresh ───────────────────────────────────────────────────────────────────

func TestAdminExchangeRateHandler_Refresh_Returns200(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{rates: sampleRates()}, &stubFXHistoryRepo{})
	w := do(router, http.MethodPost, "/exchange-rate/refresh", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp handler.ExchangeRatesResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Source != "banguat" {
		t.Errorf("source: want banguat, got %s", resp.Source)
	}
}

func TestAdminExchangeRateHandler_Refresh_ServiceError_Returns500(t *testing.T) {
	router := newAdminFXRouter(&stubFXSvc{err: errors.New("all sources failed")}, &stubFXHistoryRepo{})
	w := do(router, http.MethodPost, "/exchange-rate/refresh", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
