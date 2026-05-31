package payoutenc_test

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/rede/world-cup-quiniela/pkg/payoutenc"
)

// testKey is a fixed 32-byte key for deterministic tests.
// Never use this value in production.
var testKey = strings.Repeat("ab", 32) // 64 hex chars = 32 bytes

func newTestEncrypter(t *testing.T) payoutenc.Encrypter {
	t.Helper()
	enc, err := payoutenc.NewAESGCM(testKey)
	if err != nil {
		t.Fatalf("NewAESGCM: %v", err)
	}
	return enc
}

func TestNewAESGCM_InvalidKey(t *testing.T) {
	cases := []struct {
		name   string
		hexKey string
	}{
		{"not hex", "zzzz"},
		{"too short (30 bytes)", hex.EncodeToString(make([]byte, 30))},
		{"too long (33 bytes)", hex.EncodeToString(make([]byte, 33))},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := payoutenc.NewAESGCM(tc.hexKey); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestMarshalUnmarshal_RoundTrip(t *testing.T) {
	enc := newTestEncrypter(t)
	original := map[string]string{
		"account_number": "12345678901",
		"bank_name":      "Banco Industrial",
	}

	blob, err := payoutenc.Marshal(enc, original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	// Encrypted blob must not contain plaintext values.
	if strings.Contains(string(blob), "12345678901") {
		t.Error("ciphertext contains plaintext account_number")
	}
	if strings.Contains(string(blob), "Banco Industrial") {
		t.Error("ciphertext contains plaintext bank_name")
	}

	// Must use envelope format.
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(blob, &probe); err != nil {
		t.Fatalf("unmarshal probe: %v", err)
	}
	if _, ok := probe["_enc"]; !ok {
		t.Error("expected _enc key in encrypted output")
	}

	got, err := payoutenc.Unmarshal(enc, blob)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if len(got) != len(original) {
		t.Fatalf("length mismatch: got %d want %d", len(got), len(original))
	}
	for k, want := range original {
		if got[k] != want {
			t.Errorf("key %q: got %q want %q", k, got[k], want)
		}
	}
}

func TestMarshalUnmarshal_EachCallProducesDistinctCiphertext(t *testing.T) {
	enc := newTestEncrypter(t)
	details := map[string]string{"paypal_email": "test@example.com"}

	b1, _ := payoutenc.Marshal(enc, details)
	b2, _ := payoutenc.Marshal(enc, details)
	if string(b1) == string(b2) {
		t.Error("two encryptions of the same plaintext must differ (nonce must be random)")
	}
}

func TestUnmarshal_LegacyPlaintextRow(t *testing.T) {
	enc := newTestEncrypter(t)
	// Simulate a legacy row written before encryption was introduced.
	legacy, _ := json.Marshal(map[string]string{"paypal_email": "old@example.com"})

	got, err := payoutenc.Unmarshal(enc, legacy)
	if err != nil {
		t.Fatalf("legacy plaintext should be readable: %v", err)
	}
	if got["paypal_email"] != "old@example.com" {
		t.Errorf("got %q, want old@example.com", got["paypal_email"])
	}
}

func TestUnmarshal_NoopRoundTrip(t *testing.T) {
	// Noop marshal produces plain JSON; Noop unmarshal reads it back.
	details := map[string]string{"bank_name": "BAC"}
	blob, err := payoutenc.Marshal(payoutenc.Noop, details)
	if err != nil {
		t.Fatalf("Marshal(Noop): %v", err)
	}
	// No envelope.
	if strings.Contains(string(blob), "_enc") {
		t.Error("Noop must not produce envelope")
	}
	got, err := payoutenc.Unmarshal(payoutenc.Noop, blob)
	if err != nil {
		t.Fatalf("Unmarshal(Noop): %v", err)
	}
	if got["bank_name"] != "BAC" {
		t.Errorf("got %q, want BAC", got["bank_name"])
	}
}

func TestUnmarshal_EncryptedRowWithNoopEncrypter_ReturnsError(t *testing.T) {
	enc := newTestEncrypter(t)
	details := map[string]string{"paypal_email": "x@y.com"}
	blob, _ := payoutenc.Marshal(enc, details)

	// Attempt to read it with Noop (simulates missing key after deployment).
	if _, err := payoutenc.Unmarshal(payoutenc.Noop, blob); err == nil {
		t.Error("expected error when reading encrypted row without a key")
	}
}

func TestUnmarshal_TamperedCiphertext_ReturnsError(t *testing.T) {
	enc := newTestEncrypter(t)
	blob, _ := payoutenc.Marshal(enc, map[string]string{"k": "v"})

	// Flip a byte in the base64 value to corrupt the GCM tag.
	s := string(blob)
	idx := strings.Index(s, `"_enc":"`) + len(`"_enc":"`)
	if idx < len(`"_enc":"`) {
		t.Fatal("could not find _enc value")
	}
	corrupted := s[:idx] + "A" + s[idx+1:]

	if _, err := payoutenc.Unmarshal(enc, []byte(corrupted)); err == nil {
		t.Error("expected error for tampered ciphertext")
	}
}

func TestUnmarshal_EmptyData_ReturnsNil(t *testing.T) {
	got, err := payoutenc.Unmarshal(payoutenc.Noop, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestNoop_EncryptDecrypt_Passthrough(t *testing.T) {
	data := []byte(`{"k":"v"}`)

	enc, err := payoutenc.Noop.Encrypt(data)
	if err != nil {
		t.Fatalf("Noop.Encrypt: unexpected error: %v", err)
	}
	if string(enc) != string(data) {
		t.Errorf("Noop.Encrypt: got %q, want %q", enc, data)
	}

	dec, err := payoutenc.Noop.Decrypt(data)
	if err != nil {
		t.Fatalf("Noop.Decrypt: unexpected error: %v", err)
	}
	if string(dec) != string(data) {
		t.Errorf("Noop.Decrypt: got %q, want %q", dec, data)
	}
}

func TestDecrypt_CiphertextTooShort_ReturnsError(t *testing.T) {
	enc := newTestEncrypter(t)
	// A valid nonce is 12 bytes; GCM overhead is 16 bytes — anything shorter
	// than 28 bytes must be rejected by Decrypt.
	if _, err := enc.Decrypt([]byte("short")); err == nil {
		t.Error("expected error for ciphertext shorter than nonce+overhead")
	}
}

func TestMarshal_EncryptError_Propagates(t *testing.T) {
	// errEncrypter is an in-line Encrypter that always fails.
	enc := &errEncrypter{}
	if _, err := payoutenc.Marshal(enc, map[string]string{"k": "v"}); err == nil {
		t.Error("expected error when Encrypt fails")
	}
}

func TestUnmarshal_InvalidBase64InEnvelope_ReturnsError(t *testing.T) {
	enc := newTestEncrypter(t)
	// Build an envelope with garbage base64 content.
	bad := []byte(`{"_enc":"!!!not-valid-base64!!!"}`)
	if _, err := payoutenc.Unmarshal(enc, bad); err == nil {
		t.Error("expected error for invalid base64 in _enc field")
	}
}

// errEncrypter implements Encrypter and always returns an error from Encrypt.
type errEncrypter struct{}

func (errEncrypter) Encrypt(_ []byte) ([]byte, error) {
	return nil, errors.New("synthetic encrypt failure")
}
func (errEncrypter) Decrypt(_ []byte) ([]byte, error) { return nil, nil }
func (errEncrypter) IsEnabled() bool                  { return true }
