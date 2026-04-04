package middleware

import (
	"net/http"
	"runtime/debug"

	"go.uber.org/zap"
)

// Recover returns a middleware that catches any panic that occurs in a
// downstream handler, logs it via zap with the request ID for correlation,
// and returns a 500 response to the client.
//
// This replaces chi's built-in Recoverer middleware. The chi implementation
// writes the stack trace to stderr, which loses the structured request context
// (request ID, path, method) that makes the panic correlatable with the
// request that triggered it in a log aggregation system.
//
// The middleware always calls next.ServeHTTP in a deferred recover block.
// If no panic occurs the performance overhead is a single deferred function
// call per request, which is negligible compared to network I/O.
func Recover(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						zap.String("request_id", GetRequestID(r.Context())),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
						zap.Any("panic", rec),
						zap.ByteString("stack", debug.Stack()),
					)
					http.Error(w, "internal server error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
