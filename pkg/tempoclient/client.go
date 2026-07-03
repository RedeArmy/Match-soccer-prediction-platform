// Package tempoclient provides a minimal Grafana Tempo HTTP search client,
// used by the admin observability tracing endpoints.
package tempoclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// maxResponseBytes bounds how much of a Tempo response body this client will
// read. 10 MiB is generous for a search-result JSON payload; the cap exists
// so a compromised or misconfigured Tempo (its URL comes from operator-
// supplied config, WCQ_OBSERVABILITY_TEMPOURL) can't make an admin request
// consume unbounded memory by streaming an endless body.
const maxResponseBytes = 10 << 20

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
// limit caps the number of traces returned.
func (c *Client) SearchErrors(ctx context.Context, since time.Time, limit int) (*SearchResponse, error) {
	return c.Search(ctx, "span.status.code=ERROR", since, limit, "")
}

// Search queries Tempo for traces matching the given tag filter string.
// nameFilter, if non-empty, narrows results to those whose RootTraceName or
// RootServiceName contains the filter (case-insensitive, client-side).
func (c *Client) Search(ctx context.Context, tags string, since time.Time, limit int, nameFilter string) (*SearchResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/search", nil)
	if err != nil {
		return nil, fmt.Errorf("tempoclient: build request: %w", err)
	}

	q := url.Values{}
	q.Set("tags", tags)
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
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&sr); err != nil {
		return nil, fmt.Errorf("tempoclient: decode: %w", err)
	}
	if nameFilter == "" {
		return &sr, nil
	}
	lower := strings.ToLower(nameFilter)
	kept := sr.Traces[:0]
	for _, t := range sr.Traces {
		if strings.Contains(strings.ToLower(t.RootTraceName), lower) ||
			strings.Contains(strings.ToLower(t.RootServiceName), lower) {
			kept = append(kept, t)
		}
	}
	sr.Traces = kept
	return &sr, nil
}
