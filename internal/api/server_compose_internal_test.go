// Package api provides internal white-box tests for unexported helpers in the
// composition root.  These tests must live in package api (not api_test) so
// they can reach private functions that are intentionally not part of the
// public API surface.
package api

import (
	"strings"
	"testing"

	"go.uber.org/zap/zaptest"
)

func TestBuildPayoutEncrypter_EmptyKey_ReturnsNoop(t *testing.T) {
	enc := buildPayoutEncrypter("", zaptest.NewLogger(t))
	if enc.IsEnabled() {
		t.Error("expected Noop encrypter for empty key: IsEnabled must be false")
	}
}

func TestBuildPayoutEncrypter_ValidKey_ReturnsEnabledEncrypter(t *testing.T) {
	// 64 hex chars = 32 bytes = valid AES-256 key.
	validKey := strings.Repeat("ab", 32)
	enc := buildPayoutEncrypter(validKey, zaptest.NewLogger(t))
	if !enc.IsEnabled() {
		t.Error("expected AES-GCM encrypter for valid 32-byte key: IsEnabled must be true")
	}
}
