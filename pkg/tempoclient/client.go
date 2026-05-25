// Package tempoclient provides a minimal Grafana Tempo HTTP search client,
// used by the admin observability tracing endpoints.
package tempoclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client queries a Grafana Tempo HTTP API.
type Client struct {
	base string
	http *http.Client
}

// New constructs a Client targeting the Tempo instance at baseURL.
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// TraceSummary is a single trace entry returned by the Tempo search API.
type TraceSummary struct {
	TraceID           string `json:"traceID"`
	RootServiceName   string `json:"rootServiceName"`
	RootTraceName     string `json:"rootTraceName"`
	StartTimeUnixNano string `json:"startTimeUnixNano"`
	DurationMs        uint32 `json:"durationMs"`
}

// SearchResponse is the envelope returned by GET /api/search.
type SearchResponse struct {
	Traces []TraceSummary `json:"traces"`
}

// SearchErrors queries Tempo for recent error spans within the given time range.
// tags is a space-separated list of Tempo tag filters (e.g. "span.status.code=ERROR").
// limit caps the number of traces returned.
func (c *Client) SearchErrors(ctx context.Context, since time.Time, limit int) (*SearchResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/search", nil)
	if err != nil {
		return nil, fmt.Errorf("tempoclient: build request: %w", err)
	}

	q := url.Values{}
	q.Set("tags", "span.status.code=ERROR")
	q.Set("start", fmt.Sprintf("%d", since.Unix()))
	q.Set("end", fmt.Sprintf("%d", time.Now().Unix()))
	q.Set("limit", fmt.Sprintf("%d", limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tempoclient: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tempoclient: unexpected status %d", resp.StatusCode)
	}

	var sr SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&sr); err != nil {
		return nil, fmt.Errorf("tempoclient: decode: %w", err)
	}
	return &sr, nil
}
