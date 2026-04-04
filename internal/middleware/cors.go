package middleware

import (
	"net/http"
	"strings"

	"github.com/rs/cors"
)

// CORS returns a middleware that enforces the Cross-Origin Resource Sharing
// policy for the API.
//
// allowedOrigins is a comma-separated list of origins that may make
// cross-origin requests (e.g. "http://localhost:3000,https://myapp.com").
// It is read from WCQ_CORS_ALLOWEDORIGINS at startup via pkg/config.
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
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	origins := parseOrigins(allowedOrigins)

	c := cors.New(cors.Options{
		AllowedOrigins:   origins,
		AllowedMethods:   []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowedHeaders:   []string{"Authorization", "Content-Type", "X-Request-Id"},
		ExposedHeaders:   []string{"X-Request-Id"},
		AllowCredentials: true,
		MaxAge:           600, // 10 minutes preflight cache
	})

	return c.Handler
}

// parseOrigins splits a comma-separated origins string and trims whitespace
// from each entry. An empty string returns a slice containing only
// "http://localhost:3000" so that local development works out of the box
// without requiring the env var to be set.
func parseOrigins(raw string) []string {
	if raw == "" {
		return []string{"http://localhost:3000"}
	}

	parts := strings.Split(raw, ",")
	origins := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}
