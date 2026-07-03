package repository

import "testing"

// ── escapeLikePattern ────────────────────────────────────────────────────────

func TestEscapeLikePattern_NoMetacharacters_ReturnsUnchanged(t *testing.T) {
	got := escapeLikePattern("alice")
	if got != "alice" {
		t.Errorf("got %q, want %q", got, "alice")
	}
}

func TestEscapeLikePattern_PercentSign_Escaped(t *testing.T) {
	got := escapeLikePattern("50%_off")
	want := `50\%\_off`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeLikePattern_BareWildcard_EscapedNotLeftAsWildcard(t *testing.T) {
	got := escapeLikePattern("%")
	if got != `\%` {
		t.Errorf("got %q, want %q", got, `\%`)
	}
}

func TestEscapeLikePattern_Backslash_EscapedBeforeWildcards(t *testing.T) {
	// A literal backslash must become \\ without also double-escaping the
	// backslashes inserted for the adjacent '%'.
	got := escapeLikePattern(`a\%b`)
	want := `a\\\%b`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEscapeLikePattern_EmptyString_ReturnsEmpty(t *testing.T) {
	if got := escapeLikePattern(""); got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}
