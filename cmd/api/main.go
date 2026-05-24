// Command api is the entry point for the HTTP API server.
//
// This binary is responsible solely for wiring dependencies together and
// managing the application lifecycle. Business logic must not live here.
// Keeping this file intentionally thin ensures that each concern - config,
// logging, persistence, and routing - can be tested and replaced in isolation
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

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
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
		InitialFields: []zap.Field{
			zap.String("service", cfg.Tracing.ServiceName),
			zap.String("env", cfg.Environment),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	// Wire the defensive logger for repository deferred rollback failures.
	// Must be called after logger initialization but before any repository methods.
	repository.SetDefensiveLogger(log)

	logStartupBanner(cfg, log)

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
// pre-cancelled context to exercise the full startup -> shutdown sequence in
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
	// Infrastructure setup (DB pool, event bus) must not be interrupted by a
	// lifecycle cancellation that arrives during a slow cold-start. Using
	// WithoutCancel derives a child that inherits ctx's values but is never
	// cancelled, keeping setup isolated from SIGTERM timing.
	setupCtx := context.WithoutCancel(ctx)

	shutdownTracing, err := setupTracing(setupCtx, cfg, log)
	if err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	defer flushShutdown(ctx, shutdownTracing, "tracing", log)

	metricsHandler, shutdownMetrics, err := setupMetrics(cfg, log)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	defer flushShutdown(ctx, shutdownMetrics, "metrics", log)

	// The database connection is treated as optional at startup intentionally.
	// The /health endpoint must remain reachable even when the database is
	// temporarily unavailable - a common situation during rolling deployments
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
	// The bus is constructed here - at the composition root - so its full
	// lifecycle (construction -> subscriber wiring -> graceful shutdown) is
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

	// The cache store is constructed once and shared between the health
	// checker, the event bus (if redis driver), and the cached service
	// decorators. A dedicated client is used for caching to avoid contention
	// with the long-lived XREADGROUP connections of the event bus.
	//
	// rc is declared outside the if block so it is visible when wiring the
	// Redis-backed idempotency store below.
	// cacheStore defaults to an in-process MemoryStore so the composition root
	// never passes nil to service constructors. The MemoryStore is not suitable
	// for multi-replica production deployments (invalidations are local and
	// entries never expire), but it provides correct single-replica behaviour
	// when Redis is not configured.
	var rc *redis.Client
	cacheStore := cache.Store(cache.NewMemoryStore())
	if cfg.Redis.Addr != "" {
		rc = redis.NewClient(&redis.Options{
			Addr:     cfg.Redis.Addr,
			Password: cfg.Redis.Password,
			DB:       cfg.Redis.DB,
		})
		defer rc.Close() //nolint:errcheck
		checkers = append(checkers, health.NewRedisChecker(rc))
		cacheStore = cache.NewRedisStore(rc)
	}

	// The api.Server owns the routing table and receives all shared
	// dependencies. Constructing it here - at the composition root -
	// rather than inside a package-level init function makes every
	// dependency explicit and eliminates hidden global state.
	app := api.New(db, cfg, log, bus, cacheStore, checkers)

	// When Redis is available, use a shared idempotency store so reservations
	// are visible across all replicas. Falls back to MemoryStore (set inside
	// Routes()) when Redis is not configured.
	if rc != nil {
		app.SetIdempotencyStore(idempotency.NewRedisStore(rc))
	}

	// Wire the /metrics endpoint. SetMetricsHandler is nil-safe: when metrics
	// are disabled metricsHandler is nil and Routes() omits the /metrics route.
	app.SetMetricsHandler(metricsHandler)

	// Wire the observability notifier. Disabled when WCQ_N8N_BASEURL is empty.
	app.SetNotifier(setupObservabilityNotifier(cfg, log))

	// setupCtx is context.WithoutCancel(ctx): OTel trace values are propagated
	// to startup DB reads while SIGTERM cannot abort them.
	srv := &http.Server{
		Addr:         ":" + cfg.Server.Port,
		Handler:      app.Routes(setupCtx),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
		IdleTimeout:  cfg.Server.IdleTimeout,
	}

	// Prefer the Redis Pub/Sub bridge when Redis is available: reconnections
	// are transparent (< 100 ms) and no long-lived database connection is held.
	// Fall back to the pg_notify bridge for single-server deployments.
	if rc != nil {
		app.StartRedisBridge(ctx, rc)
	} else {
		app.StartPgNotifyBridge(ctx)
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
	// grace period (also 30 s by default - adjust both together if changed).
	// ctx is already cancelled at this point (it unblocked the select above),
	// so WithoutCancel is required to give the timeout a valid parent.
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Sugar().Errorf("graceful shutdown failed: %v", err)
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	// Drain in-flight audit writes before closing the database pool. This
	// prevents losing audit entries that were queued during request processing
	// but not yet persisted. The audit service write timeout (default 5 s)
	// caps the maximum wait per goroutine, so this returns promptly even under
	// high load. Must run after srv.Shutdown to ensure no new audit entries
	// can be queued while we drain.
	log.Sugar().Info("draining audit log writes...")
	app.DrainAudit()

	// Cancel the pg_notify bridge goroutine and wait for it to exit before
	// db.Close() runs (deferred above). The bridge holds an acquired pool
	// connection; releasing it first prevents pool.Close() from deadlocking
	// on WG.Wait().
	log.Sugar().Info("stopping pg_notify bridge...")
	app.StopPgNotifyBridge()

	log.Sugar().Info("server stopped")
	return nil
}
