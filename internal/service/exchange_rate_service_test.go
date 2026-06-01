package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// ── stub implementations ──────────────────────────────────────────────────────

// stubParamService implements ExchangeRateParamRW for testing.
type stubParamService struct {
	ints   map[string]int
	setKey string
	setVal string
	setErr error
}

func newStubParamService(ints map[string]int) *stubParamService {
	if ints == nil {
		ints = map[string]int{}
	}
	return &stubParamService{ints: ints}
}

func (s *stubParamService) GetInt(_ context.Context, key string, def int) int {
	if v, ok := s.ints[key]; ok {
		return v
	}
	return def
}

func (s *stubParamService) Set(_ context.Context, key, value string, _ int) (*domain.SystemParam, error) {
	s.setKey = key
	s.setVal = value
	return &domain.SystemParam{Key: key, Value: value}, s.setErr
}

// stubFXRepo implements ExchangeRateRepository for testing.
type stubFXRepo struct {
	latest  *domain.ExchangeRateRecord
	saved   []*domain.ExchangeRateRecord
	saveErr error
}

func (r *stubFXRepo) Save(_ context.Context, rec *domain.ExchangeRateRecord) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saved = append(r.saved, rec)
	return nil
}

func (r *stubFXRepo) GetLatest(_ context.Context) (*domain.ExchangeRateRecord, error) {
	return r.latest, nil
}

func (r *stubFXRepo) GetHistory(_ context.Context, _, _ int) ([]*domain.ExchangeRateRecord, error) {
	if r.latest == nil {
		return nil, nil
	}
	return []*domain.ExchangeRateRecord{r.latest}, nil
}

func (r *stubFXRepo) GetOverrides(_ context.Context, _ int) ([]*domain.ExchangeRateRecord, error) {
	return nil, nil
}

// stubStaleNotifier captures stale-rate alert calls.
type stubStaleNotifier struct {
	called   bool
	lastRate string
	ageHours float64
	source   string
}

func (n *stubStaleNotifier) NotifyFXRateStale(_ context.Context, lastRate string, ageHours float64, source string) {
	n.called = true
	n.lastRate = lastRate
	n.ageHours = ageHours
	n.source = source
}

// rawRate constructs a RawRate for use in stubFetcher.
func rawRate(rateStr, source string) *service.RawRate {
	return &service.RawRate{
		ReferenceRate: decimal.RequireFromString(rateStr),
		Source:        source,
		FetchedAt:     time.Now(),
	}
}

// ── TestMarginEngine ──────────────────────────────────────────────────────────

func TestMarginEngine_ComputesBuyRate(t *testing.T) {
	// reference=7.75, buy_margin=150 bps (1.5%) → buy=7.75×0.985=7.63625
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.RequireFromString("7.63375") // 7.75 × 0.985 = 7.75 - 0.11625
	if !rates.BuyRate.Equal(want) {
		t.Errorf("BuyRate = %s, want %s", rates.BuyRate, want)
	}
}

func TestMarginEngine_ComputesSellRate(t *testing.T) {
	// reference=7.75, sell_margin=200 bps (2.0%) → sell=7.75×1.020=7.905
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.RequireFromString("7.905") // 7.75 × 1.020
	if !rates.SellRate.Equal(want) {
		t.Errorf("SellRate = %s, want %s", rates.SellRate, want)
	}
}

func TestMarginEngine_NeverUsesFloat64(t *testing.T) {
	// Verify that fractional basis points produce correct decimal results
	// and do not suffer float64 rounding: 7.7500 × 0.9985 = 7.738875
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  15, // 0.15%
		domain.ParamKeyFXSellMarginBPS: 0,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := decimal.RequireFromString("7.738375") // 7.75 × 0.9985 = 7.75 - 0.011625
	if !rates.BuyRate.Equal(want) {
		t.Errorf("BuyRate = %s, want %s (possible float64 precision loss)", rates.BuyRate, want)
	}
}

// ── TestRefreshRate ───────────────────────────────────────────────────────────

func TestRefreshRate_PersistsToHistory(t *testing.T) {
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	_, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 history row, got %d", len(repo.saved))
	}
	rec := repo.saved[0]
	if rec.IsOverride {
		t.Error("automated refresh must not set is_override=true")
	}
	if rec.Stale {
		t.Error("successful fetch must not be stale")
	}
	if rec.Source != "banguat" {
		t.Errorf("Source = %q, want %q", rec.Source, "banguat")
	}
}

