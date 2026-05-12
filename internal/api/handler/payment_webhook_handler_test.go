package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
)

func webhookRouter(t *testing.T, svc *stubWebhookPaymentSvc) http.Handler {
	t.Helper()
	h := handler.NewPaymentWebhookHandler(svc, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /webhooks/recurrente", h.HandleRecurrente)
	mux.HandleFunc("POST /webhooks/paypal", h.HandlePayPal)
	return mux
}

func postJSON(t *testing.T, router http.Handler, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	b, _ := json.Marshal(body)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(rec, req)
	return rec
}

// ── Recurrente ────────────────────────────────────────────────────────────────

func TestWebhookHandler_Recurrente_PaymentConfirmed_Returns204(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data":       map[string]any{"reference": "REF001", "amount_cents": 5000, "currency": "GTQ", "user_id": 42},
	}
	rec := postJSON(t, router, "/webhooks/recurrente", payload)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestWebhookHandler_Recurrente_IgnoresOtherEvents(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{"event_type": "payment.refunded", "data": map[string]any{}}
	rec := postJSON(t, router, "/webhooks/recurrente", payload)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for ignored event, got %d", rec.Code)
	}
}

func TestWebhookHandler_Recurrente_MissingFields_Returns422(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data":       map[string]any{"reference": "", "amount_cents": 0, "user_id": 0},
	}
	rec := postJSON(t, router, "/webhooks/recurrente", payload)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for missing fields, got %d", rec.Code)
	}
}

func TestWebhookHandler_Recurrente_InvalidJSON_Returns422(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader([]byte("not-json")))
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for invalid JSON, got %d", rec.Code)
	}
}

func TestWebhookHandler_Recurrente_ServiceError_Returns500(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{err: errors.New("db error")})
	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data":       map[string]any{"reference": "REF002", "amount_cents": 1000, "currency": "GTQ", "user_id": 1},
	}
	rec := postJSON(t, router, "/webhooks/recurrente", payload)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}

// ── PayPal ────────────────────────────────────────────────────────────────────

const testIntentToken = "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

func TestWebhookHandler_PayPal_CaptureCompleted_Returns204(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "CAPTURE123",
			"custom_id": testIntentToken,
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_IgnoresOtherEvents(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{"event_type": "PAYMENT.CAPTURE.DENIED", "resource": map[string]any{}}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for ignored event, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_EmptyCustomID_Returns422(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "CAP-X",
			"custom_id": "",
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty custom_id, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_EmptyCaptureID_Returns422(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "",
			"custom_id": testIntentToken,
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422 for empty capture ID, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_ServiceError_Returns500(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{err: errors.New("db error")})
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "CAP999",
			"custom_id": testIntentToken,
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}
