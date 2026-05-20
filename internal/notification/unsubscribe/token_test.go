package unsubscribe_test

import (
	"strings"
	"testing"
	"time"

	"github.com/rede/world-cup-quiniela/internal/notification/unsubscribe"
)

const testSecret = "test-secret-key-32-bytes-longXXX"

func TestSignVerify_RoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now()
	tok := unsubscribe.SignToken(42, testSecret, now)
	if tok == "" {
		t.Fatal("SignToken returned empty string")
	}
	userID, err := unsubscribe.VerifyToken(tok, testSecret)
	if err != nil {
		t.Fatalf("VerifyToken: %v", err)
	}
	if userID != 42 {
		t.Errorf("user ID: got %d; want 42", userID)
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	t.Parallel()
	// Sign with a time far in the past so the token is already expired.
	past := time.Now().Add(-(unsubscribe.TokenTTL + time.Second))
	tok := unsubscribe.SignToken(7, testSecret, past)
	_, err := unsubscribe.VerifyToken(tok, testSecret)
	if err == nil {
		t.Fatal("expected error for expired token; got nil")
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	t.Parallel()
	tok := unsubscribe.SignToken(10, testSecret, time.Now())
	_, err := unsubscribe.VerifyToken(tok, "wrong-secret")
	if err == nil {
		t.Fatal("expected error for wrong secret; got nil")
	}
}

func TestVerify_TamperedUserID(t *testing.T) {
	t.Parallel()
	tok := unsubscribe.SignToken(5, testSecret, time.Now())
	// Replace the user ID part with a different value; signature must not match.
	parts := strings.SplitN(tok, ".", 3)
	tampered := "999." + parts[1] + "." + parts[2]
	_, err := unsubscribe.VerifyToken(tampered, testSecret)
	if err == nil {
		t.Fatal("expected error for tampered user ID; got nil")
	}
}

func TestVerify_MalformedToken(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"", "onlyone", "two.parts", "no.dot.hex.extra"} {
		if _, err := unsubscribe.VerifyToken(bad, testSecret); err == nil {
			t.Errorf("VerifyToken(%q) returned nil error; want error", bad)
		}
	}
}

func TestVerify_InvalidUserID(t *testing.T) {
	t.Parallel()
	tok := unsubscribe.SignToken(0, testSecret, time.Now())
	_, err := unsubscribe.VerifyToken(tok, testSecret)
	if err == nil {
		t.Fatal("expected error for user ID 0; got nil")
	}
}
