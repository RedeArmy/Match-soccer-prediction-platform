package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

// NewMetrics returns a chi-compatible middleware that records one
// http.server.request.duration observation per completed request.
//
// The histogram uses chi's route template (e.g. "/api/v1/matches/{id}") as the
// http.route attribute, preventing high-cardinality label explosion from raw
// URL paths. When no chi context is present the raw URL path is used instead.
//
// The meter is expected to be the application-wide OTel Meter (typically
// obtained from otel.GetMeterProvider().Meter("wcq")). Passing a noop Meter
// (the global default when metrics are disabled) makes the middleware a
// transparent passthrough: all calls compile and execute without side-effects.
func NewMetrics(meter metric.Meter) func(http.Handler) http.Handler {
	hist, _ := meter.Float64Histogram(
		"http.server.request.duration",
		metric.WithDescription("HTTP server request duration."),
		metric.WithUnit("s"),
		metric.WithExplicitBucketBoundaries(
			0.005, 0.010, 0.025, 0.050, 0.100, 0.250, 0.500, 1, 2.5, 5,
		),
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			mrw := &metricsResponseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(mrw, r)

			hist.Record(r.Context(), time.Since(start).Seconds(),
				metric.WithAttributes(
					attribute.String("http.request.method", r.Method),
					attribute.String("http.route", routePattern(r)),
					attribute.Int("http.response.status_code", mrw.status),
				),
			)
		})
	}
}

// routePattern returns chi's matched route template when available, falling back
// to the raw URL path. This keeps label cardinality bounded regardless of how
// many distinct IDs appear in URLs.
func routePattern(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return r.URL.Path
}

// metricsResponseWriter captures the HTTP status code written by the handler.
// It forwards Flush() calls to the underlying writer so SSE streams remain
// functional when this middleware is present in the chain.
type metricsResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *metricsResponseWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush implements http.Flusher. If the underlying ResponseWriter supports
// flushing (e.g. for Server-Sent Events), the call is forwarded; otherwise it
// is silently ignored so that handlers performing a safe type assertion do not
// receive a nil value.
func (w *metricsResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
