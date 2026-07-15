package middleware

import (
	"net/http"

	"github.com/rs/cors"
)

// CORS returns a middleware that enforces the Cross-Origin Resource Sharing
// policy for the API.
//
// allowedOrigins is the list of origins that may make cross-origin requests.
// It is populated from WCQ_CORS_ALLOWEDORIGINS at startup via pkg/config,
// which parses the comma-separated env var into a []string before the
// middleware is constructed. An empty slice blocks all cross-origin requests
// — see the AllowOriginFunc branch below for why this needs explicit handling
// rather than just passing the empty slice through to rs/cors.
//
// The rs/cors library is used rather than a hand-rolled implementation
// because the CORS specification has several non-obvious edge cases:
// preflight caching, the Vary header, credentialed requests, and wildcard
// interactions with credentials. Using a well-tested library eliminates
// the risk of subtle security regressions when the policy is updated.
//
// Allowed methods and headers are fixed to the set required by a standard
// JSON REST API. Expand them here if the API later requires additional
// HTTP methods or custom request headers.
func CORS(allowedOrigins []string) func(http.Handler) http.Handler {
	opts := cors.Options{
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           600, // 10 minutes preflight cache
	}

	if len(allowedOrigins) == 0 {
		// rs/cors treats a nil/empty AllowedOrigins as "allow every origin"
		// (see cors.go: `case len(options.AllowedOrigins) == 0: ... allowedOriginsAll
		// = true` when AllowOriginFunc is also unset) — the exact opposite of
		// what every call site of this function expects an empty list to mean.
		// AllowOriginFunc takes priority over that default, so an explicit
		// deny-all function is required to actually close CORS when no origin
		// has been configured, rather than silently defaulting to open.
		opts.AllowOriginFunc = func(string) bool { return false }
	} else {
		opts.AllowedOrigins = allowedOrigins
	}

	return cors.New(opts).Handler
}
