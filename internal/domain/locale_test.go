package domain_test

import (
	"testing"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

func TestParseLocale_KnownTags(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  domain.Locale
	}{
		{"en", domain.LocaleEN},
		{"es", domain.LocaleES},
		{"en-US", domain.LocaleEN},
		{"es-GT", domain.LocaleES},
		{"EN", domain.DefaultLocale}, // uppercase falls through to default
		{"fr", domain.DefaultLocale},
		{"", domain.DefaultLocale},
		{"de-DE", domain.DefaultLocale},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			if got := domain.ParseLocale(tc.input); got != tc.want {
				t.Errorf("ParseLocale(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestLocaleStr_SelectsByLocale(t *testing.T) {
	t.Parallel()
	cases := []struct {
		locale domain.Locale
		en     string
		es     string
		want   string
	}{
		{domain.LocaleEN, "Hello", "Hola", "Hello"},
		{domain.LocaleES, "Hello", "Hola", "Hola"},
		{domain.DefaultLocale, "a", "b", "b"}, // DefaultLocale is "es"
	}
	for _, tc := range cases {
		tc := tc
		t.Run(string(tc.locale), func(t *testing.T) {
			t.Parallel()
			if got := domain.LocaleStr(tc.en, tc.es, tc.locale); got != tc.want {
				t.Errorf("LocaleStr(%q,%q,%q) = %q; want %q", tc.en, tc.es, tc.locale, got, tc.want)
			}
		})
	}
}

func TestDefaultLocale_IsSpanish(t *testing.T) {
	if domain.DefaultLocale != domain.LocaleES {
		t.Errorf("DefaultLocale = %q; want %q (Guatemala-primary platform)", domain.DefaultLocale, domain.LocaleES)
	}
}
