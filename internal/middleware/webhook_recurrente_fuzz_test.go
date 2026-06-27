package middleware

import (
	"testing"
)

// FuzzParseRecurrenteSigningSecret ensures parseRecurrenteSigningSecret never
// panics on arbitrary input, including empty strings, whsec_ prefixes with
// malformed base64, and raw byte sequences.
func FuzzParseRecurrenteSigningSecret(f *testing.F) {
	f.Add("")
	f.Add("whsec_")
	f.Add("whsec_dGVzdA==") // valid base64
	f.Add("whsec_!!!notbase64!!!")
	f.Add("rawsecret")
	f.Add("whsec_" + string(make([]byte, 512))) // very long

	f.Fuzz(func(t *testing.T, secret string) {
		// Must never panic regardless of input.
		key, err := parseRecurrenteSigningSecret(secret)
		if err == nil && secret != "" && key == nil {
			t.Error("non-empty secret with no error must return a non-nil key")
		}
	})
}

// FuzzSvixSignatureValid ensures svixSignatureValid never panics on arbitrary
// header and body inputs. The security property (returning false for invalid
// sigs) is only checked for well-structured but wrong signatures.
func FuzzSvixSignatureValid(f *testing.F) {
	f.Add([]byte("key"), "msg-id-1", "1700000000", []byte(`{"event":"test"}`), "v1,abc123")
	f.Add([]byte{}, "id", "ts", []byte{}, "")
	f.Add([]byte("k"), "", "", []byte{}, "v1,dGVzdA== v1,other")
	f.Add([]byte("secret"), "id", "0", []byte("body"), "notprefixed garbage v1,")

	f.Fuzz(func(t *testing.T, key []byte, msgID, msgTimestamp string, body []byte, sigHeader string) {
		// Must never panic regardless of input.
		_ = svixSignatureValid(key, msgID, msgTimestamp, body, sigHeader)
	})
}
