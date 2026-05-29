package dispatcher

import "github.com/rede/world-cup-quiniela/internal/domain"

// Locale is a type alias for domain.Locale so that content builder functions
// use the canonical type while keeping their signatures unchanged.
type Locale = domain.Locale

// Re-export the canonical locale constants so existing content builders
// continue to compile without updating their import list.
const (
	LocaleEN = domain.LocaleEN
	LocaleES = domain.LocaleES
)

// localeStr is a package-local alias for domain.LocaleStr. Both arguments are
// always evaluated by the caller; use only when the formatting cost of the
// unused string is negligible (notification content builder path).
func localeStr(en, es string, locale Locale) string {
	return domain.LocaleStr(en, es, locale)
}
