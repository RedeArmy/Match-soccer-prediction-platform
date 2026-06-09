package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/pkg/recurrente"
)

func TestRecurrenteCheckoutAdapter_CreateCheckout_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":           "ch_123",
			"checkout_url": "https://checkout.recurrente.com/ch_123",
			"live_mode":    false,
			"status":       "open",
		})
	}))
	defer srv.Close()

	client := recurrente.New("test_key", srv.URL)
	adapter := handler.NewRecurrenteCheckoutAdapter(client)

	url, err := adapter.CreateCheckout(
		context.Background(), 1, 5000, "GTQ", "REF-001",
		"https://app.test/success", "https://app.test/cancel",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if url == "" {
		t.Fatal("expected non-empty checkout URL")
	}
}

func TestRecurrenteCheckoutAdapter_CreateCheckout_ClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream error"))
	}))
	defer srv.Close()

	client := recurrente.New("test_key", srv.URL)
	adapter := handler.NewRecurrenteCheckoutAdapter(client)

	_, err := adapter.CreateCheckout(
		context.Background(), 1, 5000, "GTQ", "REF-001",
		"https://app.test/success", "https://app.test/cancel",
	)
	if err == nil {
		t.Fatal("expected error on 500 response, got nil")
	}
}
