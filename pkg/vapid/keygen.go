// Package vapid provides VAPID (Voluntary Application Server Identification)
// key generation for Web Push (RFC 8292).
//
// Keys are ECDH P-256 pairs encoded as Base64URL without padding.  Generate
// once per deployment:
//
//	go run ./cmd/vapid-keygen
//
// Copy the output into your environment variables or into the system_params
// table (keys notify.web_push_vapid_public_key / notify.web_push_vapid_private_key).
// All existing browser subscriptions must be re-registered when keys are rotated.
package vapid

import (
	"fmt"

	wp "github.com/SherClockHolmes/webpush-go"
)

// Keys holds a Base64URL-encoded ECDH P-256 key pair for VAPID.
type Keys struct {
	// PublicKey is shared with browsers via the applicationServerKey
	// parameter of PushManager.subscribe().  Store in
	// WCQ_NOTIFY_VAPID_PUBLIC_KEY.
	PublicKey string
	// PrivateKey signs the VAPID JWT sent to push services.  Treat it as a
	// secret — store in WCQ_NOTIFY_VAPID_PRIVATE_KEY and never commit it.
	PrivateKey string
}

// GenerateKeys returns a fresh ECDH P-256 key pair in Base64URL encoding.
// Call once per deployment; the returned keys are safe to write directly into
// environment variables or system_params rows.
func GenerateKeys() (Keys, error) {
	// webpush-go returns (privateKey, publicKey, err) — note the order.
	priv, pub, err := wp.GenerateVAPIDKeys()
	if err != nil {
		return Keys{}, fmt.Errorf("vapid: generate keys: %w", err)
	}
	return Keys{PublicKey: pub, PrivateKey: priv}, nil
}
