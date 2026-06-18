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
	PaymentIntentPending     PaymentIntentStatus = "pending"      // awaiting webhook capture
	PaymentIntentCaptured    PaymentIntentStatus = "captured"     // webhook confirmed payment
	PaymentIntentExpired     PaymentIntentStatus = "expired"      // TTL elapsed before capture
	PaymentIntentRejected    PaymentIntentStatus = "rejected"     // admin rejected comprobante
	PaymentIntentUnderReview PaymentIntentStatus = "under_review" // user disputed rejection with evidence
)

// PaymentIntent is a server-generated, single-use record that the frontend
// embeds as custom_id when creating a PayPal order or as wcq_reference when
// creating a Recurrente checkout. The opaque token prevents a user from
// substituting another user's ID in the payment provider's metadata.
//
// Lifecycle:
//
//	pending      →  captured     (webhook confirms payment)
//	pending      →  expired      (TTL elapsed before capture)
//	pending      →  rejected     (admin rejects the comprobante)
//	rejected     →  under_review (user disputes with evidence; admin re-evaluates)
//	under_review →  captured     (admin credits after reviewing dispute)
//	under_review →  rejected     (admin rejects dispute)
//
// Provider identifies the payment processor ("paypal" | "recurrente").
// CaptureID is the provider capture/transaction ID for idempotency.
// Comprobante* fields hold the user-uploaded payment proof when the webhook
// did not arrive automatically.
// ComprobanteRequired is set by an admin to request the user upload a receipt
// for a pending intent that arrived without automatic webhook confirmation.
// UserNotes contains the user's description when submitting a dispute.
type PaymentIntent struct {
	ID                     int64
	Token                  string
	UserID                 int
	AmountCents            int
	Currency               string
	Provider               string // "paypal" | "recurrente"
	Status                 PaymentIntentStatus
	CaptureID              *string
	ExpiresAt              time.Time
	ComprobanteKey         *string
	ComprobanteContentType *string
	ComprobanteFileSize    *int
	ComprobanteRequired    bool
	UserNotes              string
	ReviewedBy             *int
	ReviewNotes            string
	RejectedAt             *time.Time
	CreatedAt              time.Time
	UpdatedAt              time.Time
}
