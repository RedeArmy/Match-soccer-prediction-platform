// Package api wires together the HTTP server, middleware, and route handlers
// for the World Cup quiniela REST API.
//
// The Server type is the central composition point for all HTTP-layer
// dependencies. It receives infrastructure clients at construction time and
// exposes a single Routes method that returns a fully configured http.Handler.
//
// This design has an important testability consequence: tests can call
// Routes() and pass the returned handler directly to httptest.NewRecorder
// without starting a real network listener or requiring a live database.
// The entire HTTP surface is therefore exercisable in milliseconds, with no
// external dependencies.
package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

// Server holds the shared dependencies made available to all HTTP handlers.
// It is constructed once at application startup and is safe for concurrent
// use by multiple goroutines once initialised.
type Server struct {
	// db may be nil if the database was unavailable at startup. Handlers that
	// require a live connection must guard against nil and return a 503 rather
	// than dereferencing a nil pointer. This allows infrastructure endpoints
	// such as /health to remain reachable during transient database outages.
	db  *pgxpool.Pool
	cfg *config.Config
	log *zap.Logger
	// bus publishes and receives domain events; driver selected via WCQ_EVENTBUS_DRIVER.
	bus events.Bus
	// cache is optional; nil disables caching and all requests hit the database directly.
	cache    cache.Store
	checkers []health.Checker
	// dlqSvc is nil when the event bus driver is not "redis"; admin DLQ endpoints use NoopDLQService.
	dlqSvc service.DLQService
	// auditSvc is set by Routes() after constructing the service; exposed to main.go
	// so the shutdown path can call Drain() to wait for in-flight audit writes.
	auditSvc service.AuditService
	// limiterStore overrides the per-user rate limiter constructed by Routes().
	// Nil means Routes() falls back to an in-process LimiterStore. Use
	// SetLimiterStore (tests) or SetRateStore (Redis wiring in production).
	limiterStore middleware.Allower
	// notifHub is the in-process SSE hub; created once in Routes() and reused
	// by the notification handler and the pg_notify bridge goroutine.
	notifHub *hub.Hub
	// stopBridge cancels the pg_notify bridge goroutine context; nil until Routes() is called.
	stopBridge context.CancelFunc
	// bridgeDone is closed when the pg_notify bridge goroutine exits.
	bridgeDone <-chan struct{}
	// idemStore is the idempotency backing store. When non-nil (Redis available),
	// reservations are shared across all replicas. Falls back to MemoryStore when nil.
	idemStore idempotency.Store
	// redisClient is the shared Redis connection. Non-nil when WCQ_REDIS_ADDR is
	// configured. Routes() uses it to build a RedisRateStore for cross-replica
	// rate limiting; a nil value falls back to the in-process LimiterStore.
	redisClient redis.UniversalClient
	// notifier ships structured JSON events to n8n webhooks for operational
	// alerting. Nil until SetNotifier is called; buildHandlers injects it into
	// the payment-path handlers that trigger observability events.
	notifier *observability.Notifier
	// breakerRegistry holds all active circuit breakers registered at startup.
	// Nil until SetBreakerRegistry is called; the observability circuit-breakers
	// endpoint uses it to report current breaker states.
	breakerRegistry *breaker.Registry
}

// SetDLQService wires an optional DLQService for the admin /dlq endpoints.
// Call this after New() when the Redis event bus driver is active.
func (s *Server) SetDLQService(dlq service.DLQService) { s.dlqSvc = dlq }

// SetLimiterStore overrides the per-user rate limiter with an in-process
// token-bucket store. Intended for tests that need to exercise the full
// middleware chain without triggering 429 responses; pass
// middleware.NewUnlimitedLimiterStore() to disable rate limiting for the test.
func (s *Server) SetLimiterStore(store *middleware.LimiterStore) { s.limiterStore = store }

// SetRateStore overrides the per-user rate limiter with any Allower
// implementation. Use this in production to wire a Redis-backed store when
// the Redis client is available, so rate limits are enforced across replicas.
func (s *Server) SetRateStore(store middleware.Allower) { s.limiterStore = store }

// SetRedisClient provides the shared Redis connection for use by Routes().
// When set, Routes() builds a RedisRateStore so rate limits are enforced across
// all replicas. When nil (default), an in-process LimiterStore is used instead.
// Call before Routes() in production; not required in tests.
func (s *Server) SetRedisClient(rc redis.UniversalClient) { s.redisClient = rc }

// SetIdempotencyStore replaces the idempotency store used by the payment write
// endpoints. Must be called before Routes() so the middleware captures the
// configured store. When Redis is available, pass idempotency.NewRedisStore(rc)
// to make reservations visible across all replicas; the MemoryStore fallback
// (used when this method is never called) is only safe for single-process
// deployments.
func (s *Server) SetIdempotencyStore(store idempotency.Store) { s.idemStore = store }

// ensureIdempotencyStore guarantees s.idemStore is non-nil before Routes()
// registers any payment endpoints. Returns true when it fell back to the
// in-process MemoryStore (Redis was not configured), false when a Redis-backed
// store was already wired via SetIdempotencyStore. Callers use the return value
// to emit a startup OTel counter so the WCQIdempotencyDegraded alert fires
// immediately rather than remaining silent until the first payment request.
func (s *Server) ensureIdempotencyStore() (degraded bool) {
	if s.idemStore != nil {
		return false
	}
	s.log.Warn("idempotency: Redis not configured — using in-process MemoryStore",
		zap.Bool("single_process_only", true),
		zap.String("remedy", "set WCQ_REDIS_ADDR; without it duplicate payment requests can commit on separate replicas"),
	)
	s.idemStore = idempotency.NewMemoryStore()
	return true
}

