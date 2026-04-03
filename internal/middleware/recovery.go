package middleware

// TODO: implement a custom panic recovery middleware if chi's built-in
// Recoverer does not meet the application's requirements.
//
// A custom implementation allows panic details to be logged via zap
// (preserving the request_id field for correlation) and reported to an
// error-tracking service (e.g. Sentry) before returning a 500 response.
// chi's Recoverer writes to stderr, which is sufficient early on but loses
// the structured context available in the request.
