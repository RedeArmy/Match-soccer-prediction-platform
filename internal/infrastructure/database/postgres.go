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

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// poolConnectBackoff is the sequence of wait durations between connection
// attempts in NewPool. A new environment (fresh pod, cold CI runner) may
// require a few seconds for PostgreSQL to finish its own startup sequence.
// The total maximum wait is 1 + 2 + 4 + 8 = 15 seconds before the fourth
// attempt, after which the fifth attempt either succeeds or returns an error.
// EffectiveTLSMode returns a short human-readable description of the TLS mode
// that pgxpool will use when connecting to the DSN.  It is derived from the
// parsed TLS configuration rather than the raw DSN string so that both URL
// format (postgres://...?sslmode=require) and keyword=value format
// (sslmode=require host=...) are handled correctly.
//
// Intended for startup logging only — not for policy enforcement.
// Policy enforcement (rejecting sslmode=disable in production) is done in
// pkg/config.validateProductionDatabaseTLS.
func EffectiveTLSMode(dsn string) string {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return "unknown (DSN parse error)"
	}
	tls := cfg.ConnConfig.TLSConfig
	if tls == nil {
		return "disable"
	}
	if tls.InsecureSkipVerify {
		return "require (encrypted, certificate not verified)"
	}
	if tls.ServerName != "" {
		return "verify-full (encrypted, certificate + hostname verified)"
	}
	return "verify-ca (encrypted, CA verified)"
}

var poolConnectBackoff = []time.Duration{
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	8 * time.Second,
}

// Config holds the parameters required to create and tune the connection
// pool.
//
// These values are deliberately separate from pkg/config.DatabaseConfig.
// Keeping infrastructure packages free of dependencies on the application
// config package allows them to be used in CLI tools, migration runners,
// and integration tests that construct their own configuration without
// loading the full application config graph.
//
// ConnMaxIdleTime caps how long a connection may sit idle before pgxpool
// evicts it. A non-zero value prevents the pool from holding connections
// open indefinitely during traffic lulls, which keeps the server-side
// connection count predictable.
//
// ConnMaxLifetimeJitter adds random noise (up to the specified duration)
// to each connection's max-lifetime deadline, spreading eviction events
// across time and avoiding a thundering-herd reconnect storm when many
// connections reach their lifetime simultaneously.
type Config struct {
	DSN                   string
	MaxOpenConns          int
	MaxIdleConns          int
	ConnMaxLifetime       time.Duration
	ConnMaxIdleTime       time.Duration
	ConnMaxLifetimeJitter time.Duration
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
	poolCfg.MaxConnIdleTime = cfg.ConnMaxIdleTime
	poolCfg.MaxConnLifetimeJitter = cfg.ConnMaxLifetimeJitter

	// QueryExecModeCacheStatement instructs pgx to prepare each unique query
	// string the first time it is executed on a connection and reuse the cached
	// plan on subsequent calls. This avoids a round-trip parse/plan step for
	// every repeated query (e.g. GetByID, ListByMatch) without requiring the
	// application code to manage named prepared statements explicitly.
	// The cache is per-connection; pgxpool handles the lifecycle transparently.
	poolCfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeCacheStatement
	poolCfg.ConnConfig.Tracer = otelpgx.NewTracer()

	// Attempt to create the pool and ping the database with exponential
	// backoff. Fresh environments (new pods, CI containers starting postgres
	// as a sidecar) may need a few seconds before accepting connections.
	// The overall budget before returning an error is 1+2+4+8 = 15 seconds
	// of sleep plus the time for the five individual dial attempts.
	maxAttempts := len(poolConnectBackoff) + 1
	var (
		pool    *pgxpool.Pool
		lastErr error
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		pool, lastErr = pgxpool.NewWithConfig(ctx, poolCfg)
		if lastErr == nil {
			if lastErr = pool.Ping(ctx); lastErr == nil {
				return pool, nil
			}
			pool.Close()
		}
		if attempt == maxAttempts {
			break
		}
		wait := poolConnectBackoff[attempt-1]
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("connect database: context cancelled during retry: %w", ctx.Err())
		case <-time.After(wait):
		}
	}
	return nil, fmt.Errorf("connect database: %w (after %d attempts)", lastErr, maxAttempts)
}