func TestRefreshRate_WritesUSDGTQRateParam(t *testing.T) {
	// sell_rate = 7.75 × 1.02 = 7.905 → centavos = floor(790.5) = 790
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	_, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if params.setKey != domain.ParamKeyUSDGTQRate {
		t.Errorf("Set key = %q, want %q", params.setKey, domain.ParamKeyUSDGTQRate)
	}
	if params.setVal != "790" {
		t.Errorf("Set value = %q, want %q (floor(7.905×100)=790)", params.setVal, "790")
	}
}

func TestRefreshRate_UsesLastKnownOnAllFailure(t *testing.T) {
	// All fetchers fail → fall back to last known rate in repo.
	primary := &stubFetcher{sourceName: "banguat", err: errors.New("timeout")}
	secondary := &stubFetcher{sourceName: "exchangerate-api", err: errors.New("401")}
	tertiary := &stubFetcher{sourceName: "openexchangerates", err: errors.New("403")}

	lastKnown := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.7500"),
		BuyRate:       decimal.RequireFromString("7.6338"),
		SellRate:      decimal.RequireFromString("7.9050"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		FetchedAt:     time.Now().Add(-2 * time.Hour),
		EffectiveAt:   time.Now().Add(-2 * time.Hour),
	}
	repo := &stubFXRepo{latest: lastKnown}
	params := newStubParamService(nil)
	notifier := &stubStaleNotifier{}

	msf := service.NewMultiSourceFetcher(zaptest.NewLogger(t), primary, secondary, tertiary)
	svc := service.NewExchangeRateService(msf, repo, params, notifier, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rates.Stale {
		t.Error("expected stale=true when all sources fail")
	}
	if rates.Source != "stale" {
		t.Errorf("Source = %q, want %q", rates.Source, "stale")
	}
	if !notifier.called {
		t.Error("expected stale notifier to be called")
	}
}

func TestRefreshRate_DoesNotPanicOnAllFailure(t *testing.T) {
	// Even when all sources fail AND the DB is empty, the call must return an
	// error, not panic. This ensures the scheduler job never crashes the worker.
	primary := &stubFetcher{sourceName: "banguat", err: errors.New("timeout")}
	repo := &stubFXRepo{latest: nil} // empty history table
	params := newStubParamService(nil)

	svc := service.NewExchangeRateService(primary, repo, params, nil, zaptest.NewLogger(t))

	defer func() {
		if r := recover(); r != nil {
			t.Errorf("RefreshRate must not panic: %v", r)
		}
	}()

	_, err := svc.RefreshRate(context.Background())
	if err == nil {
		t.Error("expected error when all sources fail and DB is empty")
	}
}

// ── TestOverrideRate ──────────────────────────────────────────────────────────

func TestOverrideRate_RequiresReason(t *testing.T) {
	repo := &stubFXRepo{}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	_, err := svc.OverrideRate(context.Background(), decimal.RequireFromString("7.75"), "", 1)
	if err == nil {
		t.Fatal("expected error for empty reason, got nil")
	}
	if !errorContains(err, "reason") {
		t.Errorf("error %q should mention 'reason'", err)
	}
}

func TestOverrideRate_RejectsOutOfRangeRate(t *testing.T) {
	repo := &stubFXRepo{}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	// Rate above 20 is unrealistic for GTQ/USD.
	_, err := svc.OverrideRate(context.Background(), decimal.RequireFromString("25.00"), "test reason for override", 1)
	if err == nil {
		t.Fatal("expected error for reference_rate=25 (out of range), got nil")
	}
}

func TestOverrideRate_AuditsIsOverrideFlag(t *testing.T) {
	repo := &stubFXRepo{}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	_, err := svc.OverrideRate(context.Background(), decimal.RequireFromString("7.75"), "manual rate adjustment for holiday weekend", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(repo.saved) != 1 {
		t.Fatalf("expected 1 saved row, got %d", len(repo.saved))
	}
	rec := repo.saved[0]
	if !rec.IsOverride {
		t.Error("expected is_override=true for admin override")
	}
	if rec.OverrideBy == nil || *rec.OverrideBy != 42 {
		t.Errorf("expected override_by=42, got %v", rec.OverrideBy)
	}
	if rec.OverrideReason != "manual rate adjustment for holiday weekend" {
		t.Errorf("override_reason = %q, want exact string", rec.OverrideReason)
	}
}

func TestOverrideRate_InvalidatesCache(t *testing.T) {
	// GetCurrentRates returns cached data; Override must replace it.
	oldRecord := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.00"),
		BuyRate:       decimal.RequireFromString("6.895"),
		SellRate:      decimal.RequireFromString("7.14"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		EffectiveAt:   time.Now().Add(-30 * time.Minute),
		FetchedAt:     time.Now().Add(-30 * time.Minute),
	}
	repo := &stubFXRepo{latest: oldRecord}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	// Warm cache with old rate.
	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache: %v", err)
	}

	// Override with new rate.
	newRef := decimal.RequireFromString("7.90")
	_, err := svc.OverrideRate(context.Background(), newRef, "adjusted for weekend trading session", 1)
	if err != nil {
		t.Fatalf("OverrideRate: %v", err)
	}

	// GetCurrentRates must now return the overridden rate.
	got, err := svc.GetCurrentRates(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentRates: %v", err)
	}
	if !got.ReferenceRate.Equal(newRef) {
		t.Errorf("after override: ReferenceRate = %s, want %s", got.ReferenceRate, newRef)
	}
}

// ── TestConvertUSDToGTQ / TestConvertGTQToUSD ─────────────────────────────────

func TestConvertUSDToGTQ_UsesBuyRate(t *testing.T) {
	// BuyRate = 7.63625 (7.75 × 0.985)
	// 1000 USD cents = $10.00 → 10 × 7.63625 = 76.3625 GTQ → 7636 GTQ cents
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	if _, err := svc.RefreshRate(context.Background()); err != nil {
		t.Fatalf("RefreshRate: %v", err)
	}

	gtqCents, rate, err := svc.ConvertUSDToGTQ(context.Background(), 1000)
	if err != nil {
		t.Fatalf("ConvertUSDToGTQ: %v", err)
	}

	wantRate := decimal.RequireFromString("7.63375")
	if !rate.Equal(wantRate) {
		t.Errorf("rate = %s, want %s (buy rate)", rate, wantRate)
	}
	wantGTQ := int64(7633) // floor(1000 × 7.63375) = floor(7633.75) = 7633
	if gtqCents != wantGTQ {
		t.Errorf("gtqCents = %d, want %d", gtqCents, wantGTQ)
	}
}

func TestConvertGTQToUSD_UsesSellRate(t *testing.T) {
	// SellRate = 7.905 (7.75 × 1.02)
	// 7905 GTQ cents = Q79.05 → 79.05 / 7.905 = 10.0 USD → 1000 USD cents
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	if _, err := svc.RefreshRate(context.Background()); err != nil {
		t.Fatalf("RefreshRate: %v", err)
	}

	usdCents, rate, err := svc.ConvertGTQToUSD(context.Background(), 7905)
	if err != nil {
		t.Fatalf("ConvertGTQToUSD: %v", err)
	}

	wantRate := decimal.RequireFromString("7.905")
	if !rate.Equal(wantRate) {
		t.Errorf("rate = %s, want %s (sell rate)", rate, wantRate)
	}
	if usdCents != 1000 {
		t.Errorf("usdCents = %d, want 1000", usdCents)
	}
}

// ── TestGetCurrentRates (cache) ───────────────────────────────────────────────

func TestGetCurrentRates_ReturnsCachedRateWithoutDBHit(t *testing.T) {
	// After RefreshRate, GetCurrentRates must not hit the DB.
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	dbHits := 0
	repo := &stubbedCountingFXRepo{hits: &dbHits}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	if _, err := svc.RefreshRate(context.Background()); err != nil {
		t.Fatalf("RefreshRate: %v", err)
	}

	dbHitsBeforeGet := dbHits
	_, err := svc.GetCurrentRates(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentRates: %v", err)
	}

	if dbHits != dbHitsBeforeGet {
		t.Errorf("GetCurrentRates hit DB %d times after cache warm; expected 0", dbHits-dbHitsBeforeGet)
	}
}

// stubbedCountingFXRepo counts GetLatest calls.
type stubbedCountingFXRepo struct {
	stubFXRepo
	hits *int
}

func (r *stubbedCountingFXRepo) GetLatest(_ context.Context) (*domain.ExchangeRateRecord, error) {
	*r.hits++
	return nil, nil
}

// ── TestRegisterMetrics ───────────────────────────────────────────────────────

func TestRegisterMetrics_WithNoopMeter_DoesNotError(t *testing.T) {
	repo := &stubFXRepo{}
	params := newStubParamService(nil)
	impl := service.NewExchangeRateServiceImpl(nil, repo, params, nil, zaptest.NewLogger(t))

	meter := noop.NewMeterProvider().Meter("test")
	if err := impl.RegisterMetrics(meter); err != nil {
		t.Fatalf("RegisterMetrics with noop meter must not error: %v", err)
	}
}

func TestRegisterMetrics_AfterRegister_RefreshRateRecordsInstruments(t *testing.T) {
	// Exercises recordRefreshMetric and recordRateGauges with non-nil instruments.
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{}

	impl := service.NewExchangeRateServiceImpl(fetcher, repo, params, nil, zaptest.NewLogger(t))
	meter := noop.NewMeterProvider().Meter("test")
	if err := impl.RegisterMetrics(meter); err != nil {
		t.Fatalf("RegisterMetrics: %v", err)
	}

	// With real (noop) instruments, RefreshRate must still succeed and not panic.
	rates, err := impl.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("RefreshRate after RegisterMetrics: %v", err)
	}
	if rates == nil {
		t.Fatal("expected non-nil rates")
	}
}

