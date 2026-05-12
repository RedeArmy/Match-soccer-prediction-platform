package middleware

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha1" //nolint:gosec // SHA1 is required for PayPal's SHA1withRSA algorithm
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"hash"
	"hash/crc32"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	paypalTransmissionIDHeader   = "PAYPAL-TRANSMISSION-ID"
	paypalTransmissionTimeHeader = "PAYPAL-TRANSMISSION-TIME"
	paypalCertURLHeader          = "PAYPAL-CERT-URL"
	paypalAuthAlgoHeader         = "PAYPAL-AUTH-ALGO"
	paypalTransmissionSigHeader  = "PAYPAL-TRANSMISSION-SIG"
	paypalCertBodyLimit          = 16 << 10 // 16 KB — generous for a PEM certificate
)

// CertFetcher retrieves a parsed X.509 certificate from the given URL.
// Implementations must be safe for concurrent use from multiple goroutines.
type CertFetcher func(ctx context.Context, certURL string) (*x509.Certificate, error)

// DefaultPayPalCertFetcher returns a CertFetcher backed by a package-level
// in-memory cache. PayPal cert URLs are versioned: a new URL is issued when
// the certificate rotates, so cached entries are never stale. The underlying
// HTTP client enforces a 10-second timeout on cert downloads.
func DefaultPayPalCertFetcher() CertFetcher {
	return defaultPayPalFetcher.fetch
}

var defaultPayPalFetcher = &cachedCertFetcher{
	client: &http.Client{Timeout: 10 * time.Second},
}

type cachedCertFetcher struct {
	cache  sync.Map // key: certURL string → value: *x509.Certificate
	client *http.Client
}

func (c *cachedCertFetcher) fetch(ctx context.Context, certURL string) (*x509.Certificate, error) {
	if v, ok := c.cache.Load(certURL); ok {
		return v.(*x509.Certificate), nil
	}
	cert, err := downloadCert(ctx, c.client, certURL)
	if err != nil {
		return nil, err
	}
	c.cache.Store(certURL, cert)
	return cert, nil
}

func downloadCert(ctx context.Context, client *http.Client, certURL string) (*x509.Certificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, certURL, nil)
	if err != nil {
		return nil, fmt.Errorf("building cert request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching PayPal cert: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP %d fetching PayPal cert from %s", resp.StatusCode, certURL)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, paypalCertBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("reading PayPal cert body: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in PayPal cert response from %s", certURL)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parsing PayPal cert: %w", err)
	}
	return cert, nil
}

