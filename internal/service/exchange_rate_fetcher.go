package service

import (
	"context"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
	"go.uber.org/zap"
)

// ExchangeRateFetcher fetches the current USD/GTQ reference rate from a single
// external source. Each implementation wraps exactly one data provider.
// Errors are wrapped with the source name so callers can distinguish failures
// by source without inspecting the error string: fmt.Errorf("banguat: %w", err).
type ExchangeRateFetcher interface {
	// FetchCurrent retrieves the latest available reference rate.
	// The returned RawRate.Source identifies which provider answered.
	// Implementations must honour context cancellation and enforce their own
	// request timeout independently of the caller's deadline.
	FetchCurrent(ctx context.Context) (*RawRate, error)

	// Source returns the canonical provider name used in metrics labels and
	// logs: "banguat", "exchangerate-api", or "openexchangerates".
	Source() string
}

// RawRate is the unprocessed reference rate returned by a single fetcher.
// Margin computation is performed downstream by the ExchangeRateService;
// the fetcher only delivers the market mid-point from its provider.
type RawRate struct {
	// ReferenceRate is the venta (sell) rate published by the provider,
	// expressed as GTQ per USD (e.g. decimal.NewFromString("7.7500")).
	ReferenceRate decimal.Decimal
	// Source is the canonical provider name ("banguat", "exchangerate-api",
	// "openexchangerates"). Matches ExchangeRateFetcher.Source().
	Source string
	// ApiVersion is the short revision identifier of the provider API endpoint
	// that answered this request: "v6" (ExchangeRate-API), "v1"
	// (OpenExchangeRates), "xml" (Banguat XML feed).  Stored in
	// exchange_rate_history.api_version for post-incident debugging when a
	// provider changes its feed format or endpoint version.
	ApiVersion string
	// FetchedAt is the wall-clock time at which the provider responded.
	FetchedAt time.Time
}

// MultiSourceFetcher tries each ExchangeRateFetcher in order and returns the
// first successful result. If all sources fail it returns an aggregate error
// that wraps every individual error so callers can log the full failure chain.
//
// Usage: construct with NewMultiSourceFetcher, pass sources in priority order
// (Banguat first, ExchangeRate-API second, OpenExchangeRates third).
type MultiSourceFetcher struct {
	sources []ExchangeRateFetcher
	log     *zap.Logger
}

// NewMultiSourceFetcher constructs a MultiSourceFetcher. sources must be
// non-empty and ordered from highest to lowest priority. The first source that
// succeeds short-circuits the chain; remaining sources are never called.
func NewMultiSourceFetcher(log *zap.Logger, sources ...ExchangeRateFetcher) *MultiSourceFetcher {
	if len(sources) == 0 {
		panic("service: NewMultiSourceFetcher requires at least one source")
	}
	return &MultiSourceFetcher{sources: sources, log: log}
}

// FetchCurrent tries sources in order and returns the first success.
// On failure of a source it logs a warning and proceeds to the next.
// Returns an error only when every source has failed.
func (m *MultiSourceFetcher) FetchCurrent(ctx context.Context) (*RawRate, error) {
	var errs []error
	for _, src := range m.sources {
		rate, err := src.FetchCurrent(ctx)
		if err == nil {
			return rate, nil
		}
		m.log.Warn("exchange rate source failed, trying next",
			zap.String("source", src.Source()),
			zap.Error(err),
		)
		errs = append(errs, err)
	}
	return nil, fmt.Errorf("all exchange rate sources failed: %w", joinErrors(errs))
}

// joinErrors wraps a slice of errors into a single error whose message lists
// each constituent. Used only internally; the callers unwrap via errors.Is/As
// against the first error, which is sufficient for logging purposes.
func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	msg := errs[0].Error()
	for _, e := range errs[1:] {
		msg += "; " + e.Error()
	}
	return fmt.Errorf("%s", msg) //nolint:err113
}
