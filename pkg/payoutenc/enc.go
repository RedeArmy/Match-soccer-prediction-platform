// Package payoutenc provides application-layer AES-256-GCM encryption for
// withdrawal payout_details stored in the database.
//
// # Threat model
//
// payout_details contains user-supplied routing information (bank account
// numbers, PayPal emails). Encrypting at the application layer ensures that a
// raw database dump, a rogue admin COPY, or a backup leak exposes only
// ciphertext — the plaintext is only accessible when the application key is
// present.
//
// This package intentionally does not use pgcrypto: the key never leaves the
// application process, which is a stronger isolation boundary than a
// database-managed function.
//
// # Storage format
//
// When encryption is enabled, the payout_details JSONB column stores:
//
//	{"_enc":"<base64url(nonce || ciphertext || GCM-tag)>"}
//
// The _enc key is the sentinel that distinguishes encrypted rows from legacy
// plaintext rows. Both formats are readable during the migration window:
// Unmarshal returns plaintext rows unchanged and decrypts _enc rows.
//
// # Key format
//
// The key is a 64-character lowercase hex string encoding 32 raw bytes
// (AES-256). Generate with: openssl rand -hex 32
// Set via WCQ_PAYMENT_PAYOUTENCRYPTIONKEY.
package payoutenc

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Encrypter encrypts and decrypts raw byte slices.
// Implementations must be safe for concurrent use.
type Encrypter interface {
	// Encrypt returns an opaque blob that Decrypt can recover.
	Encrypt(plaintext []byte) ([]byte, error)
	// Decrypt recovers the original plaintext from a blob produced by Encrypt.
	Decrypt(ciphertext []byte) ([]byte, error)
	// IsEnabled reports whether this instance applies real encryption.
	// A Noop encrypter returns false; an AES-GCM encrypter returns true.
	IsEnabled() bool
}

// Noop is an Encrypter that passes data through unchanged.
// Use in local development or tests when no encryption key is configured.
var Noop Encrypter = noopEncrypter{}

type noopEncrypter struct{}

func (noopEncrypter) Encrypt(p []byte) ([]byte, error) { return p, nil }
func (noopEncrypter) Decrypt(c []byte) ([]byte, error) { return c, nil }
func (noopEncrypter) IsEnabled() bool                  { return false }

// aesGCMEncrypter is the production AES-256-GCM implementation.
// The 12-byte nonce is prepended to the ciphertext so each call to Encrypt
// produces a distinct, self-contained blob.
type aesGCMEncrypter struct {
	aead cipher.AEAD
}

// NewAESGCM constructs an Encrypter from hexKey, a 64-character lowercase hex
// string that encodes a 32-byte AES-256 key.
//
// Generate a suitable key with:
//
//	openssl rand -hex 32
func NewAESGCM(hexKey string) (Encrypter, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: invalid key encoding: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("payoutenc: key must be 32 bytes (64 hex chars), got %d bytes", len(key))
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: create GCM: %w", err)
	}
	return &aesGCMEncrypter{aead: aead}, nil
}

// Encrypt returns nonce || ciphertext || GCM-tag using a freshly generated
// 12-byte random nonce.
func (e *aesGCMEncrypter) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, e.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("payoutenc: generate nonce: %w", err)
	}
	// Seal appends ciphertext+tag after nonce in the same slice.
	return e.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Decrypt expects the layout produced by Encrypt (nonce prepended).
func (e *aesGCMEncrypter) Decrypt(data []byte) ([]byte, error) {
	ns := e.aead.NonceSize()
	if len(data) < ns+e.aead.Overhead() {
		return nil, errors.New("payoutenc: ciphertext too short")
	}
	plaintext, err := e.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: decrypt: %w", err)
	}
	return plaintext, nil
}

func (e *aesGCMEncrypter) IsEnabled() bool { return true }

// encKey is the JSONB object key that signals an encrypted payload.
const encKey = "_enc"

// encEnvelope is used solely for JSON probing — not for general use.
type encEnvelope struct {
	Enc string `json:"_enc"`
}

// Marshal encodes details as JSON and, when enc is enabled, wraps the result
// in the encrypted envelope:
//
//	{"_enc":"<base64url(nonce||ciphertext||tag)>"}
//
// When enc is Noop the plain JSON is returned unchanged.
func Marshal(enc Encrypter, details map[string]string) ([]byte, error) {
	raw, err := json.Marshal(details)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: marshal: %w", err)
	}
	if !enc.IsEnabled() {
		return raw, nil
	}
	ct, err := enc.Encrypt(raw)
	if err != nil {
		return nil, err
	}
	env := encEnvelope{Enc: base64.URLEncoding.EncodeToString(ct)}
	out, err := json.Marshal(env)
	if err != nil {
		return nil, fmt.Errorf("payoutenc: marshal envelope: %w", err)
	}
	return out, nil
}

// Unmarshal decrypts (when necessary) and JSON-decodes a payout_details blob
// from the database.
//
// Decision table:
//   - enc enabled  + _enc key present  → decrypt, then unmarshal
//   - enc enabled  + no _enc key       → legacy plaintext row; unmarshal directly
//   - enc disabled + _enc key present  → error: key required to read encrypted row
//   - enc disabled + no _enc key       → unmarshal directly (dev / plaintext)
//
// The legacy-plaintext fallback exists solely to allow a rolling migration
// where old rows are re-encrypted in the background. Once all rows have been
// migrated the fallback can be removed.
func Unmarshal(enc Encrypter, data []byte) (map[string]string, error) {
	if len(data) == 0 {
		return nil, nil
	}

	hasEnvelope := bytes.Contains(data, []byte(`"`+encKey+`"`))

	if hasEnvelope {
		if !enc.IsEnabled() {
			return nil, errors.New("payoutenc: row contains encrypted data but no encryption key is configured (WCQ_PAYMENT_PAYOUTENCRYPTIONKEY)")
		}
		var env encEnvelope
		if err := json.Unmarshal(data, &env); err != nil {
			return nil, fmt.Errorf("payoutenc: unmarshal envelope: %w", err)
		}
		ct, err := base64.URLEncoding.DecodeString(env.Enc)
		if err != nil {
			return nil, fmt.Errorf("payoutenc: base64 decode: %w", err)
		}
		plain, err := enc.Decrypt(ct)
		if err != nil {
			return nil, err
		}
		var m map[string]string
		if err := json.Unmarshal(plain, &m); err != nil {
			return nil, fmt.Errorf("payoutenc: unmarshal plaintext: %w", err)
		}
		return m, nil
	}

	// Legacy plaintext row or dev mode.
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("payoutenc: unmarshal: %w", err)
	}
	return m, nil
}
