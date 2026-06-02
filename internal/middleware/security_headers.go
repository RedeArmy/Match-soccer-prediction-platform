package middleware

import "net/http"

// SecurityHeaders sets defensive HTTP response headers on every response.
// Apply this as the outermost middleware so headers are present even on
// early-exit responses (401, 404, 429).
//
// Headers set:
//   - Strict-Transport-Security: max-age=31536000; includeSubDomains —
//     instructs browsers to use HTTPS exclusively for one year. Set
//     unconditionally: TLS may be terminated at a reverse proxy (Fly.io,
//     nginx, ALB), but the header is forwarded to browsers in all cases.
//     Setting it here ensures protection is active regardless of the
//     deployment topology (Fly.io, raw EC2, future migrations).
//   - X-Content-Type-Options: nosniff — prevents MIME-type sniffing in browsers.
//   - X-Frame-Options: DENY — prevents the API being embedded in an iframe.
//   - Referrer-Policy: strict-origin-when-cross-origin — limits referrer leakage.
//   - Content-Security-Policy: default-src 'none' — the API returns JSON only;
//     no scripts, images, or stylesheets are intentionally served from this origin.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		next.ServeHTTP(w, r)
	})
}
