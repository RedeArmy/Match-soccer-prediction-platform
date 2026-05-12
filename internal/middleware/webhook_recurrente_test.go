package middleware_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

const (
	testRecurrenteSecret = "super-secret-recurrente-key" // NOSONAR: test constant
	testRecurrenteBody   = `{"event_type":"payment.confirmed","data":{"reference":"REF001","amount_cents":5000}}`
)

// signRecurrente computes the HMAC-SHA256 signature expected by RecurrenteWebhookAuth.
func signRecurrente(t *testing.T, body, secret string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

// recurrenteRequest builds a signed POST request for /webhooks/recurrente.
func recurrenteRequest(t *testing.T, body, secret string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		req.Header.Set("X-Recurrente-Hmac-Sha256", signRecurrente(t, body, secret))
	}
	return req
}

// captureHandler is a trivial downstream handler that records the body it received.
type captureHandler struct {
	body []byte
}

func (h *captureHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.body, _ = io.ReadAll(r.Body)
	w.WriteHeader(http.StatusNoContent)
}

func TestRecurrenteWebhookAuth_ValidSignature_Passes(t *testing.T) {
	log := zaptest.NewLogger(t)
	downstream := &captureHandler{}
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(downstream)

	req := recurrenteRequest(t, testRecurrenteBody, testRecurrenteSecret)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_DownstreamReceivesFullBody(t *testing.T) {
	log := zaptest.NewLogger(t)
	downstream := &captureHandler{}
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(downstream)

	req := recurrenteRequest(t, testRecurrenteBody, testRecurrenteSecret)
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if string(downstream.body) != testRecurrenteBody {
		t.Errorf("downstream body = %q, want %q", downstream.body, testRecurrenteBody)
	}
}

func TestRecurrenteWebhookAuth_WrongSignature_Returns401(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("downstream should not be called on invalid signature")
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	req.Header.Set("X-Recurrente-Hmac-Sha256", "deadbeefdeadbeefdeadbeefdeadbeef")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_SignatureForDifferentBody_Returns401(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("downstream should not be called on tampered body")
	}))

	// Sign a different body but send the original.
	tamperedSig := signRecurrente(t, `{"event_type":"payment.refunded"}`, testRecurrenteSecret)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	req.Header.Set("X-Recurrente-Hmac-Sha256", tamperedSig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for tampered body, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_MissingHeader_Returns401(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("downstream should not be called when signature header is absent")
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	// Intentionally no X-Recurrente-Hmac-Sha256 header.

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing header, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_EmptySecret_PassesThrough(t *testing.T) {
	log := zaptest.NewLogger(t)
	called := false
	mw := middleware.RecurrenteWebhookAuth("", log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	// No signature header — should still pass when secret is empty.
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if !called {
		t.Error("downstream was not called when secret is empty (dev mode)")
	}
	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_ErrorResponseIsJSON(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	// No signature header.
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"error"`)) {
		t.Errorf("expected JSON error envelope in body, got: %s", rec.Body.String())
	}
}
