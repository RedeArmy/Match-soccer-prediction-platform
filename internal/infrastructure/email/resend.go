package email

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const resendBaseURL = "https://api.resend.com"

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
		var resErr resendError
		_ = json.Unmarshal(raw, &resErr)
		if resErr.Message != "" {
			return "", fmt.Errorf("resend: status %d: %s", resp.StatusCode, resErr.Message)
		}
		return "", fmt.Errorf("resend: status %d", resp.StatusCode)
	}

	var result resendResponse
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("resend: parse response: %w", err)
	}
	return result.ID, nil
}

// NoopClient discards every message and returns an empty ID.  Use it in
// tests or when the Resend API key is not configured.
type NoopClient struct{}

// Send is a no-op that always succeeds.
func (NoopClient) Send(_ context.Context, _ Message) (string, error) { return "", nil }
