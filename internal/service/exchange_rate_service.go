package service

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// systemActorID is used as the actorID when automated jobs write system params.
// 0 is not a valid user ID (PostgreSQL SERIAL starts at 1), so audit queries
// can identify automated writes by filtering on actor_id = 0.
const systemActorID = 0

// ExchangeRateParamRW is the subset of SystemParamService consumed by
// ExchangeRateService.  Using a narrow interface keeps the service testable
// without instantiating the full SystemParamService.
type ExchangeRateParamRW interface {
	GetInt(ctx context.Context, key string, def int) int
	Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error)
}

// RateFetcher is the subset of ExchangeRateFetcher consumed by
// ExchangeRateService.  *MultiSourceFetcher satisfies this interface even
// though it does not implement ExchangeRateFetcher.Source().
type RateFetcher interface {
	FetchCurrent(ctx context.Context) (*RawRate, error)
}

// FXStaleNotifier is the subset of the observability Notifier consumed by
// ExchangeRateService to fire the n8n stale-rate alert.
type FXStaleNotifier interface {
	NotifyFXRateStale(ctx context.Context, lastRate string, ageHours float64, source string)
}

// ExchangeRateService manages the full lifecycle of the USD/GTQ exchange rate:
// daily automated fetch, admin override, in-memory caching, and conversion helpers.
type ExchangeRateService interface {
	// RefreshRate fetches the current rate from external providers, applies
	// buy/sell margins, persists to exchange_rate_history, and updates the
	// payment.usd_gtq_rate system param (backward compat for WithdrawalService).
	// Called by the daily scheduler job; never called inline on a payment path.
	RefreshRate(ctx context.Context) (*domain.ExchangeRates, error)

	// GetCurrentRates returns the current rates from the in-memory cache.
	// Cache miss falls back to the most recent exchange_rate_history row.
	// Never calls an external API — safe to call on every payment request.
	GetCurrentRates(ctx context.Context) (*domain.ExchangeRates, error)

	// OverrideRate sets a manual reference rate with a required audit reason.
	// Invalidates and repopulates the cache immediately.
	OverrideRate(ctx context.Context, referenceRate decimal.Decimal, reason string, adminID int) (*domain.ExchangeRates, error)

	// ConvertUSDToGTQ converts USD cents to GTQ cents using the buy rate
	// (platform buys USD from the user at below-reference rate).
	// Returns the GTQ cent amount and the applied rate.
	ConvertUSDToGTQ(ctx context.Context, usdCents int64) (gtqCents int64, rate decimal.Decimal, err error)

	// ConvertGTQToUSD converts GTQ cents to USD cents using the sell rate
	// (platform sells USD to the user at above-reference rate).
	// Returns the USD cent amount and the applied rate.
	ConvertGTQToUSD(ctx context.Context, gtqCents int64) (usdCents int64, rate decimal.Decimal, err error)

	// WarmCache pre-loads the cache from the most recent exchange_rate_history row.
	// Call once at API server startup so the first GetCurrentRates request
	// is served from cache, not from the DB.
	WarmCache(ctx context.Context) error
}

// ── in-memory cache ───────────────────────────────────────────────────────────

type exchangeRateCache struct {
	mu            sync.RWMutex
	rates         *domain.ExchangeRates
	cachedAt      time.Time
	lastFetchedAt time.Time // time of the last successful provider fetch
	ttl           time.Duration
}

func newExchangeRateCache(ttl time.Duration) *exchangeRateCache {
	return &exchangeRateCache{ttl: ttl}
}

func (c *exchangeRateCache) get() *domain.ExchangeRates {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.rates == nil || time.Now().After(c.cachedAt.Add(c.ttl)) {
		return nil
	}
	return c.rates
}

func (c *exchangeRateCache) set(rates *domain.ExchangeRates, fetchedAt time.Time) {
	c.mu.Lock()
	c.rates = rates
	c.cachedAt = time.Now()
	if !fetchedAt.IsZero() {
		c.lastFetchedAt = fetchedAt
	}
	c.mu.Unlock()
}

