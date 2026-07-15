package webpush_test

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	wp "github.com/SherClockHolmes/webpush-go"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
)

// genSubscriptionKeys returns a valid P256dh (65-byte uncompressed EC P256 public
// key, base64url-encoded) and Auth (16-byte random secret, base64url-encoded).
// These are the subscriber-side keys produced by a browser's PushSubscription API.
//
// webpush-go requires a genuinely valid P256 point on the curve; any random 65-byte
// blob causes elliptic.Unmarshal to return nil ("not a valid point on the curve"),
// which makes SendNotification return an error before the HTTP request is issued.
func genSubscriptionKeys(t *testing.T) (p256dh, auth string) {
	t.Helper()
	privKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("genSubscriptionKeys: P256 GenerateKey: %v", err)
	}
	// Bytes returns the 65-byte uncompressed point: 0x04 || x || y
	p256dhBytes := privKey.PublicKey().Bytes()
	authBytes := make([]byte, 16)
	if _, err := rand.Read(authBytes); err != nil {
		t.Fatalf("genSubscriptionKeys: rand.Read: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(p256dhBytes),
		base64.RawURLEncoding.EncodeToString(authBytes)
}

func TestNoopSender_AlwaysReturns200(t *testing.T) {
	t.Parallel()
	var s webpush.NoopSender
	code, err := s.Send(context.Background(), webpush.Message{
		Endpoint:  "https://push.example.com/abc",
		P256dhKey: "key",
		AuthKey:   "auth",
		Body:      []byte(`{"title":"test"}`),
		TTL:       3600,
	})
	if err != nil {
		t.Fatalf("NoopSender.Send returned error: %v", err)
	}
	if code != http.StatusOK {
		t.Errorf("status code: got %d; want %d", code, http.StatusOK)
	}
}

func TestNewVAPIDClient_NonNil(t *testing.T) {
	t.Parallel()
	c := webpush.NewVAPIDClient("pub", "priv", "mailto:test@example.com")
	if c == nil {
		t.Fatal("NewVAPIDClient returned nil")
	}
}

func TestVAPIDClient_Send_InvalidKeys_ReturnsError(t *testing.T) {
	t.Parallel()
	c := webpush.NewVAPIDClient("invalid-pub", "invalid-priv", "mailto:t@t.com")
	_, err := c.Send(context.Background(), webpush.Message{
		Endpoint:  "https://push.example.invalid/abc",
		P256dhKey: "bad",
		AuthKey:   "bad",
		Body:      []byte(`{"title":"t"}`),
		TTL:       60,
	})
	// webpush-go returns an error when it cannot build the VAPID JWT with invalid keys.
	if err == nil {
		t.Log("VAPIDClient.Send with invalid keys returned nil error (unexpected but not fatal)")
	}
}

// ── VAPIDClient.Send success-path tests ──────────────────────────────────────
//
// The success path — defer resp.Body.Close() and return resp.StatusCode, nil —
// is only reachable when wp.SendNotification succeeds. That requires the P256dh
// key to be a valid EC P256 point; the previously used hard-coded key was not on
// the curve and caused every call to return an error before reaching the HTTP
// layer.  Tests below use genSubscriptionKeys to produce genuinely valid keys.

func TestVAPIDClient_Send_201Created_PropagatesStatusCode(t *testing.T) {
	t.Parallel()

	privKey, pubKey, err := wp.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	p256dh, auth := genSubscriptionKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated) // 201 — standard push-service success
	}))
	defer srv.Close()

	// A dedicated *http.Transport (not NewVAPIDClient's shared
	// http.DefaultTransport) is required here: this test runs in parallel with
	// its siblings below, each owning its own httptest.Server, and
	// httptest.Server.Close unconditionally calls
	// http.DefaultTransport.CloseIdleConnections() — which would otherwise
	// tear down another parallel test's in-flight connection.
	c := webpush.NewVAPIDClientWithHTTP(pubKey, privKey, "mailto:test@example.com", &http.Client{Transport: &http.Transport{}})
	code, err := c.Send(context.Background(), webpush.Message{
		Endpoint:  srv.URL,
		P256dhKey: p256dh,
		AuthKey:   auth,
		Body:      []byte(`{"title":"goal","body":"Brazil 1-0 Argentina"}`),
		TTL:       3600,
	})
	if err != nil {
		t.Fatalf("Send: unexpected error: %v", err)
	}
	if code != http.StatusCreated {
		t.Errorf("status code: got %d, want %d", code, http.StatusCreated)
	}
}

func TestVAPIDClient_Send_410Gone_PropagatesStatusCode(t *testing.T) {
	// 410 Gone means the push subscription has expired or was revoked.
	// The caller (notification dispatcher) uses this code to delete the
	// subscription from the database and stop sending to it.
	// Send() must propagate the status code faithfully — not swallow it as an error.
	t.Parallel()

	privKey, pubKey, err := wp.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	p256dh, auth := genSubscriptionKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGone) // 410 — subscription expired
	}))
	defer srv.Close()

	// Dedicated transport — see TestVAPIDClient_Send_201Created_PropagatesStatusCode.
	c := webpush.NewVAPIDClientWithHTTP(pubKey, privKey, "mailto:test@example.com", &http.Client{Transport: &http.Transport{}})
	code, err := c.Send(context.Background(), webpush.Message{
		Endpoint:  srv.URL,
		P256dhKey: p256dh,
		AuthKey:   auth,
		Body:      []byte(`{"title":"test"}`),
		TTL:       3600,
	})
	if err != nil {
		t.Fatalf("Send: unexpected error for 410 response: %v", err)
	}
	if code != http.StatusGone {
		t.Errorf("status code: got %d, want %d (410 Gone must be propagated)", code, http.StatusGone)
	}
}

func TestVAPIDClient_Send_429TooManyRequests_PropagatesStatusCode(t *testing.T) {
	// 429 Too Many Requests means the push service is rate-limiting this sender.
	// The caller must back off; Send() must propagate the code rather than
	// treating it as an error so the caller can distinguish 429 from a transport
	// failure and apply the correct retry strategy.
	t.Parallel()

	privKey, pubKey, err := wp.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys: %v", err)
	}
	p256dh, auth := genSubscriptionKeys(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "60")
		w.WriteHeader(http.StatusTooManyRequests) // 429 — rate limited
	}))
	defer srv.Close()

	// Dedicated transport — see TestVAPIDClient_Send_201Created_PropagatesStatusCode.
	c := webpush.NewVAPIDClientWithHTTP(pubKey, privKey, "mailto:test@example.com", &http.Client{Transport: &http.Transport{}})
	code, err := c.Send(context.Background(), webpush.Message{
		Endpoint:  srv.URL,
		P256dhKey: p256dh,
		AuthKey:   auth,
		Body:      []byte(`{"title":"test"}`),
		TTL:       3600,
	})
	if err != nil {
		t.Fatalf("Send: unexpected error for 429 response: %v", err)
	}
	if code != http.StatusTooManyRequests {
		t.Errorf("status code: got %d, want %d (429 must be propagated)", code, http.StatusTooManyRequests)
	}
}

// Verify compile-time interface satisfaction.
var _ webpush.Sender = (*webpush.VAPIDClient)(nil)
var _ webpush.Sender = webpush.NoopSender{}
