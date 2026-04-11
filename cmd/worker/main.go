// Command worker runs background event processing tasks for the World Cup
// quiniela application.
//
// The worker consumes domain events from the Redis Streams event bus and
// reacts to them asynchronously. Its primary responsibility today is scoring:
// when the API server publishes a MatchFinished event after an admin confirms
// a result, the worker calculates points for every prediction on that match
// and persists them to the database.
//
// Running scoring in the worker rather than inside the API server prevents
// background CPU work from competing with HTTP handlers for goroutines and
// database connections, and lets the two components be scaled independently
// based on their respective load profiles.
//
// The worker requires the Redis event bus driver (WCQ_EVENTBUS_DRIVER=redis).
// Starting it with the in-memory driver is rejected at startup: in-memory
// events are not visible across process boundaries and the worker would
// never receive any events.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
	"github.com/rede/world-cup-quiniela/pkg/logger"
)

func main() {
	cfg, err := config.LoadWorker()
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
		log.Sugar().Fatalf("worker: %v", err)
	}
}

// run bootstraps all worker dependencies and delegates lifecycle management
// to startWorker. It is extracted from main so that integration tests can
// drive the full startup sequence with a cancellable context without forking
// a process or intercepting os.Exit.
//
// Order of operations:
//  1. Validate that the event bus driver is "redis" (fail fast before I/O).
//  2. Open the event bus connection (Redis is the worker's primary interface).
//  3. Open the database pool.
//  4. Build repositories and the scoring service.
//  5. Create health checkers for readiness probes.
//  6. Delegate the subscriber + health server lifecycle to startWorker.
//
// The event bus is opened before the database because Redis is the worker's
// primary interface: if the event bus is unreachable, the worker cannot
// receive any events and has no useful work to do regardless of DB state.
// Detecting that failure first produces a clearer startup error.
func run(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	// Validate the event bus driver before establishing any connections.
	// Failing here surfaces a misconfiguration error without incurring the
	// latency of any dial that would ultimately be useless.
	if cfg.EventBus.Driver != "redis" {
		return fmt.Errorf(
			"worker requires eventBus.driver=redis (got %q); set WCQ_EVENTBUS_DRIVER=redis",
			cfg.EventBus.Driver,
		)
	}

	bus, closeBus, err := setupEventBus(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("event bus: %w", err)
	}
	defer closeBus()

	db, err := setupDB(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer db.Close()

	matchRepo := repository.NewPostgresMatchRepository(db)
	predRepo := repository.NewPostgresPredictionRepository(db)
	scorer := service.NewScoringService(matchRepo, predRepo, log)

	// A dedicated Redis client for health checks avoids sharing connections
	// with the event bus, whose long-lived XREADGROUP calls would otherwise
	// inflate the apparent latency of a ping.
	rc := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rc.Close() //nolint:errcheck

	checkers := []health.Checker{
		health.NewDBChecker(db),
		health.NewRedisChecker(rc),
	}

	return startWorker(ctx, cfg, bus, scorer, checkers, log)
}

// startWorker wires event subscribers, starts the health HTTP server, and
// blocks until ctx is cancelled (i.e. until an OS signal is received).
//
// All parameters are already constructed so this function has no I/O of its
// own. This makes it the boundary between infrastructure setup (run) and
// lifecycle management — and the part that can be exercised in unit tests
// by injecting an InMemoryBus, a stub scorer, and a pre-cancelled context.
func startWorker(
	ctx context.Context,
	cfg *config.Config,
	bus events.Bus,
	scorer service.MatchScorer,
	checkers []health.Checker,
	log *zap.Logger,
) error {
	bus.Subscribe(events.EventMatchFinished, newMatchFinishedHandler(scorer, log))
	log.Sugar().Info("worker: subscribed to MatchFinished events")

	healthSrv := newHealthServer(cfg.Worker.HealthPort, checkers, log)

	go func() {
		log.Sugar().Infof("worker health server listening on :%s", cfg.Worker.HealthPort)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Sugar().Fatalf("health server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Sugar().Info("worker: shutdown signal received, stopping...")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
	defer cancel()

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		log.Sugar().Errorf("worker: health server shutdown failed: %v", err)
	}
	log.Sugar().Info("worker stopped")
	return nil
}

// newHealthServer constructs the lightweight HTTP server that exposes liveness
// and readiness probes for the worker process.
func newHealthServer(port string, checkers []health.Checker, log *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleLiveness)
	mux.HandleFunc("/health/ready", handleReadiness(checkers, log))

	return &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
}

// handleLiveness responds to liveness probes. It only verifies that the
// process is alive — not that its dependencies are reachable.
func handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"world-cup-quiniela-worker"}`)
}

// handleReadiness runs all registered health checkers concurrently under a
// 5-second timeout and returns a JSON report. Returns 200 when every check
// passes or 503 when any check fails.
func handleReadiness(checkers []health.Checker, _ *zap.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()

		resp := health.Response{
			Status: "ok",
			Checks: make(map[string]health.Result, len(checkers)),
		}

		type item struct {
			name   string
			result health.Result
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
