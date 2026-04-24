// Command worker runs background event processing tasks for the World Cup
// quiniela application.
//
// The worker consumes domain events from the Redis Streams event bus and
// reacts to them asynchronously. It handles two event types:
//
//   - EventMatchStarted: emitted when an admin transitions a match to Live.
//     The handler emits a structured audit log entry confirming that the
//     prediction window is now closed. Prediction enforcement itself is
//     synchronous in PredictionService; this handler exists for observability.
//
//   - EventMatchFinished: emitted when an admin confirms a match result.
//     The handler calls ScoringService to calculate and persist points for
//     every prediction on that match. On transient failure the bus retries
//     and, if all attempts are exhausted, routes the event to the dead-letter
//     queue for manual replay.
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
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
	"github.com/rede/world-cup-quiniela/pkg/logger"
)

// dlqMonitorInterval controls how often the DLQ monitoring goroutine logs
// the dead-letter queue state. Five minutes is frequent enough to surface a
// stuck queue within a reasonable SLA without spamming logs during normal
// operation.
const dlqMonitorInterval = 5 * time.Minute

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
	systemParamRepo := repository.NewPostgresSystemParamRepository(db)
	params := service.NewSystemParamService(systemParamRepo, log)
	scorer := service.NewScoringService(matchRepo, predRepo, params, log)

	quinielaRepo := repository.NewPostgresQuinielaRepository(db)
	userRepo := repository.NewPostgresUserRepository(db)
	tiebreakerRepo := repository.NewPostgresTiebreakerRepository(db)
	tiebreakerConfigRepo := repository.NewPostgresTiebreakerConfigRepository(db)
	snapRepo := repository.NewPostgresLeaderboardSnapshotRepository(db)
	ranker := service.NewRankingService(quinielaRepo, predRepo, userRepo, tiebreakerRepo, tiebreakerConfigRepo, params, log)
	snapshotter := service.NewLeaderboardSnapshotService(ranker, snapRepo)

	// A dedicated Redis client for health checks avoids sharing connections
	// with the event bus, whose long-lived XREADGROUP calls would otherwise
	// inflate the apparent latency of a ping.
	rc := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rc.Close() //nolint:errcheck

	return startWorker(ctx, cfg, bus, scorer, snapshotter, predRepo, rc, buildHealthCheckers(db, rc), log)
}

// startWorker wires event subscribers, starts the health HTTP server, starts
// the DLQ monitoring goroutine, and blocks until ctx is cancelled (i.e. until
// an OS signal is received).
//
// workerConsumerGroup is the Redis Streams consumer group name used by this
// worker process. Both the event bus and the stream-pending health checker
// must reference the same name to correctly report consumer lag.
const workerConsumerGroup = "quiniela-workers"

// buildHealthCheckers constructs the full set of readiness checkers for the
// worker process. Extracting this into its own function keeps run() readable
// and makes the checker list independently testable without needing a live
// database or Redis connection — the constructors are pure and only perform
// I/O when Check() is called.
func buildHealthCheckers(db *pgxpool.Pool, rc *redis.Client) []health.Checker {
	return []health.Checker{
		health.NewDBChecker(db),
		health.NewRedisChecker(rc),
		health.NewDLQChecker(rc, string(events.EventMatchStarted)),
		health.NewDLQChecker(rc, string(events.EventMatchFinished)),
		health.NewStreamPendingChecker(rc, "stream:"+string(events.EventMatchStarted), workerConsumerGroup),
		health.NewStreamPendingChecker(rc, "stream:"+string(events.EventMatchFinished), workerConsumerGroup),
	}
}

// All parameters are already constructed so this function has no I/O of its
// own. This makes it the boundary between infrastructure setup (run) and
// lifecycle management — and the part that can be exercised in unit tests
// by injecting an InMemoryBus, a stub scorer, and a pre-cancelled context.
func startWorker(
	ctx context.Context,
	cfg *config.Config,
	bus events.Bus,
	scorer service.MatchScorer,
	snapshotter service.Snapshotter,
	predRepo repository.PredictionRepository,
	rc *redis.Client,
	checkers []health.Checker,
	log *zap.Logger,
) error {
	bus.Subscribe(events.EventMatchStarted, newMatchStartedHandler(log))
	log.Sugar().Info("worker: subscribed to MatchStarted events")

	bus.Subscribe(events.EventMatchFinished, newMatchFinishedHandler(scorer, snapshotter, predRepo, log))
	log.Sugar().Info("worker: subscribed to MatchFinished events")

	healthSrv := newHealthServer(cfg.Worker.HealthPort, checkers, log)

	go func() {
		log.Sugar().Infof("worker health server listening on :%s", cfg.Worker.HealthPort)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Sugar().Fatalf("health server error: %v", err)
		}
	}()

	// DLQ monitoring goroutine: logs the size of each dead-letter queue at a
	// fixed interval. Operators should configure an alert on log lines that
	// contain "dead-letter queue is non-empty" to be notified within one
	// dlqMonitorInterval of a scoring failure.
	go monitorDLQ(ctx, rc, log)

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

// monitorDLQ runs until ctx is cancelled, periodically logging the size of
// every event-type dead-letter queue managed by this worker. A non-zero count
// indicates events that exhausted all handler retry attempts and require manual
// operator replay. The log line is structured so log-based alerting systems
// (Datadog, CloudWatch Logs Insights, Loki) can match on the "dlq_size" field.
//
// If rc is nil (e.g. in unit tests where Redis is not available), the goroutine
// exits immediately — DLQ monitoring is best-effort and must not block startup.
func monitorDLQ(ctx context.Context, rc *redis.Client, log *zap.Logger) {
	if rc == nil {
		return
	}
	ticker := time.NewTicker(dlqMonitorInterval)
	defer ticker.Stop()

	// The event types whose DLQ keys this worker is responsible for.
	monitoredEvents := []events.EventType{events.EventMatchStarted, events.EventMatchFinished}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for _, et := range monitoredEvents {
				dlqKey := "dlq:" + string(et)
				n, err := rc.LLen(ctx, dlqKey).Result()
				if err != nil {
					log.Warn("worker: DLQ monitor: LLEN failed",
						zap.String("dlq_key", dlqKey),
						zap.Error(err),
					)
					continue
				}
				if n > 0 {
					log.Error("worker: dead-letter queue is non-empty — manual replay required",
						zap.String("dlq_key", dlqKey),
						zap.String("event_type", string(et)),
						zap.Int64("dlq_size", n),
					)
				} else {
					log.Debug("worker: DLQ monitor: queue is empty",
						zap.String("dlq_key", dlqKey),
					)
				}
			}
		}
	}
}

// newHealthServer constructs the lightweight HTTP server that exposes liveness
// and readiness probes for the worker process.
func newHealthServer(port string, checkers []health.Checker, log *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleLiveness)
	mux.HandleFunc("/health/ready", health.ReadinessHandler(checkers))

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