func (c *exchangeRateCache) ageSeconds() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.lastFetchedAt.IsZero() {
		return 0
	}
	return time.Since(c.lastFetchedAt).Seconds()
}

// ── service implementation ────────────────────────────────────────────────────

// ExchangeRateServiceImpl is the concrete implementation of ExchangeRateService.
// Exported so callers (server_compose.go) can call RegisterMetrics on it
// without adding RegisterMetrics to the interface — consistent with the project
// pattern of registering OTel instruments on concrete types.
type ExchangeRateServiceImpl struct {
	fetcher  RateFetcher
	repo     repository.ExchangeRateRepository
	params   ExchangeRateParamRW
	notifier FXStaleNotifier
	cache    *exchangeRateCache
	log      *zap.Logger

	// OTel instruments — set by RegisterMetrics; safe to leave nil.
	refreshCounter  metric.Int64Counter
	rateGauge       metric.Float64Gauge
	overrideCounter metric.Int64Counter
	ageGauge        metric.Float64ObservableGauge
}

const defaultCacheTTLFX = 1 * time.Hour

// NewExchangeRateServiceImpl constructs the concrete ExchangeRateServiceImpl.
// Use this constructor when the caller needs to call RegisterMetrics.
// notifier may be nil — stale alerts are silently skipped in that case.
func NewExchangeRateServiceImpl(
	fetcher RateFetcher,
	repo repository.ExchangeRateRepository,
	params ExchangeRateParamRW,
	notifier FXStaleNotifier,
	log *zap.Logger,
) *ExchangeRateServiceImpl {
	return &ExchangeRateServiceImpl{
		fetcher:  fetcher,
		repo:     repo,
		params:   params,
		notifier: notifier,
		cache:    newExchangeRateCache(defaultCacheTTLFX),
		log:      log,
	}
}

// NewExchangeRateService constructs an ExchangeRateService interface value.
// Use this constructor when RegisterMetrics is not needed (e.g., in tests).
func NewExchangeRateService(
	fetcher RateFetcher,
	repo repository.ExchangeRateRepository,
	params ExchangeRateParamRW,
	notifier FXStaleNotifier,
	log *zap.Logger,
) ExchangeRateService {
	return NewExchangeRateServiceImpl(fetcher, repo, params, notifier, log)
}

// RegisterMetrics wires OTel instruments.  Call once after construction and
// before the service processes any requests.  A nil meter is a no-op.
func (s *ExchangeRateServiceImpl) RegisterMetrics(meter metric.Meter) error {
	var err error
	s.refreshCounter, err = meter.Int64Counter("wcq_fx_rate_refresh_total",
		metric.WithDescription("Total automated exchange rate refresh attempts"),
	)
	if err != nil {
		return fmt.Errorf("fx metrics: wcq_fx_rate_refresh_total: %w", err)
	}
	s.rateGauge, err = meter.Float64Gauge("wcq_fx_rate_current_gtq_per_usd",
		metric.WithDescription("Current USD/GTQ rates by type (reference, buy, sell)"),
		metric.WithUnit("GTQ/USD"),
	)
	if err != nil {
		return fmt.Errorf("fx metrics: wcq_fx_rate_current_gtq_per_usd: %w", err)
	}
	s.overrideCounter, err = meter.Int64Counter("wcq_fx_rate_override_total",
		metric.WithDescription("Total admin manual exchange rate overrides"),
	)
	if err != nil {
		return fmt.Errorf("fx metrics: wcq_fx_rate_override_total: %w", err)
	}
	s.ageGauge, err = meter.Float64ObservableGauge("wcq_fx_rate_age_seconds",
		metric.WithDescription("Seconds since the last successful exchange rate fetch"),
		metric.WithUnit("s"),
	)
	if err != nil {
		return fmt.Errorf("fx metrics: wcq_fx_rate_age_seconds: %w", err)
	}
	if _, err = meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		o.ObserveFloat64(s.ageGauge, s.cache.ageSeconds())
		return nil
	}, s.ageGauge); err != nil {
		return fmt.Errorf("fx metrics: register age callback: %w", err)
	}
	return nil
}

