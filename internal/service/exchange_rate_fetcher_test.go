package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shopspring/decimal"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── helpers ──────────────────────────────────────────────────────────────────

// banguatXML returns a minimal valid Banguat dolares XML response with the
// provided venta (sell) and compra (buy) rate strings.
func banguatXML(venta, compra string) string {
	return `<?xml version="1.0" encoding="utf-8"?>
<VarDolares>
  <VarDolares>
    <fechaDate>2026-06-17T00:00:00</fechaDate>
    <compra>` + compra + `</compra>
    <venta>` + venta + `</venta>
  </VarDolares>
</VarDolares>`
}

// exchangeRateAPIJSON returns a minimal valid exchangerate-api.com JSON response.
func exchangeRateAPIJSON(rate float64) string {
	b, _ := json.Marshal(map[string]any{
		"result":          "success",
		"conversion_rate": rate,
	})
	return string(b)
}

// openExchangeJSON returns a minimal valid openexchangerates.org JSON response.
func openExchangeJSON(gtq float64) string {
	b, _ := json.Marshal(map[string]any{
		"rates": map[string]float64{"GTQ": gtq},
	})
	return string(b)
}

// serveOnce starts a test server that responds exactly once with the given
// status code and body, then returns a 500 for any subsequent request.
func serveOnce(t *testing.T, status int, body string) *httptest.Server {
	t.Helper()
	called := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if called {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		called = true
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

// stubFetcher is a test double for ExchangeRateFetcher.
type stubFetcher struct {
	sourceName string
	rate       *service.RawRate
	err        error
	calls      int
}

func (s *stubFetcher) Source() string { return s.sourceName }

func (s *stubFetcher) FetchCurrent(_ context.Context) (*service.RawRate, error) {
	s.calls++
	return s.rate, s.err
}

// ── BanguatFetcher ────────────────────────────────────────────────────────────

func TestBanguatFetcher_Source_ReturnsBanguat(t *testing.T) {
	f := service.NewBanguatFetcher()
	if f.Source() != "banguat" {
		t.Errorf("Source() = %q, want %q", f.Source(), "banguat")
	}
}

func TestBanguatFetcher_FetchCurrent_ValidXML_ReturnsVentaRate(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, banguatXML("7.75000", "7.74000"))
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	got, err := f.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.RequireFromString("7.75000")
	if !got.ReferenceRate.Equal(want) {
		t.Errorf("ReferenceRate = %s, want %s", got.ReferenceRate, want)
	}
	if got.Source != "banguat" {
		t.Errorf("Source = %q, want %q", got.Source, "banguat")
	}
	if got.FetchedAt.IsZero() {
		t.Error("FetchedAt must not be zero")
	}
}

func TestBanguatFetcher_FetchCurrent_NonOKStatus_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusServiceUnavailable, "")
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for non-200 status, got nil")
	}
	if !errorContains(err, "banguat") {
		t.Errorf("error %q should be prefixed with 'banguat:'", err)
	}
}

func TestBanguatFetcher_FetchCurrent_EmptyEntries_ReturnsError(t *testing.T) {
	emptyXML := `<?xml version="1.0" encoding="utf-8"?><VarDolares></VarDolares>`
	srv := serveOnce(t, http.StatusOK, emptyXML)
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for empty entries, got nil")
	}
}

func TestBanguatFetcher_FetchCurrent_MalformedXML_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, "not xml at all")
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected parse error for malformed XML, got nil")
	}
}

func TestBanguatFetcher_FetchCurrent_ZeroVenta_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, banguatXML("0.00000", "0.00000"))
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for zero venta rate, got nil")
	}
}

func TestBanguatFetcher_FetchCurrent_ContextCancelled_ReturnsError(t *testing.T) {
	// Server that blocks indefinitely — cancelled context must abort the call.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(ctx)
	if err == nil {
		t.Fatal("expected context cancellation error, got nil")
	}
}

// ── ExchangeRateAPIFetcher ────────────────────────────────────────────────────

func TestExchangeRateAPIFetcher_Source_ReturnsName(t *testing.T) {
	f := service.NewExchangeRateAPIFetcher("key")
	if f.Source() != "exchangerate-api" {
		t.Errorf("Source() = %q, want %q", f.Source(), "exchangerate-api")
	}
}

func TestExchangeRateAPIFetcher_NoAPIKey_ReturnsError(t *testing.T) {
	f := service.NewExchangeRateAPIFetcher("")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for missing API key, got nil")
	}
	if !errorContains(err, "not configured") {
		t.Errorf("error %q should mention 'not configured'", err)
	}
}

