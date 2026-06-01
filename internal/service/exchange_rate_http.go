package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// fetchJSON executes a GET request to url using client, asserts the response
// status is 200, reads at most limit bytes from the body, and JSON-unmarshals
// the result into dest.  All errors are prefixed with source so callers can
// identify the provider without adding a second fmt.Errorf wrap.
func fetchJSON(ctx context.Context, client *http.Client, source, url string, limit int64, dest any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("%s: build request: %w", source, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("%s: GET: %w", source, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: unexpected status %d", source, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return fmt.Errorf("%s: read body: %w", source, err)
	}

	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("%s: parse JSON: %w", source, err)
	}
	return nil
}