// ── TestGetCurrentRates (DB fallback) ─────────────────────────────────────────

func TestGetCurrentRates_ColdCache_FallsBackToDatabase(t *testing.T) {
	// No RefreshRate called — cache is cold. GetCurrentRates must hit DB.
	rec := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.8000"),
		BuyRate:       decimal.RequireFromString("7.6830"),
		SellRate:      decimal.RequireFromString("7.9560"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		EffectiveAt:   time.Now().Add(-1 * time.Hour),
		FetchedAt:     time.Now().Add(-1 * time.Hour),
	}
	repo := &stubFXRepo{latest: rec}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	rates, err := svc.GetCurrentRates(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !rates.ReferenceRate.Equal(rec.ReferenceRate) {
		t.Errorf("ReferenceRate = %s, want %s", rates.ReferenceRate, rec.ReferenceRate)
	}
	if rates.Source != "banguat" {
		t.Errorf("Source = %q, want %q", rates.Source, "banguat")
	}
}

func TestGetCurrentRates_EmptyDB_ReturnsError(t *testing.T) {
	repo := &stubFXRepo{latest: nil}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	_, err := svc.GetCurrentRates(context.Background())
	if err == nil {
		t.Fatal("expected error when cache cold and DB empty, got nil")
	}
}

func TestGetCurrentRates_DBError_PropagatesError(t *testing.T) {
	repo := &stubErrFXRepo{err: errors.New("pgx: connection reset")}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	_, err := svc.GetCurrentRates(context.Background())
	if err == nil {
		t.Fatal("expected error when DB returns error, got nil")
	}
}

// stubErrFXRepo always returns an error from GetLatest.
type stubErrFXRepo struct {
	err error
}

func (r *stubErrFXRepo) Save(_ context.Context, _ *domain.ExchangeRateRecord) error { return nil }
func (r *stubErrFXRepo) GetLatest(_ context.Context) (*domain.ExchangeRateRecord, error) {
	return nil, r.err
}
func (r *stubErrFXRepo) GetHistory(_ context.Context, _, _ int) ([]*domain.ExchangeRateRecord, error) {
	return nil, r.err
}
func (r *stubErrFXRepo) GetOverrides(_ context.Context, _ int) ([]*domain.ExchangeRateRecord, error) {
	return nil, r.err
}

// ── TestWarmCache ─────────────────────────────────────────────────────────────

func TestWarmCache_EmptyDB_ReturnsNilWithoutError(t *testing.T) {
	repo := &stubFXRepo{latest: nil}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache with empty DB must succeed: %v", err)
	}
}

