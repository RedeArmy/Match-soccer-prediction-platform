package randcode

import (
	"crypto/rand"
	"fmt"
	"io"
)

const defaultAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// Generator produces fixed-length random strings drawn from an alphabet.
// Inject via constructor so tests can substitute a deterministic implementation.
type Generator interface {
	Generate(length int) (string, error)
}

// Crypto uses crypto/rand. Alphabet defaults to an unambiguous character set
// (uppercase letters and digits, visually similar pairs removed).
// Rand overrides the entropy source; nil uses crypto/rand.Reader.
type Crypto struct {
	Alphabet string
	Rand     io.Reader
}

// Generate returns a random string of the given length drawn from Alphabet.
func (c Crypto) Generate(length int) (string, error) {
	alphabet := c.Alphabet
	if alphabet == "" {
		alphabet = defaultAlphabet
	}
	r := c.Rand
	if r == nil {
		r = rand.Reader
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", fmt.Errorf("randcode: %w", err)
	}
	result := make([]byte, length)
	for i, v := range b {
		result[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(result), nil
}

// Fixed always returns Code unchanged. Use in tests to pin generated values.
type Fixed struct{ Code string }

// Generate returns Code unchanged, ignoring length.
func (f Fixed) Generate(_ int) (string, error) { return f.Code, nil }