// ── RefreshRate ───────────────────────────────────────────────────────────────

func (s *ExchangeRateServiceImpl) RefreshRate(ctx context.Context) (*domain.ExchangeRates, error) {
	raw, fetchErr := s.fetcher.FetchCurrent(ctx)

	var rates *domain.ExchangeRates
	var source string
	var stale bool

	if fetchErr != nil {
		// All external sources failed — fall back to last known rate.
		s.log.Warn("exchange_rate: all sources failed, using last known rate", zap.Error(fetchErr))
		last, dbErr := s.repo.GetLatest(ctx)
		if dbErr != nil {
			s.recordRefreshMetric("stale", false)
			return nil, fmt.Errorf("exchange_rate: fetch failed and DB fallback unavailable: fetch=%w db=%v", fetchErr, dbErr)
		}
		if last == nil {
			s.recordRefreshMetric("stale", false)
			return nil, fmt.Errorf("exchange_rate: all sources failed and no historical rate available: %w", fetchErr)
		}
		rates = last.ToRates()
		rates.Stale = true
		rates.Source = "stale"
		source = "stale"
		stale = true

		if s.notifier != nil {
			s.notifier.NotifyFXRateStale(ctx,
				last.SellRate.String(),
				time.Since(last.FetchedAt).Hours(),
				last.Source,
			)
		}
	} else {
		buyBPS := s.params.GetInt(ctx, domain.ParamKeyFXBuyMarginBPS, domain.DefaultFXBuyMarginBPS)
		sellBPS := s.params.GetInt(ctx, domain.ParamKeyFXSellMarginBPS, domain.DefaultFXSellMarginBPS)
		rates = computeRates(raw.ReferenceRate, buyBPS, sellBPS, raw.Source, false)
		source = raw.Source
	}

	// Persist to history.
	fetchedAt := time.Now()
	if raw != nil {
		fetchedAt = raw.FetchedAt
	}
	rec := &domain.ExchangeRateRecord{
		ReferenceRate: rates.ReferenceRate,
		BuyRate:       rates.BuyRate,
		SellRate:      rates.SellRate,
		BuyMarginPct:  rates.BuyMarginPct,
		SellMarginPct: rates.SellMarginPct,
		Source:        source,
		Stale:         stale,
		IsOverride:    false,
		FetchedAt:     fetchedAt,
		EffectiveAt:   fetchedAt,
	}
	if saveErr := s.repo.Save(ctx, rec); saveErr != nil {
		// Log but do not fail — stale cache is better than no cache.
		s.log.Warn("exchange_rate: failed to persist history row", zap.Error(saveErr))
	}

	// Write payment.usd_gtq_rate for backward compat (WithdrawalService).
	if !stale {
		if writeErr := s.writeUSDGTQRate(ctx, rates.SellRate); writeErr != nil {
			s.log.Warn("exchange_rate: failed to update payment.usd_gtq_rate", zap.Error(writeErr))
		}
	}

	s.cache.set(rates, fetchedAt)
	s.recordRefreshMetric(source, true)
	s.recordRateGauges(rates)

	s.log.Info("exchange_rate: refreshed",
		zap.String("source", source),
		zap.String("reference", rates.ReferenceRate.String()),
		zap.String("buy", rates.BuyRate.String()),
		zap.String("sell", rates.SellRate.String()),
		zap.Bool("stale", stale),
	)
	return rates, nil
}

// ── GetCurrentRates ───────────────────────────────────────────────────────────

