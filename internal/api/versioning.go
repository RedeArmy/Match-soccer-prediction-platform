// API versioning contract
//
// All business endpoints are served under a major-version prefix (/api/v1,
// /api/v2, …). The prefix is incremented only when a breaking change is
// introduced. Additive changes — new endpoints, new optional response fields,
// relaxed validation — are deployed under the existing version without a bump.
//
// # What constitutes a breaking change
//
//   - Removing an endpoint or changing its HTTP method
//   - Renaming, removing, or retyping a field in a response body
//   - Adding a required field to a request body
//   - Changing an HTTP status code in a way that alters client error handling
//   - Altering authentication requirements (scope, header name, token format)
//   - Changing pagination mechanics (cursor format, sort order, page-size defaults)
//   - Changing the semantics of an existing field (e.g. from inclusive to exclusive)
//
// # What is NOT a breaking change (safe to ship without a version bump)
//
//   - Adding new optional fields to a response body
//   - Adding new optional request fields with sane defaults
//   - Adding new endpoints or new HTTP methods on existing paths
//   - Relaxing validation (accepting a wider input set)
//   - Performance improvements with no observable contract change
//
// # Deprecation process
//
//  1. Apply Deprecated(sunsetDate) middleware to every affected route.
//     Clients receive Deprecation and Sunset headers on every response.
//  2. Publish a minimum 90-day advance notice in changelog / release notes.
//  3. Implement the replacement at the new version prefix.
//  4. Remove the deprecated route on or after the sunset date.
//
// # Adding a new API version
//
// Mount a second subrouter alongside v1 in server.go:
//
//	r.Route("/api/v2", func(r chi.Router) {
//	    r.Use(authMiddleware)
//	    mountV2Routes(r, handlers)
//	})
//
// Version subrouters are fully independent. v2 may delegate to v1 handlers for
// unchanged resources and override only the routes whose contract changed.

package api

import (
	"net/http"
	"time"
)

// VersionHeader returns middleware that stamps every response with an
// API-Version header. Clients that inspect this header can detect the active
// version without parsing the URL path, which is useful for logging,
// contract negotiation, and automated compatibility checks.
//
// Mount this middleware on each versioned subrouter so that every business
// endpoint carries the header automatically:
//
//	r.Route("/api/v1", func(r chi.Router) {
//	    r.Use(api.VersionHeader("v1"))
//	    ...
//	})
func VersionHeader(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

// Deprecated returns middleware that marks an endpoint as deprecated per
// RFC 8594. Every response from the wrapped handler carries:
//
//   - Deprecation: true — signals that the endpoint is officially deprecated
//   - Sunset: <HTTP-date> — the date/time after which the endpoint is removed
//   - Link: <successorURL>; rel="successor-version" — when successorURL is
//     non-empty, points clients to the replacement endpoint or version.
//
// The middleware is advisory: it does not reject requests. Clients, API
// gateways, and monitoring tools that understand these headers surface warnings
// before the endpoint disappears.
//
// Usage — mark a route deprecated with a 90-day sunset and a successor link:
//
//	r.With(api.Deprecated(
//	    time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
//	    "/api/v2/resource",
//	)).Get("/api/v1/resource", oldHandler)
//
// Pass an empty string for successorURL when no replacement exists yet.
func Deprecated(sunset time.Time, successorURL string) func(http.Handler) http.Handler {
	// Format once at construction time to avoid repeated allocation per request.
	sunsetVal := sunset.UTC().Format(http.TimeFormat)
	linkVal := ""
	if successorURL != "" {
		linkVal = `<` + successorURL + `>; rel="successor-version"`
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Deprecation", "true")
			w.Header().Set("Sunset", sunsetVal)
			if linkVal != "" {
				w.Header().Add("Link", linkVal)
			}
			next.ServeHTTP(w, r)
		})
	}
}
