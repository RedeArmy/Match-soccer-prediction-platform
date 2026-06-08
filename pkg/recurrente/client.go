// Package recurrente provides a minimal HTTP client for the Recurrente
// payments API, used to create hosted checkout sessions.
//
// The Recurrente API is documented at https://docs.recurrente.com/api.
// Authentication is via a Bearer API key obtained from the Recurrente
// dashboard (Configuración → Desarrolladores y API → API Keys).
package recurrente

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultBaseURL = "https://app.recurrente.com/api"

// Client calls the Recurrente REST API on behalf of the merchant.
type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

// New constructs a Client authenticated with apiKey.
// baseURL defaults to the production Recurrente API when empty.
func New(apiKey, baseURL string) *Client {
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	return &Client{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 15 * time.Second},
	}
}

// CheckoutItem is a single line item for a checkout session.
type CheckoutItem struct {
	Name          string `json:"name"`
	AmountInCents int    `json:"amount_in_cents"`
	Currency      string `json:"currency"`
	Quantity      int    `json:"quantity"`
}

// CheckoutRequest is the payload sent to POST /api/checkouts.
type CheckoutRequest struct {
	Items      []CheckoutItem    `json:"items"`
	SuccessURL string            `json:"success_url"`
	CancelURL  string            `json:"cancel_url"`
	Metadata   map[string]any    `json:"metadata,omitempty"`
}

// CheckoutResponse is the relevant subset of the Recurrente checkout object.
type CheckoutResponse struct {
	ID          string `json:"id"`
	CheckoutURL string `json:"checkout_url"` // hosted payment page; redirect the user here
	LiveMode    bool   `json:"live_mode"`    // false for TEST-key checkouts
	Status      string `json:"status"`
}

// CreateCheckout opens a new hosted checkout session and returns the URL to
// redirect the user to. The caller is responsible for setting Metadata fields
// (e.g. wcq_user_id, wcq_reference) so that the incoming webhook can credit
// the correct user.
func (c *Client) CreateCheckout(ctx context.Context, req CheckoutRequest) (*CheckoutResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("recurrente: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/checkouts", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("recurrente: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-SECRET-KEY", c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("recurrente: send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return nil, fmt.Errorf("recurrente: read response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("recurrente: unexpected status %d: %s", resp.StatusCode, respBody)
	}

	var out CheckoutResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		return nil, fmt.Errorf("recurrente: decode response: %w", err)
	}
	if out.CheckoutURL == "" {
		return nil, fmt.Errorf("recurrente: response missing checkout_url field")
	}
	return &out, nil
}
