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
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"go.uber.org/zap"
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
}

// New constructs a Server with the provided dependencies.
//
// db may be nil when the database is unreachable at startup time; see the
// field comment on Server.db for the expected handler behaviour in that case.
func New(db *pgxpool.Pool, cfg *config.Config, log *zap.Logger) *Server {
	return &Server{db: db, cfg: cfg, log: log}
}

// Routes returns the fully configured http.Handler for the application.
//
// The routing table is arranged in two tiers:
//
//  1. Infrastructure endpoints (/health) are mounted at the root without a
//     version prefix. They are consumed by load balancers and monitoring
//     systems, not by API clients, and must not be versioned — a load
//     balancer should not need to know which API version it is probing.
//
//  2. Business endpoints are mounted under /api/v1 via a sub-router. The
//     sub-router is the correct place to attach authentication middleware so
//     that it applies to all business routes without touching /health.
//
// Middleware is applied in declaration order. Each middleware wraps all
// middleware declared after it, so RequestID must be declared first to ensure
// its value is available to every subsequent handler and middleware.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// Global middleware — applied to every request including /health.
	// Order matters: RequestID must be first so its value is available to
	// every subsequent middleware (logging, recovery) via GetRequestID.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Recover(s.log))
	r.Use(middleware.RequestLogger(s.log))
	r.Use(middleware.CORS(s.cfg.CORS.AllowedOrigins))

	// Infrastructure endpoints — not versioned, no authentication required.
	r.Get("/health", s.handleHealth)

	// Versioned API surface. Authentication is enforced here so that /health
	// remains publicly accessible to load balancers and monitoring systems.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(s.cfg.Clerk.JWKSURL, s.log))
		// TODO: mount route groups as handlers are implemented.
		// Example:
		//   r.Mount("/matches",     matchHandler.New(s.db, s.log).Routes())
		//   r.Mount("/predictions", predictionHandler.New(s.db, s.log).Routes())
	})

	return r
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
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"world-cup-quiniela"}`)
}
