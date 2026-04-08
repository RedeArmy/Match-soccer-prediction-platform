// Package health defines the Checker interface and the response types used
// by the /health/ready readiness endpoint.
//
// Each infrastructure component (database, Redis, …) implements Checker and
// is registered with the Server at startup. The readiness handler runs all
// registered checkers concurrently, aggregates their results, and returns
// HTTP 200 when every check passes or HTTP 503 when at least one fails.
//
// Keeping the interface in a top-level package (rather than internal/api)
// makes it available to future binaries — e.g. the background worker — without
// creating an import cycle.
package health

import "context"

// Checker reports the health of a single infrastructure component.
// Name returns a stable, lowercase key used as the field name in the JSON
// response (e.g. "db", "redis"). Check probes the component and returns a
// Result; it must respect ctx cancellation so the readiness handler can
// enforce an overall timeout.
type Checker interface {
	Name() string
	Check(ctx context.Context) Result
}

// Result is the outcome of a single health check.
// LatencyMS is omitted from the JSON output when zero (i.e. on error) so
// the response stays clean. Error is omitted when empty (i.e. on success).
type Result struct {
	Status    string `json:"status"`
	LatencyMS int64  `json:"latency_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// Response is the JSON body returned by GET /health/ready.
// Status is "ok" when every Checks entry is "ok", otherwise "error".
type Response struct {
	Status string            `json:"status"`
	Checks map[string]Result `json:"checks"`
}