func TestWarmCache_WithHistory_PopulatesCache(t *testing.T) {
	rec := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.75"),
		BuyRate:       decimal.RequireFromString("7.63375"),
		SellRate:      decimal.RequireFromString("7.905"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		EffectiveAt:   time.Now().Add(-30 * time.Minute),
		FetchedAt:     time.Now().Add(-30 * time.Minute),
	}
	repo := &stubFXRepo{latest: rec}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	if err := svc.WarmCache(context.Background()); err != nil {
		t.Fatalf("WarmCache: %v", err)
	}

	// Cache should now be populated — GetCurrentRates returns from cache, not DB.
	rates, err := svc.GetCurrentRates(context.Background())
	if err != nil {
		t.Fatalf("GetCurrentRates after WarmCache: %v", err)
	}
	if !rates.ReferenceRate.Equal(rec.ReferenceRate) {
		t.Errorf("ReferenceRate = %s, want %s (expected cached value)", rates.ReferenceRate, rec.ReferenceRate)
	}
}

func TestWarmCache_DBError_PropagatesError(t *testing.T) {
	repo := &stubErrFXRepo{err: errors.New("pgx: timeout")}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	if err := svc.WarmCache(context.Background()); err == nil {
		t.Fatal("expected error when DB fails, got nil")
	}
}

