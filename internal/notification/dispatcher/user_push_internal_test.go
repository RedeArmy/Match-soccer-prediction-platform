package dispatcher

import "testing"

func TestTruncateRunes_MaxZeroOrNegative_ReturnsOriginal(t *testing.T) {
	t.Parallel()
	s := "hello"
	if got := truncateRunes(s, 0); got != s {
		t.Errorf("truncateRunes(%q, 0) = %q; want %q", s, got, s)
	}
	if got := truncateRunes(s, -5); got != s {
		t.Errorf("truncateRunes(%q, -5) = %q; want %q", s, got, s)
	}
}

func TestTruncateRunes_WithinLimit_ReturnsOriginal(t *testing.T) {
	t.Parallel()
	s := "hi"
	if got := truncateRunes(s, 10); got != s {
		t.Errorf("truncateRunes(%q, 10) = %q; want %q", s, got, s)
	}
}

func TestTruncateRunes_ExceedsLimit_TruncatesAtRuneBoundary(t *testing.T) {
	t.Parallel()
	// Use multibyte runes to verify rune-aware truncation.
	s := "héllo wörld"
	got := truncateRunes(s, 5)
	want := "héllo"
	if got != want {
		t.Errorf("truncateRunes(%q, 5) = %q; want %q", s, got, want)
	}
	if len([]rune(got)) != 5 {
		t.Errorf("truncated string has %d runes; want 5", len([]rune(got)))
	}
}
