package middleware_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SHA1 needed to test PayPal's SHA1withRSA verification path
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/crc32"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/middleware"
)

const testPayPalWebhookID = "WH-1234567890"

// testKeyPair holds a key pair and the corresponding self-signed cert for tests.
type testKeyPair struct {
	privKey *rsa.PrivateKey
	cert    *x509.Certificate
}

// newTestKeyPair generates a 2048-bit RSA key and a self-signed certificate.
// Using a cached instance per test binary avoids regenerating the key on every
// test case — RSA key generation is the dominant cost in this test suite.
var testPair = func() testKeyPair {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("generating test RSA key: %v", err))
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "PayPal Test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic(fmt.Sprintf("creating test cert: %v", err))
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		panic(fmt.Sprintf("parsing test cert: %v", err))
	}
	return testKeyPair{privKey: key, cert: cert}
}()

// mockFetcher returns a CertFetcher that always returns the test cert,
// regardless of the URL.
func mockFetcher(cert *x509.Certificate) middleware.CertFetcher {
	return func(_ context.Context, _ string) (*x509.Certificate, error) {
		return cert, nil
	}
}

// errorFetcher returns a CertFetcher that always returns the given error.
func errorFetcher(err error) middleware.CertFetcher {
	return func(_ context.Context, _ string) (*x509.Certificate, error) {
		return nil, err
	}
}

// signPayPalMessage signs the PayPal verification message with SHA256withRSA.
func signPayPalMessage(t *testing.T, key *rsa.PrivateKey, transmissionID, transmissionTime, webhookID string, body []byte) string {
	t.Helper()
	crc := crc32.ChecksumIEEE(body)
	msg := fmt.Sprintf("%s|%s|%s|%d", transmissionID, transmissionTime, webhookID, crc)
	h := sha256.New()
	_, _ = h.Write([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, h.Sum(nil))
	if err != nil {
		t.Fatalf("signing PayPal test message: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

// paypalRequestConfig groups the PayPal signature-header parameters for paypalRequest.
type paypalRequestConfig struct {
	transmissionID   string
	transmissionTime string
	certURL          string
	authAlgo         string
	webhookID        string
	key              *rsa.PrivateKey
}

// paypalRequest builds a POST request with all PayPal signature headers set.
func paypalRequest(t *testing.T, body []byte, cfg paypalRequestConfig) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(paypalTransmissionIDHeaderName, cfg.transmissionID)
	req.Header.Set(paypalTransmissionTimeHeaderName, cfg.transmissionTime)
	req.Header.Set(paypalCertURLHeaderName, cfg.certURL)
	req.Header.Set(paypalAuthAlgoHeaderName, cfg.authAlgo)
	req.Header.Set(paypalTransmissionSigHeaderName, signPayPalMessage(t, cfg.key, cfg.transmissionID, cfg.transmissionTime, cfg.webhookID, body))
	return req
}

// Header name constants — local copies so tests don't depend on unexported middleware symbols.
const (
	paypalTransmissionIDHeaderName   = "PAYPAL-TRANSMISSION-ID"
	paypalTransmissionTimeHeaderName = "PAYPAL-TRANSMISSION-TIME"
	paypalCertURLHeaderName          = "PAYPAL-CERT-URL"
	paypalAuthAlgoHeaderName         = "PAYPAL-AUTH-ALGO"
	paypalTransmissionSigHeaderName  = "PAYPAL-TRANSMISSION-SIG"
)

const (
	testPayPalCertURL        = "https://api.paypal.com/v1/notifications/certs/test-cert"
	testPayPalTransmissionID = "tx-abc-123"
	testPayPalBody           = `{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"CAP1"}}`
)

// testPayPalTransmissionTime is generated at package init to ensure it is
// always within the 5-minute PAYPAL-TRANSMISSION-TIME tolerance window.
// A fixed timestamp would become stale and cause all signature tests to fail.
var testPayPalTransmissionTime = time.Now().UTC().Format(time.RFC3339)

func applyPayPalMiddleware(t *testing.T, webhookID string, fetcher middleware.CertFetcher, downstream http.Handler) http.Handler {
	t.Helper()
	return middleware.PayPalWebhookAuth(webhookID, fetcher, zaptest.NewLogger(t))(downstream)
}

// ── Happy path ────────────────────────────────────────────────────────────────

func TestPayPalWebhookAuth_ValidSignature_Passes(t *testing.T) {
	downstream := &captureHandler{}
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert), downstream)

	req := paypalRequest(t, []byte(testPayPalBody), paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256withRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPayPalWebhookAuth_DownstreamReceivesFullBody(t *testing.T) {
	downstream := &captureHandler{}
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert), downstream)

	body := []byte(testPayPalBody)
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256withRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if string(downstream.body) != string(body) {
		t.Errorf("downstream body = %q, want %q", downstream.body, body)
	}
}

// ── Empty webhookID (dev mode) ────────────────────────────────────────────────

func TestPayPalWebhookAuth_EmptyWebhookID_PassesThrough(t *testing.T) {
	called := false
	mw := applyPayPalMiddleware(t, "", mockFetcher(testPair.cert), http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))
	mw.ServeHTTP(httptest.NewRecorder(), req)

	if !called {
		t.Error("downstream was not called when webhookID is empty (dev mode)")
	}
}