// ── TestConvertGTQToUSD edge case ─────────────────────────────────────────────

func TestConvertGTQToUSD_CacheCold_UsesDBFallback(t *testing.T) {
	// ConvertGTQToUSD calls GetCurrentRates, which must fall back to DB.
	rec := &domain.ExchangeRateRecord{
		ReferenceRate: decimal.RequireFromString("7.75"),
		BuyRate:       decimal.RequireFromString("7.63375"),
		SellRate:      decimal.RequireFromString("7.905"),
		BuyMarginPct:  decimal.RequireFromString("0.015"),
		SellMarginPct: decimal.RequireFromString("0.020"),
		Source:        "banguat",
		EffectiveAt:   time.Now(),
		FetchedAt:     time.Now(),
	}
	repo := &stubFXRepo{latest: rec}
	params := newStubParamService(nil)
	svc := service.NewExchangeRateService(nil, repo, params, nil, zaptest.NewLogger(t))

	usdCents, rate, err := svc.ConvertGTQToUSD(context.Background(), 7905)
	if err != nil {
		t.Fatalf("ConvertGTQToUSD: %v", err)
	}
	if usdCents != 1000 {
		t.Errorf("usdCents = %d, want 1000", usdCents)
	}
	wantRate := decimal.RequireFromString("7.905")
	if !rate.Equal(wantRate) {
		t.Errorf("rate = %s, want %s", rate, wantRate)
	}
}

// ── TestRefreshRate additional paths ──────────────────────────────────────────

func TestRefreshRate_SaveError_LogsAndContinues(t *testing.T) {
	// If saving to history fails, RefreshRate must still return the rates
	// (best-effort persistence: stale cache > failed DB write).
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	repo := &stubFXRepo{saveErr: errors.New("pgx: disk full")}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("RefreshRate must succeed even when save fails: %v", err)
	}
	if rates == nil {
		t.Fatal("expected non-nil rates")
	}
}

func TestRefreshRate_USDGTQRateWriteError_LogsAndContinues(t *testing.T) {
	// If writing payment.usd_gtq_rate fails, RefreshRate must still return rates.
	fetcher := &stubFetcher{
		sourceName: "banguat",
		rate:       rawRate("7.7500", "banguat"),
	}
	params := newStubParamService(map[string]int{
		domain.ParamKeyFXBuyMarginBPS:  150,
		domain.ParamKeyFXSellMarginBPS: 200,
	})
	params.setErr = errors.New("param service unavailable")
	repo := &stubFXRepo{}

	svc := service.NewExchangeRateService(fetcher, repo, params, nil, zaptest.NewLogger(t))
	rates, err := svc.RefreshRate(context.Background())
	if err != nil {
		t.Fatalf("RefreshRate must succeed even when param write fails: %v", err)
	}
	if rates == nil {
		t.Fatal("expected non-nil rates")
	}
}
