package middleware

// TODO: implement structured HTTP request logging middleware using zap.
//
// Log the following fields on every completed request:
//   - request_id  (from chi's RequestID middleware, already in context)
//   - method, path, status, latency_ms
//   - user_id     (from JWT claims, if authenticated)
//   - remote_ip   (from RealIP middleware)
//
// Use zap.Logger rather than chi's built-in Logger middleware, which writes
// to a plain io.Writer. Structured fields allow log aggregation tools to
// filter and alert on specific request paths or status codes without regex.
