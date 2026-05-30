// Package middleware provides HTTP middleware for the chi router.
//
// Each file in this package implements a single, self-contained concern.
// Middleware is applied in the Routes() method of internal/api/Server and
// must be stateless and safe for concurrent use by multiple goroutines.
//
// Custom middleware in this package supplements - rather than replaces - the
// middleware provided by go-chi/chi/v5/middleware. The RequestID concern is
// delegated to chi; application-specific concerns (JWT validation, structured
// zap logging, trusted IP extraction, Clerk authentication) are implemented
// here to keep business context out of the chi package.
package middleware

import (
	"context"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// GetRequestID extracts the request ID injected by chi's RequestID middleware
// from the given context.
//
// Returning the ID through this helper rather than importing chimiddleware
// directly at every call site decouples the rest of the codebase from the
// chi package. If the source of the request ID ever changes (e.g. honouring
// an upstream X-Trace-Id header instead of generating a UUID), only this
// function needs to change.
func GetRequestID(ctx context.Context) string {
	return chimiddleware.GetReqID(ctx)
}
