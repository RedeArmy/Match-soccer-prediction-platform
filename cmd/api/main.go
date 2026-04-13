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

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
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

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Sugar().Fatalf("server: %v", err)
	}
}

// run bootstraps all application dependencies and manages the HTTP server
// lifecycle. It blocks until ctx is cancelled (i.e. until an OS signal is
// received) and then performs a graceful shutdown.
//
// Extracting lifecycle management from main makes every code path testable
// without forking a subprocess or intercepting os.Exit: tests can pass a
// pre-cancelled context to exercise the full startup → shutdown sequence in
// milliseconds, or an invalid DSN / Redis address to cover error branches.
//
// Order of operations:
//  1. Open the database pool and apply pending migrations.
//  2. Construct the event bus (in-memory or Redis, based on config).
//  3. Build health checkers for GET /health/ready.
//  4. Wire the HTTP server and start it in a background goroutine.
//  5. Block until ctx is cancelled or the server reports a fatal error.
//  6. Drain in-flight requests via graceful shutdown.
func run(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	// Infrastructure setup (DB pool, event bus) uses a background context so
	// that a cancellation of the lifecycle ctx (e.g. a SIGTERM arriving during
	// a slow cold-start) does not abort half-initialised resources mid-flight.
	// The lifecycle ctx is reserved for the HTTP server's select loop.
	setupCtx := context.Background()

	// The database connection is treated as optional at startup intentionally.
	// The /health endpoint must remain reachable even when the database is
	// temporarily unavailable — a common situation during rolling deployments
	// or cold-start sequences in container orchestration platforms. Handlers
	// that require a live connection will fail at request time rather than
	// preventing the entire process from starting.
	db, err := setupDB(setupCtx, cfg, log)
	if err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}
	if db != nil {
		defer db.Close()
	}

	// Select and construct the event bus implementation based on configuration.
	// The bus is constructed here — at the composition root — so its full
	// lifecycle (construction → subscriber wiring → graceful shutdown) is
	// visible in one place without any hidden state inside the Server.
	bus, closeBus, err := setupEventBus(setupCtx, cfg, log)
	if err != nil {
		return fmt.Errorf("event bus: %w", err)
	}
	defer closeBus()

	// Build health checkers for GET /health/ready. Each checker probes one
	// infrastructure dependency. The list is passed to the Server rather than
	// constructed inside it so that future binaries (e.g. the worker) can
	// register a different set of checkers without changing the Server itself.
	var checkers []health.Checker
	if db != nil {
		checkers = append(checkers, health.NewDBChecker(db))
	}
	if cfg.Redis.Addr != "" {
		// A dedicated client is used for health checks so that ping latency
		// reflects the health-check connection path rather than a shared pool
		// that may already be saturated by pub/sub or caching operations.
		rc := redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		defer rc.Close() //nolint:errcheck
		checkers = append(checkers, health.NewRedisChecker(rc))
	}

	// The api.Server owns the routing table and receives all shared
	// dependencies. Constructing it here — at the composition root —
	// rather than inside a package-level init function makes every
	// dependency explicit and eliminates hidden global state.
	app := api.New(db, cfg, log, bus, checkers)

	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      app.Routes(),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	srvErr := make(chan error, 1)
	go func() {
		log.Sugar().Infof("server listening on :%s", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	select {
	case err := <-srvErr:
		return fmt.Errorf("server: %w", err)
	case <-ctx.Done():
	}

	log.Sugar().Info("shutdown signal received, draining connections...")

	// Shutdown instructs the HTTP server to stop accepting new connections
	// and wait for in-flight requests to complete. The 30-second budget is
	// chosen to be longer than the slowest expected handler (a full scoring
	// recalculation) but shorter than the default Kubernetes termination
	// grace period (also 30 s by default — adjust both together if changed).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Sugar().Errorf("graceful shutdown failed: %v", err)
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	log.Sugar().Info("server stopped")
	return nil
}
