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
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
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
	// limiterStore overrides the default per-user rate limiter when non-nil.
	// Typically set in tests via SetLimiterStore to bypass throttling when
	// exercising the full middleware chain with many requests for the same user.
	limiterStore *middleware.LimiterStore
	// notifHub is the in-process SSE hub; created once in Routes() and reused
	// by the notification handler and the pg_notify bridge goroutine.
	notifHub *hub.Hub
	// stopBridge cancels the pg_notify bridge goroutine context; nil until Routes() is called.
	stopBridge context.CancelFunc
	// bridgeDone is closed when the pg_notify bridge goroutine exits.
	bridgeDone <-chan struct{}
}

// SetDLQService wires an optional DLQService for the admin /dlq endpoints.
// Call this after New() when the Redis event bus driver is active.
func (s *Server) SetDLQService(dlq service.DLQService) { s.dlqSvc = dlq }

// SetLimiterStore overrides the per-user rate limiter constructed by Routes().
// Intended for tests that need to exercise the full middleware chain for many
// requests with the same user ID without triggering 429 responses; pass
// middleware.NewUnlimitedLimiterStore() to disable rate limiting for the test.
func (s *Server) SetLimiterStore(store *middleware.LimiterStore) { s.limiterStore = store }

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
func (s *Server) StartPgNotifyBridge() {
	bridgeCtx, bridgeCancel := context.WithCancel(context.Background())
	s.stopBridge = bridgeCancel
	done := make(chan struct{})
	s.bridgeDone = done
	go func() {
		defer close(done)
		s.runPgNotifyBridge(bridgeCtx)
	}()
}

// StopPgNotifyBridge cancels the pg_notify bridge goroutine and waits for it
// to exit. Must be called before closing the database pool so the bridge
// releases its acquired connection before pool.Close() calls WG.Wait().
// Safe to call if StartPgNotifyBridge was never invoked (no-op in that case).
func (s *Server) StopPgNotifyBridge() {
	if s.stopBridge != nil {
		s.stopBridge()
	}
	if s.bridgeDone != nil {
		<-s.bridgeDone
	}
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