func TestExchangeRateAPIFetcher_FetchCurrent_ValidJSON_ReturnsRate(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, exchangeRateAPIJSON(7.8500))
	defer srv.Close()

	f := service.NewExchangeRateAPIFetcherWithURL("testkey", srv.URL+"/%s/pair/USD/GTQ")
	got, err := f.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.NewFromFloat(7.85)
	if !got.ReferenceRate.Equal(want) {
		t.Errorf("ReferenceRate = %s, want %s", got.ReferenceRate, want)
	}
	if got.Source != "exchangerate-api" {
		t.Errorf("Source = %q, want %q", got.Source, "exchangerate-api")
	}
}

func TestExchangeRateAPIFetcher_FetchCurrent_ErrorResult_ReturnsError(t *testing.T) {
	body, _ := json.Marshal(map[string]any{
		"result":     "error",
		"error-type": "invalid-key",
	})
	srv := serveOnce(t, http.StatusOK, string(body))
	defer srv.Close()

	f := service.NewExchangeRateAPIFetcherWithURL("testkey", srv.URL+"/%s/pair/USD/GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected provider error, got nil")
	}
	if !errorContains(err, "exchangerate-api") {
		t.Errorf("error %q should be prefixed with 'exchangerate-api:'", err)
	}
}

func TestExchangeRateAPIFetcher_FetchCurrent_NonOKStatus_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusUnauthorized, "")
	defer srv.Close()

	f := service.NewExchangeRateAPIFetcherWithURL("testkey", srv.URL+"/%s/pair/USD/GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for 401 status, got nil")
	}
}

// ── OpenExchangeFetcher ───────────────────────────────────────────────────────

func TestOpenExchangeFetcher_Source_ReturnsName(t *testing.T) {
	f := service.NewOpenExchangeFetcher("appid")
	if f.Source() != "openexchangerates" {
		t.Errorf("Source() = %q, want %q", f.Source(), "openexchangerates")
	}
}

func TestOpenExchangeFetcher_NoAppID_ReturnsError(t *testing.T) {
	f := service.NewOpenExchangeFetcher("")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for missing app ID, got nil")
	}
	if !errorContains(err, "not configured") {
		t.Errorf("error %q should mention 'not configured'", err)
	}
}

func TestOpenExchangeFetcher_FetchCurrent_ValidJSON_ReturnsGTQRate(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, openExchangeJSON(7.9200))
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	got, err := f.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.NewFromFloat(7.92)
	if !got.ReferenceRate.Equal(want) {
		t.Errorf("ReferenceRate = %s, want %s", got.ReferenceRate, want)
	}
	if got.Source != "openexchangerates" {
		t.Errorf("Source = %q, want %q", got.Source, "openexchangerates")
	}
}

func TestOpenExchangeFetcher_FetchCurrent_MissingGTQ_ReturnsError(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"rates": map[string]float64{"USD": 1.0}})
	srv := serveOnce(t, http.StatusOK, string(body))
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error when GTQ is absent, got nil")
	}
}

func TestOpenExchangeFetcher_FetchCurrent_ErrorFlag_ReturnsError(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"error": true, "message": "invalid_app_id"})
	srv := serveOnce(t, http.StatusOK, string(body))
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected provider error, got nil")
	}
}

func TestBanguatFetcher_FetchCurrent_EmptyVentaField_ReturnsError(t *testing.T) {
	xml := `<?xml version="1.0" encoding="utf-8"?>
<VarDolares>
  <VarDolares>
    <fechaDate>2026-06-17T00:00:00</fechaDate>
    <compra>7.74</compra>
    <venta></venta>
  </VarDolares>
</VarDolares>`
	srv := serveOnce(t, http.StatusOK, xml)
	defer srv.Close()

	f := service.NewBanguatFetcherWithURL(srv.URL)
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for empty venta field, got nil")
	}
}

func TestExchangeRateAPIFetcher_MalformedJSON_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, "not valid json {{{")
	defer srv.Close()

	f := service.NewExchangeRateAPIFetcherWithURL("testkey", srv.URL+"/%s/pair/USD/GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !errorContains(err, "exchangerate-api") {
		t.Errorf("error %q should be prefixed with 'exchangerate-api:'", err)
	}
}

func TestExchangeRateAPIFetcher_ZeroConversionRate_ReturnsError(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"result": "success", "conversion_rate": 0.0})
	srv := serveOnce(t, http.StatusOK, string(body))
	defer srv.Close()

	f := service.NewExchangeRateAPIFetcherWithURL("testkey", srv.URL+"/%s/pair/USD/GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for zero conversion rate, got nil")
	}
}

