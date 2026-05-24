package main

// setup.go contains the infrastructure bootstrap helpers used by the worker's
// main function. Each function constructs exactly one dependency and returns
// a cleanup function where applicable, keeping main.go focused on lifecycle
// management rather than construction details.

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/logger"
	"github.com/rede/world-cup-quiniela/pkg/metrics"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

// flushShutdown calls shutdown with a 5-second deadline derived from ctx and
// logs any failure. Used as a deferred cleanup for tracing and metrics providers.
func flushShutdown(ctx context.Context, shutdown func(context.Context) error, label string, log *zap.Logger) {
	flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := shutdown(flushCtx); err != nil {
		log.Sugar().Warnf("%s flush: %v", label, err)
	}
}

// setupTracing initialises the global OpenTelemetry TracerProvider and returns
// a shutdown function that flushes pending spans on process exit.
//
// When tracing is disabled (cfg.Tracing.Enabled == false) the function
// installs a no-op provider and returns a no-op shutdown — no network
// connections are made and the call completes in nanoseconds.
func setupTracing(ctx context.Context, cfg *config.Config, log *zap.Logger) (func(context.Context) error, error) {
	tc := tracing.Config{
		Enabled:        cfg.Tracing.Enabled,
		OTLPEndpoint:   cfg.Tracing.OTLPEndpoint,
		ServiceName:    cfg.Tracing.ServiceName,
		ServiceVersion: cfg.Tracing.ServiceVersion,
		Environment:    cfg.Environment,
		SampleRate:     cfg.Tracing.SampleRate,
	}
	shutdown, err := tracing.Setup(ctx, tc)
	if err != nil {
		return nil, fmt.Errorf("tracing: %w", err)
	}
	if cfg.Tracing.Enabled {
		log.Info("tracing enabled",
			zap.String("otlp_endpoint", cfg.Tracing.OTLPEndpoint),
			zap.String("service_name", cfg.Tracing.ServiceName),
			zap.Float64("sample_rate", cfg.Tracing.SampleRate),
		)
	} else {
		log.Info("tracing disabled (noop provider)")
	}
	return shutdown, nil
}

