package middleware

import (
	"net/http"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

// responseWriter wraps http.ResponseWriter to capture the status code and
// response size written by downstream handlers.
//
// http.ResponseWriter does not expose the status code after it has been
// written, so wrapping is the only way to observe it in middleware that runs
// after the handler (post-handler logging). The default status is 200 because
// a handler that calls Write without calling WriteHeader first implicitly
// sends a 200 response.
type responseWriter struct {
	http.ResponseWriter
	status int
	size   int
}

func (rw *responseWriter) WriteHeader(status int) {
	rw.status = status
	rw.ResponseWriter.WriteHeader(status)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.size += n
	return n, err
}

// RequestLogger returns a middleware that logs one structured entry per
// completed HTTP request using the provided zap logger.
//
// The following fields are logged on every request:
//   - request_id  - correlates log entries with a specific request
//   - method      - HTTP verb
//   - path        - URL path (query string excluded to avoid logging PII)
//   - status      - HTTP status code written by the handler
//   - latency_ms  - wall-clock duration from first byte received to last byte sent
//   - remote_ip   - client IP address as set by chi's RealIP middleware
//
// The user_id field is appended only when the request context contains a
// Clerk user ID (i.e. the request passed through RequireAuth).
func RequestLogger(log *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			fields := []zap.Field{
				zap.String("request_id", GetRequestID(r.Context())),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", wrapped.status),
				zap.Int64("latency_ms", time.Since(start).Milliseconds()),
				zap.String("remote_ip", r.RemoteAddr),
			}

			if sc := trace.SpanFromContext(r.Context()).SpanContext(); sc.IsValid() {
				fields = append(fields, zap.String("trace_id", sc.TraceID().String()))
				fields = append(fields, zap.String("span_id", sc.SpanID().String()))
			}

			if userID, ok := UserIDFromContext(r.Context()); ok {
				fields = append(fields, zap.String("user_id", userID))
			}

			log.Info("request completed", fields...)
		})
	}
}
