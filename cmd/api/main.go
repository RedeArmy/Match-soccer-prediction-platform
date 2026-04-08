// Command api is the entry point for the HTTP API server.
//
// This binary is responsible solely for wiring dependencies together and
// managing the application lifecycle. Business logic must not live here.
// Keeping this file intentionally thin ensures that each concern — config,
// logging, persistence, and routing — can be tested and replaced in isolation
// without modifying the composition root.
//
// @title           World Cup Quiniela API
// @version         1.0
// @description     REST API for the World Cup prediction game. Manage fixtures, submit score forecasts, and track leaderboards.
//
// @host            localhost:8080
// @BasePath        /
//
// @securityDefinitions.apikey  BearerAuth
// @in                          header
// @name                        Authorization
// @description                 Clerk JWT token. Format: "Bearer <token>"
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{
		Level:    cfg.Logger.Level,
		Encoding: cfg.Logger.Encoding,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	ctx := context.Background()

	// The database connection is treated as optional at startup intentionally.
	// The /health endpoint must remain reachable even when the database is
	// temporarily unavailable — a common situation during rolling deployments
	// or cold-start sequences in container orchestration platforms. Handlers
	// that require a live connection will fail at request time rather than
	// preventing the entire process from starting.
	db, err := setupDB(ctx, cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
		os.Exit(1)
	}
	if db != nil {
		defer db.Close()
	}

	// Select and construct the event bus implementation based on configuration.
	// The bus is constructed here — at the composition root — so its full
	// lifecycle (construction → subscriber wiring → graceful shutdown) is
	// visible in one place without any hidden state inside the Server.
	bus, closeBus, err := setupEventBus(ctx, cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "event bus error: %v\n", err)
		os.Exit(1)
	}
	defer closeBus()

	// The api.Server owns the routing table and receives all shared
	// dependencies. Constructing it here — at the composition root —
	// rather than inside a package-level init function makes every
	// dependency explicit and eliminates hidden global state.
	app := api.New(db, cfg, log, bus)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      app.Routes(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	go func() {
		log.Sugar().Infof("server listening on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Sugar().Fatalf("server error: %v", err)
		}
	}()

	// Block until the OS delivers SIGINT (Ctrl+C) or SIGTERM (sent by a
	// container orchestrator when stopping a pod). The buffered channel of
	// size 1 ensures the signal is not lost if it fires before we reach
	// this receive operation.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Sugar().Info("shutdown signal received, draining connections...")

	// Shutdown instructs the HTTP server to stop accepting new connections
	// and wait for in-flight requests to complete. The 30-second budget is
	// chosen to be longer than the slowest expected handler (a full scoring
	// recalculation) but shorter than the default Kubernetes termination
	// grace period (also 30 s by default — adjust both together if changed).
	// A non-zero exit code signals to the orchestrator that the shutdown
	// was not clean, triggering alerting or a restart policy.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Sugar().Errorf("graceful shutdown failed: %v", err)
		os.Exit(1)
	}
	log.Sugar().Info("server stopped")
}

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
// An unknown driver value causes an immediate fatal error at startup rather
// than silently falling back to a default, preventing configuration mistakes
// from going unnoticed in production.
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
		return messaging.NewInMemoryBus(), func() { /* nothing to release: in-memory bus has no external resources */ }, nil
	}
}

// setupDB applies pending migrations and opens a connection pool.
//
// Returns (nil, nil) when DSN is empty — the database is treated as optional
// so /health remains reachable during rolling deploys or cold starts.
// Returns (nil, error) when migrations fail — starting against a stale schema
// would cause unpredictable runtime errors, so the error is fatal at the
// call site. A failed pool ping is non-fatal: it logs a warning and returns
// (nil, nil) so the server can still start and serve non-DB endpoints.
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
		DSN:             cfg.Database.DSN,
		MaxOpenConns:    cfg.Database.MaxOpenConns,
		MaxIdleConns:    cfg.Database.MaxIdleConns,
		ConnMaxLifetime: cfg.Database.ConnMaxLifetime,
	})
	if err != nil {
		log.Sugar().Warnf("database unavailable: %v", err)
		return nil, nil
	}
	log.Sugar().Info("database connection established")
	return pool, nil
}
