package middleware_test

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

const (
	testRecurrenteSecret = "super-secret-recurrente-key" // NOSONAR: test constant
	testRecurrenteBody   = `{"event_type":"payment.confirmed","data":{"reference":"REF001","amount_cents":5000}}`
)

// svixSign computes the Svix HMAC-SHA256 signature over the signed-content string
// msgID + "." + timestamp + "." + body. The raw secret is used as the key directly
// (mirrors the middleware's legacy-secret path). Returns a base64-encoded string.
func svixSign(t *testing.T, secret, msgID, timestamp string, body []byte) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(msgID))
	mac.Write([]byte("."))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

// svixRequest builds a signed POST request for /webhooks/recurrente.
// When secret is non-empty it sets all three Svix signing headers.
func svixRequest(t *testing.T, body []byte, secret string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if secret != "" {
		msgID := "msg_test_" + t.Name()
		timestamp := strconv.FormatInt(time.Now().Unix(), 10)
		sig := svixSign(t, secret, msgID, timestamp, body)
		req.Header.Set("svix-id", msgID)
		req.Header.Set("svix-timestamp", timestamp)
		req.Header.Set("svix-signature", "v1,"+sig)
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

	req := svixRequest(t, []byte(testRecurrenteBody), testRecurrenteSecret)
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

	req := svixRequest(t, []byte(testRecurrenteBody), testRecurrenteSecret)
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
	req.Header.Set("svix-id", "msg_wrong")
	req.Header.Set("svix-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	req.Header.Set("svix-signature", "v1,deadbeefdeadbeefdeadbeefdeadbeefdeadbeef==")

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

	// Sign a different body but send the actual body.
	msgID := "msg_tampered"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	tamperedSig := svixSign(t, testRecurrenteSecret, msgID, timestamp, []byte(`{"event_type":"payment.refunded"}`))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	req.Header.Set("svix-id", msgID)
	req.Header.Set("svix-timestamp", timestamp)
	req.Header.Set("svix-signature", "v1,"+tamperedSig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for tampered body, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_MissingHeaders_Returns401(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("downstream should not be called when Svix headers are absent")
	}))

	// No svix-id / svix-timestamp / svix-signature headers.
	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing headers, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_ExpiredTimestamp_Returns401(t *testing.T) {
	log := zaptest.NewLogger(t)
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("downstream should not be called for stale timestamp")
	}))

	// Timestamp 10 minutes in the past — outside the 5-minute tolerance window.
	msgID := "msg_stale"
	timestamp := strconv.FormatInt(time.Now().Add(-10*time.Minute).Unix(), 10)
	sig := svixSign(t, testRecurrenteSecret, msgID, timestamp, []byte(testRecurrenteBody))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", strings.NewReader(testRecurrenteBody))
	req.Header.Set("svix-id", msgID)
	req.Header.Set("svix-timestamp", timestamp)
	req.Header.Set("svix-signature", "v1,"+sig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired timestamp, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_WhsecSecret_Passes(t *testing.T) {
	// Verify that whsec_<base64> format secrets are decoded and used correctly.
	rawKey := []byte("thirty-two-byte-key-for-testing!")
	b64Key := base64.StdEncoding.EncodeToString(rawKey)
	secret := "whsec_" + b64Key

	log := zaptest.NewLogger(t)
	downstream := &captureHandler{}
	mw := middleware.RecurrenteWebhookAuth(secret, log)(downstream)

	body := []byte(testRecurrenteBody)
	msgID := "msg_whsec_test"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)

	// Compute expected signature using the decoded key (not the raw secret string).
	mac := hmac.New(sha256.New, rawKey)
	mac.Write([]byte(msgID))
	mac.Write([]byte("."))
	mac.Write([]byte(timestamp))
	mac.Write([]byte("."))
	mac.Write(body)
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader(body))
	req.Header.Set("svix-id", msgID)
	req.Header.Set("svix-timestamp", timestamp)
	req.Header.Set("svix-signature", "v1,"+sig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 with whsec_ secret, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_MultipleSignatureTokens_AnyMatchPasses(t *testing.T) {
	// Svix can send multiple v1,<sig> tokens during key rotation; any match must pass.
	log := zaptest.NewLogger(t)
	downstream := &captureHandler{}
	mw := middleware.RecurrenteWebhookAuth(testRecurrenteSecret, log)(downstream)

	body := []byte(testRecurrenteBody)
	msgID := "msg_rotation"
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	validSig := svixSign(t, testRecurrenteSecret, msgID, timestamp, body)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/recurrente", bytes.NewReader(body))
	req.Header.Set("svix-id", msgID)
	req.Header.Set("svix-timestamp", timestamp)
	// First token is garbage; second is valid.
	req.Header.Set("svix-signature", "v1,aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa== v1,"+validSig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 when any token matches, got %d", rec.Code)
	}
}

func TestRecurrenteWebhookAuth_EmptySecret_PassesThrough(t *testing.T) {
	log := zaptest.NewLogger(t)
	called := false
	mw := middleware.RecurrenteWebhookAuth("", log)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	// No signing headers — should still pass when secret is empty (dev bypass).
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
	// No Svix headers → 401.
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
