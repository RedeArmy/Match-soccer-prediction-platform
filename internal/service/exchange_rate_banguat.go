package service

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

const (
	banguatURL        = "https://www.banguat.gob.gt/variables/xml/dolares.xml"
	banguatTimeout    = 10 * time.Second
	banguatBodyLimit  = 512 * 1024 // 512 KB — far exceeds any realistic XML response
	banguatSourceName = "banguat"
)

// banguatRootXML is the outer envelope of the Banguat dolares feed.
// The root element <VarDolares> contains a repeated <VarDolares> child for
// each daily entry. The most recent entry is always first.
type banguatRootXML struct {
	XMLName xml.Name          `xml:"VarDolares"`
	Entries []banguatEntryXML `xml:"VarDolares"`
}

// banguatEntryXML represents one daily rate entry from the Banguat feed.
// compra = buy rate, venta = sell rate, both expressed as GTQ per USD.
// Rates are parsed as strings to avoid float64 precision loss before
// conversion to decimal.Decimal.
type banguatEntryXML struct {
	DateStr string `xml:"fechaDate"`
	BuyStr  string `xml:"compra"`
	SellStr string `xml:"venta"`
}

// BanguatFetcher fetches the official Banco de Guatemala USD/GTQ exchange rate
// from the public XML feed at banguat.gob.gt. No API key required.
//
// The feed updates at approximately 10:00 AM Guatemala time each banking day.
// A fetch at 01:00 AM will return the previous day's official rate, which is
// acceptable and standard practice for daily FX reference rates.
//
// HTTP client is constructed per the project pattern:
// http.Client{Timeout: 10s, Transport: otelhttp.NewTransport(http.DefaultTransport)}.
type BanguatFetcher struct {
	url    string
	client *http.Client
}

// NewBanguatFetcher constructs a BanguatFetcher pointed at the real Banguat
// production endpoint.
func NewBanguatFetcher() *BanguatFetcher {
	return newBanguatFetcher(banguatURL)
}

// NewBanguatFetcherWithURL constructs a BanguatFetcher pointed at url.
// Intended for use in tests that substitute a local httptest.Server.
func NewBanguatFetcherWithURL(url string) *BanguatFetcher {
	return newBanguatFetcher(url)
}

func newBanguatFetcher(url string) *BanguatFetcher {
	return &BanguatFetcher{
		url: url,
		client: &http.Client{
			Timeout:   banguatTimeout,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
	}
}

// Source implements ExchangeRateFetcher.
func (f *BanguatFetcher) Source() string { return banguatSourceName }

// FetchCurrent retrieves the latest venta (sell) rate from the Banguat XML feed.
// Returns the most recent entry's sell rate as a decimal.Decimal.
// Errors are wrapped with the "banguat:" prefix.
func (f *BanguatFetcher) FetchCurrent(ctx context.Context) (*RawRate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.url, nil)
	if err != nil {
		return nil, fmt.Errorf("banguat: build request: %w", err)
	}
	req.Header.Set("Accept", "application/xml, text/xml")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("banguat: GET %s: %w", banguatURL, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("banguat: unexpected status %d from %s", resp.StatusCode, f.url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, banguatBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("banguat: read body: %w", err)
	}

	var root banguatRootXML
	if err := xml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("banguat: parse XML: %w", err)
	}
	if len(root.Entries) == 0 {
		return nil, fmt.Errorf("banguat: XML contains no rate entries")
	}

	// The first entry is always the most recent.
	entry := root.Entries[0]
	sellStr := strings.TrimSpace(entry.SellStr)
	if sellStr == "" {
		return nil, fmt.Errorf("banguat: venta field is empty in most recent entry")
	}

	sellRate, err := decimal.NewFromString(sellStr)
	if err != nil {
		return nil, fmt.Errorf("banguat: parse venta %q: %w", sellStr, err)
	}
	if sellRate.IsZero() || sellRate.IsNegative() {
		return nil, fmt.Errorf("banguat: venta rate %s is not positive", sellStr)
	}

	return &RawRate{
		ReferenceRate: sellRate,
		Source:        banguatSourceName,
		FetchedAt:     time.Now(),
	}, nil
}