func (s *ExchangeRateServiceImpl) GetCurrentRates(ctx context.Context) (*domain.ExchangeRates, error) {
	if cached := s.cache.get(); cached != nil {
		return cached, nil
	}
	latest, err := s.repo.GetLatest(ctx)
	if err != nil {
		return nil, fmt.Errorf("exchange_rate: get current rates: %w", err)
	}
	if latest == nil {
		return nil, fmt.Errorf("exchange_rate: no rate available (table empty)")
	}
	rates := latest.ToRates()
	s.cache.set(rates, latest.FetchedAt)
	return rates, nil
}

// ── OverrideRate ──────────────────────────────────────────────────────────────

func (s *ExchangeRateServiceImpl) OverrideRate(
	ctx context.Context,
	referenceRate decimal.Decimal,
	reason string,
	adminID int,
) (*domain.ExchangeRates, error) {
	if reason == "" {
		return nil, fmt.Errorf("exchange_rate: override reason is required")
	}
	if referenceRate.LessThanOrEqual(decimal.Zero) || referenceRate.GreaterThan(decimal.NewFromInt(20)) {
		return nil, fmt.Errorf("exchange_rate: reference_rate %s is outside allowed range (0, 20]", referenceRate)
	}

	buyBPS := s.params.GetInt(ctx, domain.ParamKeyFXBuyMarginBPS, domain.DefaultFXBuyMarginBPS)
	sellBPS := s.params.GetInt(ctx, domain.ParamKeyFXSellMarginBPS, domain.DefaultFXSellMarginBPS)
	rates := computeRates(referenceRate, buyBPS, sellBPS, "override", false)
	rates.IsOverride = true

	now := time.Now()
	rec := &domain.ExchangeRateRecord{
		ReferenceRate:  rates.ReferenceRate,
		BuyRate:        rates.BuyRate,
		SellRate:       rates.SellRate,
		BuyMarginPct:   rates.BuyMarginPct,
		SellMarginPct:  rates.SellMarginPct,
		Source:         "override",
		Stale:          false,
		IsOverride:     true,
		OverrideReason: reason,
		OverrideBy:     &adminID,
		FetchedAt:      now,
		EffectiveAt:    now,
	}
	if err := s.repo.Save(ctx, rec); err != nil {
		return nil, fmt.Errorf("exchange_rate: override: persist: %w", err)
	}

	if err := s.writeUSDGTQRate(ctx, rates.SellRate); err != nil {
		s.log.Warn("exchange_rate: override: failed to update payment.usd_gtq_rate", zap.Error(err))
	}

	s.cache.set(rates, now)
	if s.overrideCounter != nil {
		s.overrideCounter.Add(ctx, 1)
	}
	s.recordRateGauges(rates)

	s.log.Info("exchange_rate: admin override applied",
		zap.Int("admin_id", adminID),
		zap.String("reference", referenceRate.String()),
		zap.String("sell", rates.SellRate.String()),
		zap.String("reason", reason),
	)
	return rates, nil
}

// ── ConvertUSDToGTQ / ConvertGTQToUSD ────────────────────────────────────────

func (s *ExchangeRateServiceImpl) ConvertUSDToGTQ(ctx context.Context, usdCents int64) (int64, decimal.Decimal, error) {
	rates, err := s.GetCurrentRates(ctx)
	if err != nil {
		return 0, decimal.Zero, err
	}
	// gtqCents = usdCents × BuyRate  (floor — platform retains any fractional cent)
	result := decimal.NewFromInt(usdCents).Mul(rates.BuyRate).Floor()
	return result.IntPart(), rates.BuyRate, nil
}

func (s *ExchangeRateServiceImpl) ConvertGTQToUSD(ctx context.Context, gtqCents int64) (int64, decimal.Decimal, error) {
	rates, err := s.GetCurrentRates(ctx)
	if err != nil {
		return 0, decimal.Zero, err
	}
	if rates.SellRate.IsZero() {
		return 0, decimal.Zero, fmt.Errorf("exchange_rate: sell rate is zero")
	}
	// usdCents = gtqCents / SellRate  (floor — platform retains any fractional cent)
	result := decimal.NewFromInt(gtqCents).Div(rates.SellRate).Floor()
	return result.IntPart(), rates.SellRate, nil
}

