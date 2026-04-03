// Package middleware provides HTTP middleware for the chi router.
//
// Each file in this package implements a single, self-contained middleware
// function. Middleware is applied in the Routes() method of internal/api/Server
// and must be stateless and safe for concurrent use by multiple goroutines.
//
// Custom middleware in this package supplements — rather than replaces — the
// middleware provided by go-chi/chi/v5/middleware. Use chi's built-in
// middleware (RequestID, RealIP, Recoverer) for generic HTTP concerns, and
// implement custom middleware here only for concerns specific to this
// application (JWT validation, structured request logging with zap, etc.).
package middleware

// TODO: implement RequireJWT middleware.
//
// RequireJWT validates the Bearer token in the Authorization header against
// the application's JWT secret. On success, it extracts the user ID and role
// from the token claims and stores them in the request context so that
// downstream handlers can access them without re-parsing the token.
// On failure, it writes a 401 response and does not call the next handler.
//
// The JWT secret must be injected via a closure or a struct method, not
// read from a global variable, to keep the middleware testable.
