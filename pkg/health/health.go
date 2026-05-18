// Package health defines the Checker interface and the response types used
// by the /health/ready readiness endpoint.
//
// Each infrastructure component (database, Redis, …) implements Checker and
// is registered with the Server at startup. The readiness handler runs all
// registered checkers concurrently, aggregates their results, and returns
// HTTP 200 when every check passes or HTTP 503 when at least one fails.
//
// Keeping the interface in a top-level package (rather than internal/api)
// makes it available to future binaries - e.g. the background worker - without
// creating an import cycle.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"
)

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

// MemStatsSnapshot captures a point-in-time view of the Go heap so that
// operators can observe memory pressure on the /health/ready endpoint without
// needing a pprof port or an external metrics agent. All byte values are raw
// (not MiB-converted) so dashboards can compute their own units.
type MemStatsSnapshot struct {
	HeapInuseBytes uint64  `json:"heap_inuse_bytes"`
	HeapSysBytes   uint64  `json:"heap_sys_bytes"`
	NumGC          uint32  `json:"num_gc"`
	GCCPUFraction  float64 `json:"gc_cpu_fraction"`
}

// Response is the JSON body returned by GET /health/ready.
// Status is "ok" when every Checks entry is "ok", otherwise "error".
// MemStats is always populated so operators can correlate heap usage with
// check failures without a separate profiling endpoint.
type Response struct {
	Status   string            `json:"status"`
	Checks   map[string]Result `json:"checks"`
	MemStats *MemStatsSnapshot `json:"mem_stats,omitempty"`
}

// ReadinessHandler returns an http.HandlerFunc that runs all registered
// checkers concurrently under a 5-second timeout and returns a JSON report.
// Returns HTTP 200 when every check passes, or 503 when any check fails.
//
// This consolidates the readiness probe logic shared by the API server and
// the background worker - both processes expose an identical /health/ready
// endpoint and previously duplicated this implementation verbatim.
func ReadinessHandler(checkers []Checker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)

		resp := Response{
			Status: "ok",
			Checks: make(map[string]Result, len(checkers)),
			MemStats: &MemStatsSnapshot{
				HeapInuseBytes: ms.HeapInuse,
				HeapSysBytes:   ms.HeapSys,
				NumGC:          ms.NumGC,
				GCCPUFraction:  ms.GCCPUFraction,
			},
		}

		type item struct {
			name   string
			result Result
		}
		ch := make(chan item, len(checkers))

		for _, c := range checkers {
			c := c
			go func() { ch <- item{c.Name(), c.Check(ctx)} }()
		}

		for range checkers {
			it := <-ch
			resp.Checks[it.name] = it.result
			if it.result.Status != "ok" {
				resp.Status = "error"
			}
		}

		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		if resp.Status != "ok" {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			w.WriteHeader(http.StatusOK)
		}
		_, _ = w.Write(data)
	}
}
