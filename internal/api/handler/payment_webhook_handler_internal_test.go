package handler

import "testing"

// ── maskIntentToken ──────────────────────────────────────────────────────────

func TestMaskIntentToken_FullLengthToken_TruncatesToPrefix(t *testing.T) {
	// generateIntentToken produces a 64-char hex string (256-bit token).
	token := "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	got := maskIntentToken(token)
	want := "a1b2c3d4…"
	if got != want {
		t.Errorf("maskIntentToken(%q) = %q, want %q", token, got, want)
	}
	if got == token {
		t.Error("maskIntentToken must not return the full token")
	}
}

func TestMaskIntentToken_ShortValue_ReturnedUnchanged(t *testing.T) {
	for _, v := range []string{"", "short", "12345678"} {
		if got := maskIntentToken(v); got != v {
			t.Errorf("maskIntentToken(%q) = %q, want unchanged", v, got)
		}
	}
}
