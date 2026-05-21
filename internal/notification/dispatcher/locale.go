package dispatcher

// Locale identifies the language used for user-facing notification text.
type Locale string

// Supported locale values for user-facing notification content.
const (
	LocaleEN Locale = "en"
	LocaleES Locale = "es"
)

// localeStr returns es when locale == LocaleES, otherwise en.
// Both arguments are always evaluated by the caller; use only when the
// formatting cost of the unused string is negligible (notification path).
func localeStr(en, es string, locale Locale) string {
	if locale == LocaleES {
		return es
	}
	return en
}
