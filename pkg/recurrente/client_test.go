package recurrente_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/recurrente"
)

func TestCreateCheckout_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/checkouts" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("X-SECRET-KEY") != "test-key" {
			t.Errorf("unexpected X-SECRET-KEY header: %s", r.Header.Get("X-SECRET-KEY"))
		}

		var req recurrente.CheckoutRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if len(req.Items) != 1 || req.Items[0].AmountInCents != 10000 {
			t.Errorf("unexpected items: %+v", req.Items)
		}
		if req.Metadata["wcq_user_id"] == nil {
			t.Error("metadata missing wcq_user_id")
		}

		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "ch_test123",
			"checkout_url": "https://app.recurrente.com/c/ch_test123",
			"live_mode":    false,
			"status":       "created",
		})
	}))
	defer srv.Close()

	client := recurrente.New("test-key", srv.URL)
	resp, err := client.CreateCheckout(context.Background(), recurrente.CheckoutRequest{
		Items: []recurrente.CheckoutItem{{
			Name:          "Depósito",
			AmountInCents: 10000,
			Currency:      "GTQ",
			Quantity:      1,
		}},
		SuccessURL: "https://example.com/ok",
		CancelURL:  "https://example.com/cancel",
		Metadata:   map[string]any{"wcq_user_id": 42, "wcq_reference": "ref-001"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.CheckoutURL != "https://app.recurrente.com/c/ch_test123" {
		t.Errorf("unexpected CheckoutURL: %s", resp.CheckoutURL)
	}
	if resp.ID != "ch_test123" {
		t.Errorf("unexpected ID: %s", resp.ID)
	}
}

func TestCreateCheckout_Non2xx_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_api_key"}`, http.StatusUnauthorized)
	}))
	defer srv.Close()

	client := recurrente.New("bad-key", srv.URL)
	_, err := client.CreateCheckout(context.Background(), recurrente.CheckoutRequest{
		Items:      []recurrente.CheckoutItem{{Name: "x", AmountInCents: 100, Currency: "GTQ", Quantity: 1}},
		SuccessURL: "https://example.com/ok",
		CancelURL:  "https://example.com/cancel",
	})
	if err == nil {
		t.Fatal("expected error for 401 response, got nil")
	}
}

func TestCreateCheckout_MissingURL_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"ch_x","status":"created"}`)) // checkout_url absent
	}))
	defer srv.Close()

	client := recurrente.New("test-key", srv.URL)
	_, err := client.CreateCheckout(context.Background(), recurrente.CheckoutRequest{
		Items:      []recurrente.CheckoutItem{{Name: "x", AmountInCents: 100, Currency: "GTQ", Quantity: 1}},
		SuccessURL: "https://example.com/ok",
		CancelURL:  "https://example.com/cancel",
	})
	if err == nil {
		t.Fatal("expected error for missing url, got nil")
	}
}

func TestNew_DefaultBaseURL(t *testing.T) {
	// Smoke-test that empty baseURL uses the production default without panicking.
	c := recurrente.New("key", "")
	if c == nil {
		t.Fatal("New returned nil")
	}
}
