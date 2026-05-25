// Package promclient provides a minimal Prometheus HTTP API client for instant
// queries, used by the admin observability endpoints.
package promclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client queries a Prometheus HTTP API.
type Client struct {
	base string
	http *http.Client
}

// New constructs a Client targeting the Prometheus instance at baseURL.
func New(baseURL string) *Client {
	return &Client{
		base: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// QueryResponse is the top-level envelope returned by /api/v1/query.
type QueryResponse struct {
	Status string    `json:"status"`
	Data   QueryData `json:"data"`
	Error  string    `json:"error,omitempty"`
}

// QueryData holds the result type and raw result vector.
type QueryData struct {
	ResultType string            `json:"resultType"`
	Result     []json.RawMessage `json:"result"`
}

// ScalarSample is a single instant-query sample with optional labels.
type ScalarSample struct {
	Labels map[string]string  `json:"metric"`
	Value  [2]json.RawMessage `json:"value"` // [timestamp, value_string]
}

// Query executes a Prometheus instant query against /api/v1/query.
// Returns the parsed response or an error if the HTTP call or status is non-OK.
func (c *Client) Query(ctx context.Context, query string) (*QueryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+"/api/v1/query", nil)
	if err != nil {
		return nil, fmt.Errorf("promclient: build request: %w", err)
	}
	q := url.Values{}
	q.Set("query", query)
	req.URL.RawQuery = q.Encode()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("promclient: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("promclient: unexpected status %d", resp.StatusCode)
	}

	var qr QueryResponse
	if err := json.NewDecoder(resp.Body).Decode(&qr); err != nil {
		return nil, fmt.Errorf("promclient: decode: %w", err)
	}
	if qr.Status != "success" {
		return nil, fmt.Errorf("promclient: error status: %s", qr.Error)
	}
	return &qr, nil
}

// FirstFloat parses the first result in a vector query as a float64.
// Returns 0 and no error when the result set is empty (metric not yet scraped).
func FirstFloat(qr *QueryResponse) (float64, error) {
	if len(qr.Data.Result) == 0 {
		return 0, nil
	}
	var s ScalarSample
	if err := json.Unmarshal(qr.Data.Result[0], &s); err != nil {
		return 0, fmt.Errorf("promclient: unmarshal sample: %w", err)
	}
	if len(s.Value) < 2 {
		return 0, nil
	}
	var raw string
	if err := json.Unmarshal(s.Value[1], &raw); err != nil {
		return 0, fmt.Errorf("promclient: unmarshal value string: %w", err)
	}
	var v float64
	if _, err := fmt.Sscanf(raw, "%f", &v); err != nil {
		return 0, fmt.Errorf("promclient: parse float %q: %w", raw, err)
	}
	return v, nil
}