func TestOpenExchangeFetcher_MalformedJSON_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusOK, "not valid json {{{")
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
}

func TestOpenExchangeFetcher_ZeroGTQRate_ReturnsError(t *testing.T) {
	body, _ := json.Marshal(map[string]any{"rates": map[string]float64{"GTQ": 0.0}})
	srv := serveOnce(t, http.StatusOK, string(body))
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for zero GTQ rate, got nil")
	}
}

func TestOpenExchangeFetcher_NonOKStatus_ReturnsError(t *testing.T) {
	srv := serveOnce(t, http.StatusForbidden, "")
	defer srv.Close()

	f := service.NewOpenExchangeFetcherWithURL("appid", srv.URL+"?app_id=%s&symbols=GTQ")
	_, err := f.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error for 403 status, got nil")
	}
}

// ── MultiSourceFetcher ────────────────────────────────────────────────────────

func TestMultiSourceFetcher_UsesPrimaryOnSuccess(t *testing.T) {
	want := decimal.RequireFromString("7.7500")
	primary := &stubFetcher{
		sourceName: "banguat",
		rate:       &service.RawRate{ReferenceRate: want, Source: "banguat"},
	}
	secondary := &stubFetcher{sourceName: "exchangerate-api", err: errors.New("should not be called")}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), primary, secondary)
	got, err := msf.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.ReferenceRate.Equal(want) {
		t.Errorf("ReferenceRate = %s, want %s", got.ReferenceRate, want)
	}
	if secondary.calls != 0 {
		t.Errorf("secondary fetcher should not have been called, got %d calls", secondary.calls)
	}
}

func TestMultiSourceFetcher_FallsBackOnPrimaryFailure(t *testing.T) {
	want := decimal.RequireFromString("7.8000")
	primary := &stubFetcher{sourceName: "banguat", err: errors.New("banguat: timeout")}
	secondary := &stubFetcher{
		sourceName: "exchangerate-api",
		rate:       &service.RawRate{ReferenceRate: want, Source: "exchangerate-api"},
	}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), primary, secondary)
	got, err := msf.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.ReferenceRate.Equal(want) {
		t.Errorf("ReferenceRate = %s, want %s", got.ReferenceRate, want)
	}
	if primary.calls != 1 {
		t.Errorf("primary should have been called once, got %d", primary.calls)
	}
	if secondary.calls != 1 {
		t.Errorf("secondary should have been called once, got %d", secondary.calls)
	}
}

func TestMultiSourceFetcher_AllSourcesFail_ReturnsAggregateError(t *testing.T) {
	p := &stubFetcher{sourceName: "banguat", err: errors.New("banguat: timeout")}
	s := &stubFetcher{sourceName: "exchangerate-api", err: errors.New("exchangerate-api: 401")}
	q := &stubFetcher{sourceName: "openexchangerates", err: errors.New("openexchangerates: 403")}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), p, s, q)
	_, err := msf.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected aggregate error when all sources fail, got nil")
	}
	if !errorContains(err, "all exchange rate sources failed") {
		t.Errorf("error %q should mention 'all exchange rate sources failed'", err)
	}
}

func TestMultiSourceFetcher_ShortCircuitsAfterFirstSuccess(t *testing.T) {
	want := decimal.RequireFromString("7.6000")
	first := &stubFetcher{
		sourceName: "banguat",
		rate:       &service.RawRate{ReferenceRate: want, Source: "banguat"},
	}
	second := &stubFetcher{sourceName: "exchangerate-api", err: errors.New("should not run")}
	third := &stubFetcher{sourceName: "openexchangerates", err: errors.New("should not run")}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), first, second, third)
	_, err := msf.FetchCurrent(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.calls != 0 || third.calls != 0 {
		t.Errorf("sources after success must not be called: second=%d, third=%d", second.calls, third.calls)
	}
}

func TestMultiSourceFetcher_PanicsOnEmptySourceList(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for empty source list, got none")
		}
	}()
	service.NewMultiSourceFetcher(zaptest.NewLogger(t)) // no sources — must panic
}

func TestMultiSourceFetcher_SingleSource_PropagatesError(t *testing.T) {
	only := &stubFetcher{sourceName: "banguat", err: errors.New("banguat: network error")}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), only)
	_, err := msf.FetchCurrent(context.Background())
	if err == nil {
		t.Fatal("expected error from single-source failure, got nil")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func errorContains(err error, substr string) bool {
	if err == nil {
		return false
	}
	return containsString(err.Error(), substr)
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > 0 && searchString(s, substr))
}

func searchString(s, substr string) bool {
	for i := range s {
		if i+len(substr) <= len(s) && s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
