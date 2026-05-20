package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const resendBaseURL = "https://api.resend.com"

// RetryAfterError is returned when the API responds HTTP 429 Too Many Requests.
// RetryAfter is parsed from the Retry-After header when present; otherwise zero,
// signalling the caller should apply its own back-off strategy.
type RetryAfterError struct {
	RetryAfter time.Duration
	Msg        string
}

func (e *RetryAfterError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("email: rate limited (retry after %s): %s", e.RetryAfter, e.Msg)
	}
	return fmt.Sprintf("email: rate limited: %s", e.Msg)
}

// ResendClient delivers email via the Resend API (https://resend.com).
// Construct with NewResendClient; zero value is not usable.
type ResendClient struct {
	apiKey  string
	baseURL string // overridable in tests
	http    *http.Client
}

// NewResendClient constructs a ResendClient.  apiKey must be non-empty;
// passing an empty key results in 401 responses from the API.
func NewResendClient(apiKey string) *ResendClient {
	return NewResendClientWithBaseURL(apiKey, resendBaseURL)
}

// NewResendClientWithBaseURL constructs a ResendClient targeting a custom base
// URL.  Use this in tests to point the client at a local httptest.Server.
func NewResendClientWithBaseURL(apiKey, baseURL string) *ResendClient {
	return &ResendClient{
		apiKey:  apiKey,
		baseURL: baseURL,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

type resendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Subject string   `json:"subject"`
	HTML    string   `json:"html"`
}

type resendResponse struct {
	ID string `json:"id"`
}

type resendError struct {
	Name       string `json:"name"`
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}

// Send delivers msg via the Resend API.  It returns the Resend message ID on
// success and a descriptive error otherwise.  The error includes the HTTP
// status code so callers can distinguish retryable (5xx) from permanent (4xx)
// failures.
func (c *ResendClient) Send(ctx context.Context, msg Message) (string, error) {
	body, err := json.Marshal(resendRequest(msg))
	if err != nil {
		return "", fmt.Errorf("resend: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("resend: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("resend: http: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", parseAPIError(resp, raw)
	}

	var result resendResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("resend: parse response: %w", err)
	}
	return result.ID, nil
}

// parseAPIError builds a typed error from a non-2xx Resend API response.
func parseAPIError(resp *http.Response, raw []byte) error {
	var resErr resendError
	_ = json.Unmarshal(raw, &resErr)
	msg := resErr.Message
	if msg == "" {
		msg = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return &RetryAfterError{RetryAfter: parseRetryAfter(resp.Header), Msg: msg}
	}
	return fmt.Errorf("resend: status %d: %s", resp.StatusCode, msg)
}

// parseRetryAfter extracts a duration from the Retry-After header, returning
// zero if the header is absent or unparseable.
func parseRetryAfter(h http.Header) time.Duration {
	s := h.Get("Retry-After")
	if s == "" {
		return 0
	}
	secs, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return time.Duration(secs) * time.Second
}

// NoopClient discards every message and returns an empty ID.  Use it in
// tests or when the Resend API key is not configured.
type NoopClient struct{}

// Send is a no-op that always succeeds.
func (NoopClient) Send(_ context.Context, _ Message) (string, error) { return "", nil }
