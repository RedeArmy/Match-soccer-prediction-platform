package handler_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/middleware"
)

// stampVerifiedBody is a test-only middleware that buffers the request body
// and stamps it into the context as the verified-body sentinel. It simulates
// what RecurrenteWebhookAuth / PayPalWebhookAuth do after signature
// verification so that unit tests can exercise handler logic without wiring
// up real HMAC/RSA verification.
func stampVerifiedBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r = r.WithContext(middleware.SetWebhookVerifiedBody(r.Context(), body))
		next.ServeHTTP(w, r)
	})
}

func webhookRouter(t *testing.T, svc *stubWebhookPaymentSvc) http.Handler {
	t.Helper()
	h := handler.NewPaymentWebhookHandler(svc, zaptest.NewLogger(t))
	mux := http.NewServeMux()
	mux.Handle("POST /webhooks/recurrente", stampVerifiedBody(http.HandlerFunc(h.HandleRecurrente)))
	mux.Handle("POST /webhooks/paypal", stampVerifiedBody(http.HandlerFunc(h.HandlePayPal)))
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
	body := []byte("not-json")
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader(body))
	req = req.WithContext(middleware.SetWebhookVerifiedBody(req.Context(), body))
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

func TestWebhookHandler_Recurrente_MissingVerifiedBody_Returns401(t *testing.T) {
	h := handler.NewPaymentWebhookHandler(&stubWebhookPaymentSvc{}, zaptest.NewLogger(t))
	rec := httptest.NewRecorder()
	payload := map[string]any{
		"event_type": "payment.confirmed",
		"data":       map[string]any{"reference": "REF001", "amount_cents": 5000, "user_id": 1},
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader(b))
	h.HandleRecurrente(rec, req) // no stampVerifiedBody middleware
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when context sentinel absent, got %d", rec.Code)
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
			"amount":    map[string]any{"value": "50.00", "currency_code": "USD"},
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
			"amount":    map[string]any{"value": "10.00", "currency_code": "USD"},
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on service error, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_MissingVerifiedBody_Returns401(t *testing.T) {
	h := handler.NewPaymentWebhookHandler(&stubWebhookPaymentSvc{}, zaptest.NewLogger(t))
	rec := httptest.NewRecorder()
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource":   map[string]any{"id": "CAP1", "custom_id": testIntentToken},
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(b))
	h.HandlePayPal(rec, req) // no stampVerifiedBody middleware
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 when context sentinel absent, got %d", rec.Code)
	}
}

func TestWebhookHandler_PayPal_MalformedAmount_StillReturns204(t *testing.T) {
	router := webhookRouter(t, &stubWebhookPaymentSvc{})
	payload := map[string]any{
		"event_type": "PAYMENT.CAPTURE.COMPLETED",
		"resource": map[string]any{
			"id":        "CAP-AMT",
			"custom_id": testIntentToken,
			"amount":    map[string]any{"value": "not-a-number"},
		},
	}
	rec := postJSON(t, router, "/webhooks/paypal", payload)
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 even with malformed amount (non-fatal), got %d", rec.Code)
	}
}
