package health

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// DBChecker implements Checker for a PostgreSQL connection pool.
// It calls pool.Ping, which acquires a connection and sends a trivial query,
// confirming that the pool can reach the database.
type DBChecker struct {
	pool *pgxpool.Pool
}

// NewDBChecker returns a DBChecker backed by pool.
func NewDBChecker(pool *pgxpool.Pool) *DBChecker {
	return &DBChecker{pool: pool}
}

// Name returns "db", the key used in the readiness response JSON.
func (c *DBChecker) Name() string { return "db" }

// Check pings the database and returns the result. Latency is measured from
// the moment Ping is called to the moment it returns; it reflects round-trip
// time plus any connection-acquisition overhead.
func (c *DBChecker) Check(ctx context.Context) Result {
	start := time.Now()
	if err := c.pool.Ping(ctx); err != nil {
		return Result{Status: "error", Error: err.Error()}
	}
	return Result{Status: "ok", LatencyMS: time.Since(start).Milliseconds()}
}
