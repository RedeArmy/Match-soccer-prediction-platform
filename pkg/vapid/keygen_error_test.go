package vapid

import (
	"errors"
	"testing"
)

func TestGenerateKeys_PropagatesUnderlyingError(t *testing.T) {
	orig := generateVAPIDKeys
	generateVAPIDKeys = func() (string, string, error) {
		return "", "", errors.New("crypto RNG unavailable")
	}
	defer func() { generateVAPIDKeys = orig }()

	_, err := GenerateKeys()
	if err == nil {
		t.Fatal("expected error from GenerateKeys, got nil")
	}
}
