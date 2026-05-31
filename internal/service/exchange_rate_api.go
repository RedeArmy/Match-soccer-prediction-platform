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
	exchangeRateAPIURLTemplate = "https://v6.exchangerate-api.com/v6/%s/pair/USD/GTQ"
	exchangeRateAPITimeout     = 5 * time.Second
	exchangeRateAPIBodyLimit   = 64 * 1024 // 64 KB
	exchangeRateAPISourceName  = "exchangerate-api"
)

// exchangeRateAPIResponse is the relevant subset of the exchangerate-api.com
// JSON response for the pair/USD/GTQ endpoint.
// Full response includes additional fields (time_last_update_unix, etc.)
// that are not needed.
type exchangeRateAPIResponse struct {
	Result         string  `json:"result"`          // "success" or "error"
	ConversionRate float64 `json:"conversion_rate"` // GTQ per 1 USD
	ErrorType      string  `json:"error-type"`      // populated on error responses
}

// ExchangeRateAPIFetcher fetches the USD/GTQ rate from v6.exchangerate-api.com.
// Used as the first fallback when Banguat is unavailable. The free tier allows
// 1 500 requests per month — one daily fetch uses fewer than 32/month.
//
// An empty API key disables this fetcher: FetchCurrent returns an error
// immediately without making a network call. This allows the fetcher to be
// included in the multi-source chain unconditionally; when no key is configured
// it simply fails fast to the next source.
type ExchangeRateAPIFetcher struct {
	apiKey      string
	urlTemplate string // format string: must contain one %s placeholder for the API key
	client      *http.Client
}

// NewExchangeRateAPIFetcher constructs the fetcher pointed at the real
// v6.exchangerate-api.com endpoint. apiKey may be empty.
func NewExchangeRateAPIFetcher(apiKey string) *ExchangeRateAPIFetcher {
	return newExchangeRateAPIFetcher(apiKey, exchangeRateAPIURLTemplate)
}

// NewExchangeRateAPIFetcherWithURL constructs the fetcher with a custom URL
// template. Intended for tests that substitute a local httptest.Server.
// urlTemplate must contain one %s placeholder for the API key.
func NewExchangeRateAPIFetcherWithURL(apiKey, urlTemplate string) *ExchangeRateAPIFetcher {
	return newExchangeRateAPIFetcher(apiKey, urlTemplate)
}

func newExchangeRateAPIFetcher(apiKey, urlTemplate string) *ExchangeRateAPIFetcher {
	return &ExchangeRateAPIFetcher{
		apiKey:      apiKey,
		urlTemplate: urlTemplate,
		client: &http.Client{
			Timeout:   exchangeRateAPITimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// Source implements ExchangeRateFetcher.
func (f *ExchangeRateAPIFetcher) Source() string { return exchangeRateAPISourceName }

// FetchCurrent retrieves the USD/GTQ conversion rate from exchangerate-api.com.
// Returns an error immediately when no API key is configured.
func (f *ExchangeRateAPIFetcher) FetchCurrent(ctx context.Context) (*RawRate, error) {
	if f.apiKey == "" {
		return nil, fmt.Errorf("exchangerate-api: API key not configured")
	}

	url := fmt.Sprintf(f.urlTemplate, f.apiKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("exchangerate-api: build request: %w", err)
	}

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("exchangerate-api: GET: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("exchangerate-api: unexpected status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, exchangeRateAPIBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("exchangerate-api: read body: %w", err)
	}

	var payload exchangeRateAPIResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("exchangerate-api: parse JSON: %w", err)
	}
	if payload.Result != "success" {
		return nil, fmt.Errorf("exchangerate-api: provider error %q", payload.ErrorType)
	}
	if payload.ConversionRate <= 0 {
		return nil, fmt.Errorf("exchangerate-api: conversion_rate %f is not positive", payload.ConversionRate)
	}

	// Convert float64 provider value to decimal via string to avoid
	// floating-point precision loss at the boundary.
	rate := decimal.NewFromFloat(payload.ConversionRate)

	return &RawRate{
		ReferenceRate: rate,
		Source:        exchangeRateAPISourceName,
		FetchedAt:     time.Now(),
	}, nil
}
