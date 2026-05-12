package domain

import "time"

// DefaultPaymentIntentTTL is how long a pending payment intent remains valid,
// expressed as a time.Duration. Derived from DefaultPaymentIntentTTLMinutes so
// the two constants always agree. Intents that expire before the webhook arrives
// are rejected, preventing stale captures from crediting an outdated balance.
const DefaultPaymentIntentTTL = time.Duration(DefaultPaymentIntentTTLMinutes) * time.Minute

// PaymentIntentStatus represents the lifecycle state of a payment intent.
type PaymentIntentStatus string

// Payment intent lifecycle states stored in the status column.
const (
	PaymentIntentPending  PaymentIntentStatus = "pending"  // awaiting PayPal capture
	PaymentIntentCaptured PaymentIntentStatus = "captured" // webhook confirmed payment
	PaymentIntentExpired  PaymentIntentStatus = "expired"  // TTL elapsed before capture
)

// PaymentIntent is a server-generated, single-use record that the frontend
// embeds as custom_id when creating a PayPal order. The opaque token prevents
// a user from substituting another user's ID in the PayPal order metadata.
//
// Lifecycle:
//
//	pending  →  captured  (webhook confirms payment)
//	pending  →  expired   (TTL elapsed before capture)
//
// CaptureID is the PayPal capture transaction ID and serves as the idempotency
// key for duplicate webhook deliveries.
type PaymentIntent struct {
	ID          int64
	Token       string
	UserID      int
	AmountCents int
	Currency    string
	Status      PaymentIntentStatus
	CaptureID   *string
	ExpiresAt   time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
