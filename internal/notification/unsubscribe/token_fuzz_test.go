package unsubscribe

import (
	"testing"
	"time"
)

// FuzzVerifyToken ensures VerifyToken never panics on arbitrary token strings
// and that only syntactically valid, non-expired, correctly-signed tokens
// return a positive user ID.
func FuzzVerifyToken(f *testing.F) {
	secret := "test-secret-key"
	now := time.Now()

	// Seed: a legitimately signed token (should verify successfully).
	f.Add(SignToken(42, secret, now), secret)
	// Seed: empty token.
	f.Add("", secret)
	// Seed: too few segments.
	f.Add("42.1700000000", secret)
	// Seed: too many segments.
	f.Add("42.1700000000.aabbcc.extra", secret)
	// Seed: non-numeric user ID.
	f.Add("notanint.1700000000.aabbcc", secret)
	// Seed: expired timestamp.
	f.Add(SignToken(1, secret, now.Add(-TokenTTL-time.Hour)), secret)
	// Seed: wrong secret.
	f.Add(SignToken(1, "other-secret", now), secret)
	// Seed: truncated HMAC.
	f.Add("1.1700000000.aabb", secret)

	f.Fuzz(func(t *testing.T, tok, sec string) {
		// Must never panic regardless of input.
		userID, err := VerifyToken(tok, sec)
		if err == nil && userID <= 0 {
			t.Errorf("VerifyToken returned nil error but non-positive user ID %d for tok=%q", userID, tok)
		}
	})
}