// setupDB opens a connection pool to the primary PostgreSQL database.
//
// setupMetrics initialises the global OTel MeterProvider backed by a Prometheus
// exporter and returns the meter, the /metrics HTTP handler, and a shutdown function.
// The /metrics handler is registered on the worker's health server mux when metrics
// are enabled. The returned meter is always non-nil (noop when disabled) so callers
// can create instruments unconditionally.
func setupMetrics(cfg *config.Config, log *zap.Logger) (metric.Meter, http.Handler, func(context.Context) error, error) {
	meter, handler, shutdown, err := metrics.Setup(metrics.Config{
		Enabled:   cfg.Metrics.Enabled,
		Namespace: cfg.Metrics.Namespace,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("metrics: %w", err)
	}
	if cfg.Metrics.Enabled {
		log.Info("metrics enabled",
			zap.String("namespace", cfg.Metrics.Namespace),
		)
	} else {
		log.Info("metrics disabled (noop provider)")
	}
	return meter, handler, shutdown, nil
}

// Unlike the API server's setupDB, this function intentionally does NOT run
// database migrations. Migrations are applied exclusively by the API server,
// which holds a PostgreSQL advisory lock during the operation. If the worker
// also attempted migrations, the two processes would contend for the lock on
// startup, causing one to fail. The worker must therefore assume that the
// schema is already up to date before it starts.
func setupDB(ctx context.Context, cfg *config.Config, log *zap.Logger) (*pgxpool.Pool, error) {
	if cfg.Database.DSN == "" {
		return nil, fmt.Errorf("database DSN is required (WCQ_DATABASE_DSN)")
	}

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

// setupEventBus constructs the Redis-backed event bus required by the worker.
//
// The worker only functions correctly with the Redis driver. In-memory events
// are not visible across process boundaries, so an in-memory bus would never
// receive the MatchFinished events published by the API server. Rather than
// silently doing nothing, this function fails fast with an actionable error
// message when the driver is not "redis".
func setupEventBus(ctx context.Context, cfg *config.Config, log *zap.Logger) (events.Bus, func(), error) {
	if cfg.EventBus.Driver != "redis" {
		return nil, func() { /* no-op: no connection was established */ }, fmt.Errorf(
			"worker requires eventBus.driver=redis, got %q; set WCQ_EVENTBUS_DRIVER=redis",
			cfg.EventBus.Driver,
		)
	}

	redisClient, err := cache.NewClient(ctx, cache.Config{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	if err != nil {
		return nil, func() { /* no-op: no connection was established */ }, fmt.Errorf("redis bus: connect: %w", err)
	}
	log.Sugar().Infof("event bus: using Redis driver (%s)", cfg.Redis.Addr)

	bus := messaging.NewRedisBus(redisClient, log)

	cleanup := func() {
		bus.Close()
		if err := redisClient.Close(); err != nil {
			log.Sugar().Warnf("redis bus: close client: %v", err)
		}
	}
	return bus, cleanup, nil
}

// setupObservabilityNotifier constructs the observability Notifier from config.
// When WCQ_N8N_BASEURL is empty the notifier is disabled (all methods no-op).
func setupObservabilityNotifier(cfg *config.Config, log *zap.Logger) *observability.Notifier {
	n := observability.New(observability.NotifierConfig{
		BaseURL: cfg.N8n.BaseURL,
		Secret:  cfg.N8n.WebhookSecret,
		Log:     log,
	})
	if n.Enabled() {
		log.Info("observability notifier enabled", zap.String("n8n_base_url", cfg.N8n.BaseURL))
	} else {
		log.Info("observability notifier disabled (WCQ_N8N_BASEURL not set)")
	}
	return n
}

// logStartupBanner emits a structured summary of the worker configuration
// immediately after logger initialisation. Matches the API banner format for
// consistency, making it easy to compare configuration across processes when
// debugging multi-process deployment issues.
func logStartupBanner(cfg *config.Config, log *zap.Logger) {
	log.Sugar().Info("╔═══════════════════════════════════════════════════════════╗")
	log.Sugar().Info("║          World Cup Quiniela Worker                        ║")
	log.Sugar().Info("╠═══════════════════════════════════════════════════════════╣")
	log.Sugar().Infof("║ Environment:      %-37s ║", cfg.Environment)
	log.Sugar().Infof("║ Event Bus Driver: %-37s ║", cfg.EventBus.Driver)
	log.Sugar().Infof("║ Database:         %-37s ║", maskDSN(cfg.Database.DSN))
	log.Sugar().Infof("║ Redis:            %-37s ║", cfg.Redis.Addr)
	log.Sugar().Infof("║ Health Port:      %-37s ║", cfg.Worker.HealthPort)
	log.Sugar().Infof("║ Tracing:          %-37s ║", tracingStatus(cfg))
	log.Sugar().Info("╚═══════════════════════════════════════════════════════════╝")

	log.Info("worker configuration loaded",
		zap.String("environment", cfg.Environment),
		zap.String("event_bus_driver", cfg.EventBus.Driver),
		zap.String("redis_addr", cfg.Redis.Addr),
		zap.String("health_port", cfg.Worker.HealthPort),
		zap.Bool("tracing_enabled", cfg.Tracing.Enabled),
	)
}

// tracingStatus returns a human-readable tracing status string for the startup banner.
func tracingStatus(cfg *config.Config) string {
	if cfg.Tracing.Enabled {
		return "enabled → " + cfg.Tracing.OTLPEndpoint
	}
	return "disabled (noop)"
}

// wireLogLevelCounters attaches OTel counters for warn and error log entries
// to the logger. Mirrors the API server helper so both processes export the
// same metric names. When metrics are disabled meter is a noop.
func wireLogLevelCounters(log *zap.Logger, meter metric.Meter) *zap.Logger {
	warnCtr, _ := meter.Int64Counter("log.warnings",
		metric.WithDescription("Total Warn-level log entries emitted by the worker"),
	)
	errCtr, _ := meter.Int64Counter("log.errors",
		metric.WithDescription("Total Error-level and above log entries emitted by the worker"),
	)
	return logger.WithLevelCounters(log,
		func() { warnCtr.Add(context.Background(), 1) },
		func() { errCtr.Add(context.Background(), 1) },
	)
}

// maskDSN redacts credentials from a PostgreSQL connection string for safe
// logging. Returns "not configured" when DSN is empty.
func maskDSN(dsn string) string {
	if dsn == "" {
		return "not configured"
	}
	if idx := strings.Index(dsn, "://"); idx != -1 {
		if idx2 := strings.Index(dsn[idx+3:], "@"); idx2 != -1 {
			return dsn[:idx+3] + "***:***" + dsn[idx+3+idx2:]
		}
	}
	return "***masked***"
}
