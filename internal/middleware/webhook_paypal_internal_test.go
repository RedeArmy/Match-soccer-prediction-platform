package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// internalTestCert is a package-level self-signed cert PEM generated once for
// all internal webhook_paypal tests. Keeps RSA keygen out of the hot path.
var internalTestCertPEM = func() []byte {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic("generating internal test RSA key: " + err.Error())
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Internal Test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		panic("creating internal test cert: " + err.Error())
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}()

func newCertServer(certPEM []byte) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(certPEM)
	}))
}

// ── downloadCert ──────────────────────────────────────────────────────────────

func TestDownloadCert_Success(t *testing.T) {
	srv := newCertServer(internalTestCertPEM)
	defer srv.Close()

	cert, err := downloadCert(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cert == nil {
		t.Fatal("returned nil cert")
	}
}

func TestDownloadCert_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := downloadCert(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestDownloadCert_NonPEMBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("this is not a PEM block"))
	}))
	defer srv.Close()

	_, err := downloadCert(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for non-PEM body, got nil")
	}
}

func TestDownloadCert_InvalidURL(t *testing.T) {
	// A URL with a space in the scheme triggers http.NewRequestWithContext error.
	_, err := downloadCert(context.Background(), http.DefaultClient, "ht tp://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

// ── cachedCertFetcher ────────────────────────────────────────────────────────

func TestCachedCertFetcher_FetchFromServer(t *testing.T) {
	srv := newCertServer(internalTestCertPEM)
	defer srv.Close()

	fetcher := &cachedCertFetcher{client: srv.Client()}
	cert, err := fetcher.fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("fetch error: %v", err)
	}
	if cert == nil {
		t.Fatal("fetch returned nil cert")
	}
}

func TestCachedCertFetcher_CacheHit(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		_, _ = w.Write(internalTestCertPEM)
	}))
	defer srv.Close()

	fetcher := &cachedCertFetcher{client: srv.Client()}

	if _, err := fetcher.fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := fetcher.fetch(context.Background(), srv.URL); err != nil {
		t.Fatalf("second fetch: %v", err)
	}

	if requestCount != 1 {
		t.Errorf("expected exactly 1 HTTP request, got %d (cache miss)", requestCount)
	}
}

func TestCachedCertFetcher_FetchError_NotCached(t *testing.T) {
	requestCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requestCount++
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	fetcher := &cachedCertFetcher{client: srv.Client()}

	// Both calls should reach the server since the first errored and was not cached.
	_, _ = fetcher.fetch(context.Background(), srv.URL)
	_, _ = fetcher.fetch(context.Background(), srv.URL)

	if requestCount != 2 {
		t.Errorf("expected 2 HTTP requests after back-to-back errors, got %d", requestCount)
	}
}

// ── DefaultPayPalCertFetcher ──────────────────────────────────────────────────

func TestDefaultPayPalCertFetcher_ReturnsCallable(t *testing.T) {
	f := DefaultPayPalCertFetcher()
	if f == nil {
		t.Fatal("DefaultPayPalCertFetcher returned nil")
	}
}

// ── validatePayPalTimestamp ───────────────────────────────────────────────────

func TestValidatePayPalTimestamp_ValidRecentTimestamp_ReturnsNil(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(-30 * time.Second).Format(time.RFC3339)
	if err := validatePayPalTimestamp(ts, now); err != nil {
		t.Errorf("expected nil for 30-second-old timestamp, got: %v", err)
	}
}

func TestValidatePayPalTimestamp_TooOld_ReturnsError(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(-6 * time.Minute).Format(time.RFC3339)
	if err := validatePayPalTimestamp(ts, now); err == nil {
		t.Error("expected error for 6-minute-old timestamp, got nil")
	}
}

func TestValidatePayPalTimestamp_TooFarFuture_ReturnsError(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(6 * time.Minute).Format(time.RFC3339)
	if err := validatePayPalTimestamp(ts, now); err == nil {
		t.Error("expected error for timestamp 6 minutes in the future, got nil")
	}
}

func TestValidatePayPalTimestamp_ExactlyAtTolerance_ReturnsNil(t *testing.T) {
	now := time.Now().UTC()
	ts := now.Add(-(paypalTimestampTolerance - time.Second)).Format(time.RFC3339)
	if err := validatePayPalTimestamp(ts, now); err != nil {
		t.Errorf("expected nil at exact tolerance boundary, got: %v", err)
	}
}

func TestValidatePayPalTimestamp_MalformedString_ReturnsError(t *testing.T) {
	if err := validatePayPalTimestamp("not-a-date", time.Now()); err == nil {
		t.Error("expected error for malformed timestamp, got nil")
	}
}

func TestValidatePayPalTimestamp_Empty_ReturnsError(t *testing.T) {
	if err := validatePayPalTimestamp("", time.Now()); err == nil {
		t.Error("expected error for empty timestamp, got nil")
	}
}
