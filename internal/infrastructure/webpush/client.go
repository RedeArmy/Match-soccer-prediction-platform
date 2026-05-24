// Package webpush wraps github.com/SherClockHolmes/webpush-go to send
// Web Push (VAPID / RFC 8292) notifications.
//
// The Sender interface is the only surface exposed to the rest of the codebase;
// the concrete implementation is swapped for a NoopSender in tests.
package webpush

import (
	"context"
	"fmt"
	"net/http"

	wp "github.com/SherClockHolmes/webpush-go"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Message is the payload delivered to a Web Push endpoint.
type Message struct {
	// Endpoint is the subscription endpoint URL (browser-generated).
	Endpoint string
	// P256dhKey is the client's Diffie-Hellman public key (base64url).
	P256dhKey string
	// AuthKey is the client's auth secret (base64url).
	AuthKey string
	// Body is the encrypted JSON payload.
	Body []byte
	// TTL is the expiry time in seconds (0 = provider default).
	TTL int
}

// Sender delivers a single Web Push message to one endpoint.
type Sender interface {
	Send(ctx context.Context, msg Message) (statusCode int, err error)
}

// VAPIDClient sends Web Push notifications using VAPID authentication.
type VAPIDClient struct {
	vapidPublicKey  string
	vapidPrivateKey string
	vapidSubject    string // mailto: or https: URI identifying the sender
	httpClient      *http.Client
}

// NewVAPIDClient constructs a VAPIDClient.
func NewVAPIDClient(publicKey, privateKey, subject string) *VAPIDClient {
	return newVAPIDClientWithHTTP(publicKey, privateKey, subject, &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	})
}

// newVAPIDClientWithHTTP constructs a VAPIDClient using the provided HTTP client.
// Used in tests to inject a custom transport without making real network calls.
func newVAPIDClientWithHTTP(publicKey, privateKey, subject string, hc *http.Client) *VAPIDClient {
	return &VAPIDClient{
		vapidPublicKey:  publicKey,
		vapidPrivateKey: privateKey,
		vapidSubject:    subject,
		httpClient:      hc,
	}
}

// Send delivers msg to its endpoint.  Returns the HTTP status code from the
// push service and any transport-level error.
func (c *VAPIDClient) Send(_ context.Context, msg Message) (int, error) {
	sub := &wp.Subscription{
		Endpoint: msg.Endpoint,
		Keys: wp.Keys{
			P256dh: msg.P256dhKey,
			Auth:   msg.AuthKey,
		},
	}

	resp, err := wp.SendNotification(msg.Body, sub, &wp.Options{
		HTTPClient:      c.httpClient,
		TTL:             msg.TTL,
		VAPIDPublicKey:  c.vapidPublicKey,
		VAPIDPrivateKey: c.vapidPrivateKey,
		Subscriber:      c.vapidSubject,
		Urgency:         wp.UrgencyNormal,
	})
	if err != nil {
		return 0, fmt.Errorf("webpush: send: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode, nil
}

// NoopSender satisfies Sender without making any network calls.  Use it in
// test environments where VAPID keys are not configured.
type NoopSender struct{}

// Send is a no-op that always returns 200.
func (NoopSender) Send(_ context.Context, _ Message) (int, error) { return http.StatusOK, nil }

var _ Sender = (*VAPIDClient)(nil)
var _ Sender = NoopSender{}