// SetNotifier wires the observability Notifier for operational alerting via
// n8n webhooks. Call before Routes() so buildHandlers can inject it into the
// payment-path handlers. When n is nil, all notification calls are no-ops.
func (s *Server) SetNotifier(n *observability.Notifier) { s.notifier = n }

// SetBreakerRegistry registers the circuit breaker registry so the admin
// observability endpoints can enumerate breaker states. Call before Routes().
func (s *Server) SetBreakerRegistry(r *breaker.Registry) { s.breakerRegistry = r }

// DrainAudit blocks until all in-flight audit log writes complete. Must be
// called during graceful shutdown before closing the database connection pool
// to prevent losing audit entries that were queued but not yet persisted.
// Safe to call even if auditSvc is nil (no-op).
func (s *Server) DrainAudit() {
	if s.auditSvc != nil {
		s.auditSvc.Drain()
	}
}

// StartPgNotifyBridge starts the pg_notify bridge goroutine under a
// cancellable context. Call this once after Routes() from the process entry
// point (cmd/api/main.go). It is intentionally NOT called inside Routes() so
// that tests which create a Server and call Routes() without a corresponding
// Stop do not leak a goroutine that holds a pool connection.
//
// ctx provides values (OTel trace IDs, request IDs) that the bridge goroutine
// inherits for structured logging and distributed tracing. Its cancellation
// signal is stripped via context.WithoutCancel so that a SIGTERM that cancels
// the caller's context does not immediately abort the bridge; the bridge is
// stopped explicitly by StopPgNotifyBridge at the right point in the shutdown
// sequence, after in-flight HTTP connections have been drained.
func (s *Server) StartPgNotifyBridge(ctx context.Context) {
	bridgeCtx, bridgeCancel := context.WithCancel(context.WithoutCancel(ctx))
	s.stopBridge = bridgeCancel
	done := make(chan struct{})
	s.bridgeDone = done
	go func() {
		defer close(done)
		s.runPgNotifyBridge(bridgeCtx)
	}()
}

// StopPgNotifyBridge cancels the SSE bridge goroutine (pg_notify or Redis
// Pub/Sub depending on which Start method was called) and waits for it to exit.
// Must be called before closing the database pool when using the pg_notify
// bridge so the acquired connection is released before pool.Close().
// Safe to call if no bridge was started (no-op in that case).
func (s *Server) StopPgNotifyBridge() {
	if s.stopBridge != nil {
		s.stopBridge()
	}
	if s.bridgeDone != nil {
		<-s.bridgeDone
	}
}

// StartRedisBridge starts the SSE notification bridge using Redis Pub/Sub.
// Prefer this over StartPgNotifyBridge when Redis is available: Redis
// reconnections are transparent (< 100 ms) vs the 1 s–30 s backoff of the
// pg_notify LISTEN loop, and no long-lived database connection is consumed.
//
// Uses the same stopBridge/bridgeDone lifecycle as StartPgNotifyBridge;
// StopPgNotifyBridge stops this bridge as well.
func (s *Server) StartRedisBridge(ctx context.Context, rc redis.UniversalClient) {
	bridgeCtx, bridgeCancel := context.WithCancel(context.WithoutCancel(ctx))
	s.stopBridge = bridgeCancel
	done := make(chan struct{})
	s.bridgeDone = done
	go func() {
		defer close(done)
		s.runRedisBridge(bridgeCtx, rc)
	}()
}

// New constructs a Server with the provided dependencies.
//
// bus must not be nil; pass messaging.NewInMemoryBus() for local development
// or a *messaging.RedisBus connected to the production Redis instance.
// db may be nil when the database is unreachable at startup time; see the
// field comment on Server.db for the expected handler behaviour in that case.
// cacheStore may be nil when Redis is not configured; in that case all cached
// service decorators are bypassed and every request hits the database directly.
// checkers is the list of health checkers executed by GET /health/ready; pass
// nil (or an empty slice) when no dependency checks are needed (e.g. tests).
func New(db *pgxpool.Pool, cfg *config.Config, log *zap.Logger, bus events.Bus, cacheStore cache.Store, checkers []health.Checker) *Server {
	return &Server{db: db, cfg: cfg, log: log, bus: bus, cache: cacheStore, checkers: checkers}
}

// handleReadiness is a thin wrapper around health.ReadinessHandler that exists
// solely to carry the OpenAPI annotations swaggo needs to document the
// /health/ready endpoint. All logic lives in health.ReadinessHandler.
//
// @Summary      Readiness check
// @Description  Readiness probe: runs all registered infrastructure checkers
//
//	and returns a detailed JSON report. Returns 503 if any check fails.
//
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  health.Response
// @Failure      503  {object}  health.Response
// @Router       /health/ready [get]
func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	health.ReadinessHandler(s.checkers)(w, r)
}

// handleHealth responds to liveness probes issued by load balancers and
// container orchestration platforms.
//
// This handler intentionally does not check database connectivity. A liveness
// probe answers the question "is this process alive and able to handle
// requests?", not "are all its dependencies healthy?". Reporting unhealthy
// when the database is temporarily unavailable would cause the orchestrator
// to restart the pod, which would not fix the database and would instead
// discard all in-flight requests. Readiness probes (a separate concern) are
// the appropriate mechanism for gating traffic on dependency availability.
//
// @Summary      Health check
// @Description  Liveness probe for load balancers and container orchestrators.
//
//	Does not verify database connectivity by design.
//
// @Tags         infrastructure
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /health [get]
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"world-cup-quiniela"}`)
}
