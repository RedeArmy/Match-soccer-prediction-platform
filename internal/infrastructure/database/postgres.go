// Package database provides the PostgreSQL connection pool used by all
// repository implementations in this service.
//
// pgx is chosen over the standard database/sql driver for two reasons.
// First, it exposes native PostgreSQL types - arrays, JSONB, UUIDs, custom
// enums - without requiring a sql.Scanner implementation for each, which
// reduces boilerplate and eliminates a category of runtime type-assertion
// panics. Second, pgxpool manages connection lifecycles more predictably
// than sql.DB under high-concurrency workloads, exposing explicit MinConns
// and MaxConnLifetime controls that sql.DB does not offer.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Config holds the parameters required to create and tune the connection
// pool.
//
// These values are deliberately separate from pkg/config.DatabaseConfig.
// Keeping infrastructure packages free of dependencies on the application
// config package allows them to be used in CLI tools, migration runners,
// and integration tests that construct their own configuration without
// loading the full application config graph.
type Config struct {
	DSN             string
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

// NewPool constructs a *pgxpool.Pool and verifies connectivity before
// returning it to the caller.
//
// The explicit Ping call is intentional: it causes the application to fail
// fast at startup if the database is unreachable, rather than surfacing
// connection errors on the first real query when the service is already
// serving traffic. Deterministic startup failures are far easier to
// diagnose in a deployment pipeline than flaky errors that appear minutes
// after a pod has been marked as ready.
//
// MinConns is mapped from MaxIdleConns. pgxpool keeps at least MinConns
// connections open at all times; a non-zero value avoids the latency spike
// that occurs when the pool has fully drained and must re-establish
// connections under a sudden burst of load.
//
// If Ping fails, the pool is closed before the error is returned. This is
// necessary because pgxpool spawns internal maintenance goroutines during
// creation; failing to close the pool leaks those goroutines even though
// the caller never uses the pool.
func NewPool(ctx context.Context, cfg Config) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.MaxConnLifetime = cfg.ConnMaxLifetime

	// QueryExecModeCacheStatement instructs pgx to prepare each unique query
	// string the first time it is executed on a connection and reuse the cached
	// plan on subsequent calls. This avoids a round-trip parse/plan step for
	// every repeated query (e.g. GetByID, ListByMatch) without requiring the
	// application code to manage named prepared statements explicitly.
	// The cache is per-connection; pgxpool handles the lifecycle transparently.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
