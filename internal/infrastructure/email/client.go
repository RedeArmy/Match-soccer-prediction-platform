// Package email defines the transactional email client contract and provides
// the Resend implementation used in production.
//
// The Client interface is the only symbol that application code outside this
// package should reference, making it straightforward to swap providers or
// inject a stub in tests.
package email

import "context"

// Message carries everything needed to send a single transactional email.
type Message struct {
	From    string   // e.g. "Quiniela <noreply@example.com>"
	To      []string // one or more recipient addresses
	Subject string
	HTML    string
}

// Client is the minimal contract for transactional email delivery.
// Implementations must be safe for concurrent use.
type Client interface {
	// Send delivers msg and returns the provider's message ID on success.
	// Callers should treat a non-empty msgID as confirmation that the provider
	// accepted the message; actual delivery is best-effort on the provider side.
	Send(ctx context.Context, msg Message) (msgID string, err error)
}
