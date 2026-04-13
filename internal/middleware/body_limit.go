package middleware

import "net/http"

// RequestBodyLimit wraps r.Body with http.MaxBytesReader so that any read
// beyond maxBytes returns a *http.MaxBytesError. The actual 413 response is
// produced by handler.decodeError when it receives that error from
// json.Decode — this middleware only installs the guard; it does not read
// the body itself.
//
// Apply this middleware at the subrouter level for endpoints that accept a
// JSON body. GET endpoints that never read r.Body are unaffected.
func RequestBodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			next.ServeHTTP(w, r)
		})
	}
}
