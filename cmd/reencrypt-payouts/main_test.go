package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
)

// testKeyHex is a 64-character hex string (32 raw bytes) — the exact format
// payoutenc.NewAESGCM requires. Built with strings.Repeat to avoid manual
// hex-digit counting errors.
var testKeyHex = strings.Repeat("ab", 32)

func testEncrypter(t *testing.T) payoutenc.Encrypter {
	t.Helper()
	enc, err := payoutenc.NewAESGCM(testKeyHex)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	return enc
}

func TestReencryptDetails_LegacyPlaintext_ProducesEncEnvelope(t *testing.T) {
	enc := testEncrypter(t)
	legacy := `{"account_number":"1234567890","bank_name":"Banrural"}`

	out, err := reencryptDetails(enc, legacy)
	if err != nil {
		t.Fatalf("reencryptDetails: %v", err)
	}
	if !strings.Contains(out, `"_enc"`) {
		t.Fatalf("expected encrypted envelope in output, got %q", out)
	}
	if strings.Contains(out, "1234567890") {
		t.Error("re-encrypted output must not contain the plaintext account number")
	}
}

func TestReencryptDetails_RoundTripsToOriginalValues(t *testing.T) {
	enc := testEncrypter(t)
	legacy := `{"account_number":"1234567890","bank_name":"Banrural"}`

	out, err := reencryptDetails(enc, legacy)
	if err != nil {
		t.Fatalf("reencryptDetails: %v", err)
	}

	decoded, err := payoutenc.Unmarshal(enc, []byte(out))
	if err != nil {
		t.Fatalf("Unmarshal re-encrypted output: %v", err)
	}
	var want map[string]string
	if err := json.Unmarshal([]byte(legacy), &want); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if len(decoded) != len(want) {
		t.Fatalf("field count: got %d, want %d", len(decoded), len(want))
	}
	for k, v := range want {
		if decoded[k] != v {
			t.Errorf("field %q: got %q, want %q", k, decoded[k], v)
		}
	}
}

func TestReencryptDetails_EmptyObject_RoundTrips(t *testing.T) {
	enc := testEncrypter(t)
	out, err := reencryptDetails(enc, `{}`)
	if err != nil {
		t.Fatalf("reencryptDetails: %v", err)
	}
	decoded, err := payoutenc.Unmarshal(enc, []byte(out))
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(decoded) != 0 {
		t.Errorf("expected empty map, got %v", decoded)
	}
}

func TestReencryptDetails_InvalidJSON_ReturnsError(t *testing.T) {
	enc := testEncrypter(t)
	if _, err := reencryptDetails(enc, `not-json`); err == nil {
		t.Fatal("expected error for invalid JSON input, got nil")
	}
}