// ── Missing headers ───────────────────────────────────────────────────────────

func TestPayPalWebhookAuth_MissingTransmissionID_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called on missing header")
		}),
	)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))
	// Only set some headers — omit PAYPAL-TRANSMISSION-ID.
	req.Header.Set(paypalTransmissionTimeHeaderName, testPayPalTransmissionTime)
	req.Header.Set(paypalCertURLHeaderName, testPayPalCertURL)
	req.Header.Set(paypalAuthAlgoHeaderName, "SHA256withRSA")
	req.Header.Set(paypalTransmissionSigHeaderName, "anysig==")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for missing transmission ID, got %d", rec.Code)
	}
}

func TestPayPalWebhookAuth_NoHeaders_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with no signature headers")
		}),
	)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for no headers, got %d", rec.Code)
	}
}

// ── SSRF guard ────────────────────────────────────────────────────────────────

func TestPayPalWebhookAuth_NonPayPalCertURL_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with non-PayPal cert URL")
		}),
	)
	body := []byte(testPayPalBody)
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          "https://evil.example.com/cert.pem", // not paypal.com
		authAlgo:         "SHA256withRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for non-PayPal cert URL, got %d", rec.Code)
	}
}

func TestPayPalWebhookAuth_HTTPCertURL_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with HTTP cert URL")
		}),
	)
	body := []byte(testPayPalBody)
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          "http://api.paypal.com/v1/notifications/certs/test", // HTTP not HTTPS
		authAlgo:         "SHA256withRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for HTTP cert URL, got %d", rec.Code)
	}
}

// ── Cert fetcher error ────────────────────────────────────────────────────────

func TestPayPalWebhookAuth_FetcherError_Returns500(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID,
		errorFetcher(errors.New("cert download failed")),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called on fetcher error")
		}),
	)
	body := []byte(testPayPalBody)
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256withRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 on fetcher error, got %d", rec.Code)
	}
}

// ── Invalid signature ─────────────────────────────────────────────────────────

func TestPayPalWebhookAuth_WrongSignature_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with wrong signature")
		}),
	)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))
	req.Header.Set(paypalTransmissionIDHeaderName, testPayPalTransmissionID)
	req.Header.Set(paypalTransmissionTimeHeaderName, testPayPalTransmissionTime)
	req.Header.Set(paypalCertURLHeaderName, testPayPalCertURL)
	req.Header.Set(paypalAuthAlgoHeaderName, "SHA256withRSA")
	req.Header.Set(paypalTransmissionSigHeaderName, base64.StdEncoding.EncodeToString([]byte("invalidsignature")))

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong signature, got %d", rec.Code)
	}
}

func TestPayPalWebhookAuth_TamperedBody_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with tampered body")
		}),
	)
	originalBody := []byte(testPayPalBody)
	tamperedBody := []byte(`{"event_type":"PAYMENT.CAPTURE.COMPLETED","resource":{"id":"TAMPERED"}}`)

	// Sign the original body but send the tampered one.
	validSig := signPayPalMessage(t, testPair.privKey,
		testPayPalTransmissionID, testPayPalTransmissionTime,
		testPayPalWebhookID, originalBody,
	)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(tamperedBody))
	req.Header.Set(paypalTransmissionIDHeaderName, testPayPalTransmissionID)
	req.Header.Set(paypalTransmissionTimeHeaderName, testPayPalTransmissionTime)
	req.Header.Set(paypalCertURLHeaderName, testPayPalCertURL)
	req.Header.Set(paypalAuthAlgoHeaderName, "SHA256withRSA")
	req.Header.Set(paypalTransmissionSigHeaderName, validSig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for tampered body, got %d", rec.Code)
	}
}

func TestPayPalWebhookAuth_WrongWebhookID_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with wrong webhookID in signature")
		}),
	)
	body := []byte(testPayPalBody)
	// Sign with a DIFFERENT webhook ID — middleware verifies using testPayPalWebhookID.
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256withRSA",
		webhookID:        "WH-WRONG-ID",
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for wrong webhook ID in signature, got %d", rec.Code)
	}
}

// ── Algorithm variants ────────────────────────────────────────────────────────