// PayPalWebhookAuth returns middleware that verifies the RSA certificate-based
// signature on PayPal webhook requests.
//
// Verification follows PayPal's documented algorithm:
//  1. Validate that PAYPAL-CERT-URL is HTTPS and from *.paypal.com (SSRF guard).
//  2. Download and cache the signing certificate from PAYPAL-CERT-URL.
//  3. Compute CRC32(IEEE) of the raw request body.
//  4. Build the signed message: {transmissionID}|{time}|{webhookID}|{crc32}.
//  5. Decode the base64 signature from PAYPAL-TRANSMISSION-SIG.
//  6. Verify the RSA PKCS#1 v1.5 signature using the algorithm in PAYPAL-AUTH-ALGO.
//
// When webhookID is empty the middleware logs a warning and passes all requests
// through without verification. This is acceptable for local development only;
// pkg/config/validation.go rejects an empty webhook ID in non-development
// environments.
func PayPalWebhookAuth(webhookID string, fetcher CertFetcher, log *zap.Logger) func(http.Handler) http.Handler {
	if webhookID == "" {
		log.Warn("PayPalWebhookAuth: WCQ_PAYMENT_PAYPALWEBHOOKID is not set — signature verification DISABLED; do not use in production")
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if webhookID == "" {
				next.ServeHTTP(w, r)
				return
			}

			body, err := io.ReadAll(io.LimitReader(r.Body, webhookBodyLimit))
			if err != nil {
				WriteError(w, r, log, apperrors.Internal(err))
				return
			}
			// Restore the body so the downstream handler can re-read it.
			r.Body = io.NopCloser(bytes.NewReader(body))

			transmissionID := r.Header.Get(paypalTransmissionIDHeader)
			transmissionTime := r.Header.Get(paypalTransmissionTimeHeader)
			certURL := r.Header.Get(paypalCertURLHeader)
			authAlgo := r.Header.Get(paypalAuthAlgoHeader)
			transmissionSig := r.Header.Get(paypalTransmissionSigHeader)

			if transmissionID == "" || transmissionTime == "" || certURL == "" || authAlgo == "" || transmissionSig == "" {
				log.Warn("paypal webhook: missing required signature headers",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.Bool("has_transmission_id", transmissionID != ""),
					zap.Bool("has_transmission_time", transmissionTime != ""),
					zap.Bool("has_cert_url", certURL != ""),
					zap.Bool("has_auth_algo", authAlgo != ""),
					zap.Bool("has_transmission_sig", transmissionSig != ""),
				)
				WriteError(w, r, log, apperrors.Unauthorised("missing PayPal signature headers"))
				return
			}

			// Guard against SSRF: only accept cert URLs from paypal.com.
			if err := validatePayPalCertURL(certURL); err != nil {
				log.Warn("paypal webhook: rejected cert URL",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.String("cert_url", certURL),
					zap.Error(err),
				)
				WriteError(w, r, log, apperrors.Unauthorised("invalid cert URL"))
				return
			}

			cert, err := fetcher(r.Context(), certURL)
			if err != nil {
				log.Error("paypal webhook: failed to fetch signing cert",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.String("cert_url", certURL),
					zap.Error(err),
				)
				WriteError(w, r, log, apperrors.Internal(err))
				return
			}

			// Build the signed message per PayPal's spec.
			bodyCRC := crc32.ChecksumIEEE(body)
			message := fmt.Sprintf("%s|%s|%s|%d", transmissionID, transmissionTime, webhookID, bodyCRC)

			sigBytes, err := base64.StdEncoding.DecodeString(transmissionSig)
			if err != nil {
				log.Warn("paypal webhook: cannot base64-decode signature",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.Error(err),
				)
				WriteError(w, r, log, apperrors.Unauthorised("malformed webhook signature"))
				return
			}

			if err := verifyPayPalSig(cert, authAlgo, []byte(message), sigBytes); err != nil {
				log.Warn("paypal webhook: signature verification failed",
					zap.String("request_id", GetRequestID(r.Context())),
					zap.String("auth_algo", authAlgo),
					zap.Error(err),
				)
				WriteError(w, r, log, apperrors.Unauthorised("invalid webhook signature"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// validatePayPalCertURL ensures the cert URL is HTTPS and from paypal.com.
// This prevents SSRF attacks where a forged request could embed an arbitrary
// URL pointing to a controlled server with a self-crafted certificate.
func validatePayPalCertURL(certURL string) error {
	u, err := url.Parse(certURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "https" {
		return fmt.Errorf("cert URL scheme must be https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host != "paypal.com" && !strings.HasSuffix(host, ".paypal.com") {
		return fmt.Errorf("cert URL host must be paypal.com or a subdomain, got %q", host)
	}
	return nil
}

// verifyPayPalSig verifies an RSA PKCS#1 v1.5 signature over message using
// the public key from cert. The hash algorithm is derived from authAlgo
// (e.g. "SHA256withRSA", "SHA1withRSA").
func verifyPayPalSig(cert *x509.Certificate, authAlgo string, message, sig []byte) error {
	rsaKey, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok {
		return fmt.Errorf("PayPal cert does not contain an RSA public key")
	}

	var (
		hashID crypto.Hash
		h      hash.Hash
	)
	switch strings.ToUpper(authAlgo) {
	case "SHA256WITHRSA":
		hashID = crypto.SHA256
		h = sha256.New()
	case "SHA1WITHRSA":
		hashID = crypto.SHA1
		h = sha1.New() //nolint:gosec // SHA1 is mandated by PayPal's protocol
	default:
		return fmt.Errorf("unsupported PayPal auth algorithm: %q", authAlgo)
	}

	_, _ = h.Write(message)
	return rsa.VerifyPKCS1v15(rsaKey, hashID, h.Sum(nil), sig)
}
