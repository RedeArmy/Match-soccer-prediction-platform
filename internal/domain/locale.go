package domain

// Locale is a BCP-47 language tag used for user-facing string selection.
// Keeping the type in the domain package makes it importable by every layer
// (service, repository, notification dispatcher) without introducing a new
// dependency or circular imports.
type Locale string

const (
	// LocaleEN selects English for user-facing strings.
	LocaleEN Locale = "en"

	// LocaleES selects Spanish for user-facing strings.
	// This is the platform default for the primary Guatemalan audience.
	LocaleES Locale = "es"

	// DefaultLocale is the locale applied when a user has not set a preference
	// and no Accept-Language negotiation produces a supported tag.
	DefaultLocale Locale = LocaleES
)

// ParseLocale converts a raw BCP-47 string into a supported Locale.
// Unknown or empty values fall back to DefaultLocale so that callers never
// hold an unsupported tag. Only the language subtag is matched (e.g. "es-GT"
// resolves to LocaleES).
func ParseLocale(raw string) Locale {
	switch {
	case len(raw) >= 2 && raw[:2] == "en":
		return LocaleEN
	case len(raw) >= 2 && raw[:2] == "es":
		return LocaleES
	default:
		return DefaultLocale
	}
}

// LocaleStr returns the es string when locale is LocaleES, and the en string
// in all other cases. Both arguments are evaluated by the caller; use only
// when the formatting cost of the unused string is negligible.
//
// This is the single source of truth for binary en/es selection. Callers in
// the notification dispatcher and service layer import this function instead
// of maintaining their own copy.
func LocaleStr(en, es string, locale Locale) string {
	if locale == LocaleES {
		return es
	}
	return en
}
