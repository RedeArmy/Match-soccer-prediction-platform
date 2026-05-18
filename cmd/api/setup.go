package main

// setup.go contains the infrastructure bootstrap helpers used by main.
//
// Each function has a single responsibility: construct one dependency and
// return it (plus a cleanup function where applicable). Keeping them in a
// separate file from main.go makes the composition root easier to read -
// main() describes the application lifecycle while setup.go describes how
// each dependency is wired - and allows the helpers to be tested in
// isolation without starting a full HTTP server.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// setupEventBus constructs the appropriate events.Bus implementation based on
// the WCQ_EVENTBUS_DRIVER configuration value and returns a cleanup function
// that must be deferred by the caller to release resources on shutdown.
//
// Supported drivers:
//   - "in_memory" (default): synchronous InMemoryBus; no external dependencies.
//     Safe for single-replica deployments and local development only.
//   - "redis": asynchronous RedisBus backed by the configured Redis instance.
//     Required when running multiple API replicas so that domain events
//     (e.g. MatchFinished) reach all replicas and the worker process.
//
// An unknown driver value falls back to in_memory with a warning rather than
// crashing, keeping deployments recoverable from configuration typos.
func setupEventBus(ctx context.Context, cfg *config.Config, log *zap.Logger) (events.Bus, func(), error) {
	switch cfg.EventBus.Driver {
	case "redis":
		// Construct a dedicated Redis client for the event bus. Although the
		// same Redis instance may also be used for caching, a separate client
		// is intentional: pub/sub connections are long-lived blocking calls that
		// should not compete for connections with short-lived cache operations.
		redisClient, err := cache.NewClient(ctx, cache.Config{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		if err != nil {
			// No resources were allocated before the connection failed, so the
			// cleanup function is a no-op. The caller still defers it for a
			// consistent call-site pattern regardless of which driver is used.
			return nil, func() { /* nothing to release: Redis client was never opened */ }, fmt.Errorf("redis bus: connect: %w", err)
		}
		log.Sugar().Infof("event bus: using Redis driver (%s)", cfg.Redis.Addr)

		bus := messaging.NewRedisBus(redisClient, log)

		// The cleanup function stops all subscription goroutines and closes
		// the Redis client. Both steps are required to avoid goroutine leaks
		// and connection exhaustion during graceful shutdown.
		cleanup := func() {
			bus.Close()
			if err := redisClient.Close(); err != nil {
				log.Sugar().Warnf("redis bus: close client: %v", err)
			}
		}
		return bus, cleanup, nil

	default:
		// "in_memory" is the default. Any unrecognised value also falls here
		// to keep startup safe; a warning is logged so mis-spellings are visible.
		if cfg.EventBus.Driver != "in_memory" {
			log.Sugar().Warnf("event bus: unknown driver %q, falling back to in_memory", cfg.EventBus.Driver)
		} else {
			log.Sugar().Info("event bus: using in_memory driver (single-replica only)")
		}
		// InMemoryBus holds no external connections or goroutines, so there is
		// nothing to close on shutdown. The no-op cleanup keeps the call-site
		// pattern uniform: the caller always defers closeBus() without needing
		// to know which driver is active.
		return messaging.NewInMemoryBus(log), func() { /* nothing to release: in-memory bus has no external resources */ }, nil
	}
}

// setupDB applies pending migrations and opens a connection pool.
//
// Returns (nil, nil) when DSN is empty - the server is intentionally started
// without a database only in that case (e.g. a /health-only smoke test).
// Any other failure - migration error or pool creation error - is returned as
// a non-nil error so that main can call os.Exit(1) immediately.  Allowing the
// server to start with a configured-but-unreachable database would serve
// requests that silently fail at the query layer, which is far harder to
// diagnose than a clean startup failure.
//
// Extracting this logic out of main keeps the composition root thin and lets
// tests exercise the migration and connection paths without spawning a full
// server or intercepting os.Exit.
func setupDB(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, error) {
	if cfg.Database.DSN == "" {
		return nil, nil
	}

	// Apply pending migrations before opening the connection pool.
	// golang-migrate holds a PostgreSQL advisory lock during execution, so
	// concurrent starts (multiple replicas) are safe: one instance applies
	// the migrations while the others wait and then find no further changes.
	// A migration failure is fatal: starting the server against an
	// out-of-date schema would cause unpredictable runtime errors that are
	// far harder to diagnose than a clean startup failure.
	log.Sugar().Info("applying database migrations...")
	if err := database.Migrate(cfg.Database.DSN, migrations.FS); err != nil {
		return nil, fmt.Errorf("migration failed: %w", err)
	}
	log.Sugar().Info("migrations up to date")

	pool, err := database.NewPool(ctx, database.Config{
		DSN:                   cfg.Database.DSN,
		MaxOpenConns:          cfg.Database.MaxOpenConns,
		MaxIdleConns:          cfg.Database.MaxIdleConns,
		ConnMaxLifetime:       cfg.Database.ConnMaxLifetime,
		ConnMaxIdleTime:       cfg.Database.ConnMaxIdleTime,
		ConnMaxLifetimeJitter: cfg.Database.ConnMaxLifetimeJitter,
	})
	if err != nil {
		return nil, fmt.Errorf("database unavailable: %w", err)
	}
	log.Sugar().Info("database connection established")
	return pool, nil
}

// logStartupBanner emits a structured summary of the application configuration
// immediately after logger initialisation. This makes critical settings visible
// at the top of the log stream rather than buried in startup messages, and
// surfaces misconfigurations before any infrastructure connections are attempted.
//
// The banner format is intentional: parseable by both humans (CloudWatch console,
// kubectl logs) and log aggregation systems (grep, awk, structured filters).
func logStartupBanner(cfg *config.Config, log *zap.Logger) {
	log.Sugar().Info("╔═══════════════════════════════════════════════════════════╗")
	log.Sugar().Info("║              World Cup Quiniela API                       ║")
	log.Sugar().Info("╠═══════════════════════════════════════════════════════════╣")
	log.Sugar().Infof("║ Environment:      %-37s ║", cfg.Environment)
	log.Sugar().Infof("║ Event Bus Driver: %-37s ║", cfg.EventBus.Driver)
	log.Sugar().Infof("║ Database:         %-37s ║", maskDSN(cfg.Database.DSN))
	log.Sugar().Infof("║ Redis:            %-37s ║", cfg.Redis.Addr)
	log.Sugar().Infof("║ Server Port:      %-37s ║", cfg.Server.Port)
	log.Sugar().Info("╚═══════════════════════════════════════════════════════════╝")

	// Emit a machine-parseable structured log for automated alerting.
	// Log aggregation systems can match on event_bus_driver="in_memory" to
	// detect misconfigured deployments even if the validation was bypassed.
	log.Info("startup configuration loaded",
		zap.String("environment", cfg.Environment),
		zap.String("event_bus_driver", cfg.EventBus.Driver),
		zap.String("redis_addr", cfg.Redis.Addr),
		zap.String("server_port", cfg.Server.Port),
	)
}

// maskDSN redacts credentials from a PostgreSQL connection string for safe
// logging. Returns "not configured" when DSN is empty, preventing confusing
// blank lines in the startup banner.
func maskDSN(dsn string) string {
	if dsn == "" {
		return "not configured"
	}
	// DSN format: postgres://user:pass@host:port/db?params
	// Mask everything between "://" and "@" to hide username:password.
	if idx := strings.Index(dsn, "://"); idx != -1 {
		if idx2 := strings.Index(dsn[idx+3:], "@"); idx2 != -1 {
			return dsn[:idx+3] + "***:***" + dsn[idx+3+idx2:]
		}
	}
	// Fallback: DSN format is unexpected; mask the whole string to be safe.
	return "***masked***"
}