// ── WarmCache ─────────────────────────────────────────────────────────────────

func (s *ExchangeRateServiceImpl) WarmCache(ctx context.Context) error {
	latest, err := s.repo.GetLatest(ctx)
	if err != nil {
		return fmt.Errorf("exchange_rate: warm cache: %w", err)
	}
	if latest == nil {
		s.log.Info("exchange_rate: cache warm skipped (no historical rate yet)")
		return nil
	}
	rates := latest.ToRates()
	s.cache.set(rates, latest.FetchedAt)
	s.log.Info("exchange_rate: cache warmed from history",
		zap.String("source", latest.Source),
		zap.String("sell", latest.SellRate.String()),
		zap.Time("effective_at", latest.EffectiveAt),
	)
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

// computeRates applies buy/sell margins (in basis points) to produce ExchangeRates.
// All arithmetic in decimal.Decimal — no float64 in the computation chain.
func computeRates(reference decimal.Decimal, buyBPS, sellBPS int, source string, stale bool) *domain.ExchangeRates {
	tenK := decimal.NewFromInt(10_000)
	buyMarginPct := decimal.NewFromInt(int64(buyBPS)).Div(tenK)
	sellMarginPct := decimal.NewFromInt(int64(sellBPS)).Div(tenK)

	one := decimal.NewFromInt(1)
	buyRate := reference.Mul(one.Sub(buyMarginPct))
	sellRate := reference.Mul(one.Add(sellMarginPct))

	return &domain.ExchangeRates{
		ReferenceRate: reference,
		BuyRate:       buyRate,
		SellRate:      sellRate,
		BuyMarginPct:  buyMarginPct,
		SellMarginPct: sellMarginPct,
		Source:        source,
		Stale:         stale,
		EffectiveAt:   time.Now(),
	}
}

// writeUSDGTQRate converts the sell rate to GTQ centavos per USD and writes it
// to payment.usd_gtq_rate for backward compatibility with WithdrawalService.
// Formula: centavos = floor(sellRate × 100).
func (s *ExchangeRateServiceImpl) writeUSDGTQRate(ctx context.Context, sellRate decimal.Decimal) error {
	centavos := sellRate.Mul(decimal.NewFromInt(100)).Floor().IntPart()
	const minCentavos, maxCentavos = int64(100), int64(10_000)
	if centavos < minCentavos || centavos > maxCentavos {
		return fmt.Errorf("sell rate %s → %d centavos is outside [%d, %d]",
			sellRate, centavos, minCentavos, maxCentavos)
	}
	_, err := s.params.Set(ctx, domain.ParamKeyUSDGTQRate, strconv.FormatInt(centavos, 10), systemActorID)
	return err
}

func (s *ExchangeRateServiceImpl) recordRefreshMetric(source string, success bool) {
	if s.refreshCounter == nil {
		return
	}
	status := "success"
	if !success {
		status = "failure"
	}
	if source == "stale" {
		status = "stale"
	}
	s.refreshCounter.Add(context.Background(), 1,
		metric.WithAttributes(
			attribute.String("source", source),
			attribute.String("status", status),
		),
	)
}

func (s *ExchangeRateServiceImpl) recordRateGauges(rates *domain.ExchangeRates) {
	if s.rateGauge == nil {
		return
	}
	ctx := context.Background()
	refF, _ := rates.ReferenceRate.Float64()
	buyF, _ := rates.BuyRate.Float64()
	sellF, _ := rates.SellRate.Float64()
	s.rateGauge.Record(ctx, refF, metric.WithAttributes(attribute.String("rate_type", "reference")))
	s.rateGauge.Record(ctx, buyF, metric.WithAttributes(attribute.String("rate_type", "buy")))
	s.rateGauge.Record(ctx, sellF, metric.WithAttributes(attribute.String("rate_type", "sell")))
}