// signPayPalMessageSHA1 signs the PayPal verification message with SHA1withRSA.
func signPayPalMessageSHA1(t *testing.T, key *rsa.PrivateKey, transmissionID, transmissionTime, webhookID string, body []byte) string {
	t.Helper()
	crc := crc32.ChecksumIEEE(body)
	msg := fmt.Sprintf("%s|%s|%s|%d", transmissionID, transmissionTime, webhookID, crc)
	h := sha1.New() //nolint:gosec // testing the SHA1withRSA verification path required by PayPal protocol
	_, _ = h.Write([]byte(msg))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA1, h.Sum(nil))
	if err != nil {
		t.Fatalf("signing PayPal message with SHA1: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func TestPayPalWebhookAuth_SHA1withRSA_ValidSignature_Passes(t *testing.T) {
	downstream := &captureHandler{}
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert), downstream)

	body := []byte(testPayPalBody)
	b64sig := signPayPalMessageSHA1(t, testPair.privKey,
		testPayPalTransmissionID, testPayPalTransmissionTime,
		testPayPalWebhookID, body,
	)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", bytes.NewReader(body))
	req.Header.Set(paypalTransmissionIDHeaderName, testPayPalTransmissionID)
	req.Header.Set(paypalTransmissionTimeHeaderName, testPayPalTransmissionTime)
	req.Header.Set(paypalCertURLHeaderName, testPayPalCertURL)
	req.Header.Set(paypalAuthAlgoHeaderName, "SHA1withRSA")
	req.Header.Set(paypalTransmissionSigHeaderName, b64sig)

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("expected 204 for valid SHA1withRSA signature, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPayPalWebhookAuth_UnknownAlgo_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with unsupported algorithm")
		}),
	)
	body := []byte(testPayPalBody)
	// paypalRequest signs with SHA256 — the unsupported algo name triggers the
	// default branch in verifyPayPalSig before any signature math is attempted.
	req := paypalRequest(t, body, paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: testPayPalTransmissionTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "MD5withRSA", // not supported
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for unknown algorithm, got %d", rec.Code)
	}
}

func TestPayPalWebhookAuth_MalformedBase64Sig_Returns401(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			t.Error("downstream called with malformed base64 signature")
		}),
	)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))
	req.Header.Set(paypalTransmissionIDHeaderName, testPayPalTransmissionID)
	req.Header.Set(paypalTransmissionTimeHeaderName, testPayPalTransmissionTime)
	req.Header.Set(paypalCertURLHeaderName, testPayPalCertURL)
	req.Header.Set(paypalAuthAlgoHeaderName, "SHA256withRSA")
	req.Header.Set(paypalTransmissionSigHeaderName, "!!!not-valid-base64!!!")

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for malformed base64 signature, got %d", rec.Code)
	}
}

// ── Error response shape ──────────────────────────────────────────────────────

// TestPayPalWebhookAuth_StaleTimestamp_Returns401 verifies that the full
// PayPalWebhookAuth middleware rejects a request whose PAYPAL-TRANSMISSION-TIME
// is more than five minutes in the past, exercising the timestamp-validation
// error path in checkPayPalWebhook. The signature is built over the stale
// timestamp; signature verification is never reached because the timestamp
// check happens first.
func TestPayPalWebhookAuth_StaleTimestamp_Returns401(t *testing.T) {
	staleTime := time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339)

	downstream := &captureHandler{}
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert), downstream)

	req := paypalRequest(t, []byte(testPayPalBody), paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: staleTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256WITHRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("stale timestamp: expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
	if len(downstream.body) != 0 {
		t.Error("downstream handler must not be called when timestamp is rejected")
	}
}

// TestPayPalWebhookAuth_FutureTimestamp_Returns401 verifies that a timestamp
// more than five minutes in the future is also rejected, preventing clock-skew
// abuse.
func TestPayPalWebhookAuth_FutureTimestamp_Returns401(t *testing.T) {
	futureTime := time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339)

	downstream := &captureHandler{}
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert), downstream)

	req := paypalRequest(t, []byte(testPayPalBody), paypalRequestConfig{
		transmissionID:   testPayPalTransmissionID,
		transmissionTime: futureTime,
		certURL:          testPayPalCertURL,
		authAlgo:         "SHA256WITHRSA",
		webhookID:        testPayPalWebhookID,
		key:              testPair.privKey,
	})

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("future timestamp: expected 401, got %d; body: %s", rec.Code, rec.Body.String())
	}
}

func TestPayPalWebhookAuth_ErrorResponseIsJSON(t *testing.T) {
	mw := applyPayPalMiddleware(t, testPayPalWebhookID, mockFetcher(testPair.cert),
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}),
	)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/paypal", strings.NewReader(testPayPalBody))
	// No headers — triggers 401.

	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "application/json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Errorf("response is not valid JSON: %v; body: %s", err, rec.Body.String())
	}
	if _, ok := resp["error"]; !ok {
		t.Errorf("expected 'error' key in JSON response, got: %v", resp)
	}
}
