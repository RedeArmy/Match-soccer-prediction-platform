package logger

// TODO: implement zap hooks for cross-cutting log enrichment.
//
// A zap hook is a function called on every log entry, useful for:
//   - Forwarding error-level log entries to an error-tracking service (Sentry).
//   - Incrementing a Prometheus counter per log level for alerting on
//     unexpected spikes in error or warning rates.
//
// Hooks are registered at logger construction time in New(), keeping the
// enrichment logic invisible to call sites.
