package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shopspring/decimal"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	openExchangeURLTemplate = "https://openexchangerates.org/api/latest.json?app_id=%s&symbols=GTQ"
	openExchangeTimeout     = 5 * time.Second
	openExchangeBodyLimit   = 64 * 1024 // 64 KB
	openExchangeSourceName  = "openexchangerates"
)

// openExchangeResponse is the relevant subset of the openexchangerates.org
// /api/latest.json response. Only the GTQ rate is requested via the symbols
// query parameter, so the rates map contains at most one entry.
type openExchangeResponse struct {
	Rates map[string]float64 `json:"rates"`
	Error bool               `json:"error"`
	// Message is populated on error responses.
	Message string `json:"message"`
}

// OpenExchangeFetcher fetches the USD/GTQ rate from openexchangerates.org.
// Used as the second fallback when both Banguat and ExchangeRate-API fail.
// The free tier allows 1 000 requests per month — one daily fetch uses at
// most 31/month.
//
// An empty app ID disables this fetcher: FetchCurrent fails fast without
// making a network call, allowing it to be included in the chain unconditionally.
//
// Note: the base currency on the free tier is always USD, which is the
// direction we need (1 USD → N GTQ).
type OpenExchangeFetcher struct {
	appID       string
	urlTemplate string // format string: must contain one %s placeholder for the app ID
	client      *http.Client
}

// NewOpenExchangeFetcher constructs the fetcher pointed at the real
// openexchangerates.org endpoint. appID may be empty.
func NewOpenExchangeFetcher(appID string) *OpenExchangeFetcher {
	return newOpenExchangeFetcher(appID, openExchangeURLTemplate)
}

// NewOpenExchangeFetcherWithURL constructs the fetcher with a custom URL
// template. Intended for tests that substitute a local httptest.Server.
// urlTemplate must contain one %s placeholder for the app ID.
func NewOpenExchangeFetcherWithURL(appID, urlTemplate string) *OpenExchangeFetcher {
	return newOpenExchangeFetcher(appID, urlTemplate)
}

func newOpenExchangeFetcher(appID, urlTemplate string) *OpenExchangeFetcher {
	return &OpenExchangeFetcher{
		appID:       appID,
		urlTemplate: urlTemplate,
		client: &http.Client{
			Timeout:   openExchangeTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// Source implements ExchangeRateFetcher.
func (f *OpenExchangeFetcher) Source() string { return openExchangeSourceName }

// FetchCurrent retrieves the USD/GTQ rate from openexchangerates.org.
// Returns an error immediately when no app ID is configured.
func (f *OpenExchangeFetcher) FetchCurrent(ctx context.Context) (*RawRate, error) {
	if f.appID == "" {
		return nil, fmt.Errorf("openexchangerates: app ID not configured")
	}

	url := fmt.Sprintf(f.urlTemplate, f.appID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("openexchangerates: build request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openexchangerates: GET: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openexchangerates: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, openExchangeBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("openexchangerates: read body: %w", err)
	}

	var payload openExchangeResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("openexchangerates: parse JSON: %w", err)
	}
	if payload.Error {
		return nil, fmt.Errorf("openexchangerates: provider error: %s", payload.Message)
	}

	gtq, ok := payload.Rates["GTQ"]
	if !ok {
		return nil, fmt.Errorf("openexchangerates: GTQ rate missing from response")
	}
	if gtq <= 0 {
		return nil, fmt.Errorf("openexchangerates: GTQ rate %f is not positive", gtq)
	}

	// Convert float64 provider value to decimal via NewFromFloat to keep the
	// full float64 precision at the boundary; downstream margin arithmetic
	// operates entirely in decimal.
	rate := decimal.NewFromFloat(gtq)

	return &RawRate{
		ReferenceRate: rate,
		Source:        openExchangeSourceName,
		FetchedAt:     time.Now(),
	}, nil
}
