package vapid_test

import (
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/vapid"
)

func TestGenerateKeys_ReturnsNonEmptyKeys(t *testing.T) {
	keys, err := vapid.GenerateKeys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if keys.PublicKey == "" {
		t.Error("PublicKey must not be empty")
	}
	if keys.PrivateKey == "" {
		t.Error("PrivateKey must not be empty")
	}
}

func TestGenerateKeys_UniqueOnEachCall(t *testing.T) {
	k1, err := vapid.GenerateKeys()
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	k2, err := vapid.GenerateKeys()
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	if k1.PublicKey == k2.PublicKey {
		t.Error("expected unique public keys across calls")
	}
	if k1.PrivateKey == k2.PrivateKey {
		t.Error("expected unique private keys across calls")
	}
}

func TestGenerateKeys_PublicKeyIsBase64URL(t *testing.T) {
	keys, err := vapid.GenerateKeys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Uncompressed P-256 point is 65 bytes → Base64URL ≈ 87 chars (no padding).
	if len(keys.PublicKey) < 80 || len(keys.PublicKey) > 90 {
		t.Errorf("public key length %d out of expected range [80,90]", len(keys.PublicKey))
	}
	if strings.ContainsAny(keys.PublicKey, "+/=") {
		t.Error("public key must be Base64URL-encoded (no '+', '/', or '=')")
	}
}

func TestGenerateKeys_PrivateKeyIsBase64URL(t *testing.T) {
	keys, err := vapid.GenerateKeys()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// P-256 private key is 32 bytes → Base64URL ≈ 43 chars (no padding).
	if len(keys.PrivateKey) < 40 || len(keys.PrivateKey) > 50 {
		t.Errorf("private key length %d out of expected range [40,50]", len(keys.PrivateKey))
	}
	if strings.ContainsAny(keys.PrivateKey, "+/=") {
		t.Error("private key must be Base64URL-encoded (no '+', '/', or '=')")
	}
}
