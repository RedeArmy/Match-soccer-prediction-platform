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
// middleware is constructed. An empty slice blocks all cross-origin requests.
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
	c := cors.New(cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           600, // 10 minutes preflight cache
	})

	return c.Handler
}
