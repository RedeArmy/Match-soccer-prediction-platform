package handler

import (
	"testing"
)

func TestExtensionForContentType(t *testing.T) {
	t.Parallel()

	cases := []struct {
		ct   string
		want string
	}{
		{"image/jpeg", ".jpg"},
		{"image/png", ".png"},
		{"image/webp", ".webp"},
		{"application/pdf", ".pdf"},
		{"application/octet-stream", ""},
		{"", ""},
		{"text/plain", ""},
	}

	for _, tc := range cases {
		got := extensionForContentType(tc.ct)
		if got != tc.want {
			t.Errorf("extensionForContentType(%q) = %q; want %q", tc.ct, got, tc.want)
		}
	}
}
