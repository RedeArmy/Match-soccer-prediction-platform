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

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"

	_ "github.com/rede/world-cup-quiniela/docs" // registers the Swagger spec at init time
	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/config"
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
// The routing table is arranged in three tiers:
//
//  1. Infrastructure endpoints (/health, /swagger) are mounted at the root.
//     They are consumed by load balancers and monitoring systems and must
//     not be versioned or gated behind authentication.
//
//  2. Webhook endpoints (/webhooks/clerk) are mounted at the root without
//     JWT authentication. They are authenticated via Svix HMAC-SHA256
//     signature validation instead.
//
//  3. Business endpoints are mounted under /api/v1. The sub-router is the
//     correct place to attach RequireAuth so it applies to all business
//     routes without touching /health or /webhooks.
//
// Middleware is applied in declaration order. RequestID must be declared
// first so its value is available to every subsequent handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// Global middleware — applied to every request.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Recover(s.log))
	r.Use(middleware.RequestLogger(s.log))
	r.Use(middleware.CORS(s.cfg.CORS.AllowedOrigins))

	// Infrastructure endpoints — not versioned, no authentication required.
	r.Get("/health", s.handleHealth)
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
	))

	if s.db == nil {
		// When the database is unavailable, register the known routes with
		// a stub that returns 503 so that registered paths return 503 and
		// unregistered paths still return 404 (chi's default).
		dbUnavailable := func(w http.ResponseWriter, req *http.Request) {
			middleware.WriteError(w, req, s.log, apperrors.Internal(fmt.Errorf("database unavailable")))
		}
		r.Route("/api/v1", func(r chi.Router) {
			r.Use(middleware.RequireAuth(s.cfg.Clerk.JWKSURL, s.log))
			r.Route("/matches", func(r chi.Router) {
				r.HandleFunc("/*", dbUnavailable)
				r.HandleFunc("/", dbUnavailable)
			})
			r.Route("/predictions", func(r chi.Router) {
				r.HandleFunc("/*", dbUnavailable)
				r.HandleFunc("/", dbUnavailable)
			})
		})
		return r
	}

	// Construct repository instances once and share them across the event bus,
	// webhook handler, and API handler layers.
	userRepo := repository.NewPostgresUserRepository(s.db)
	matchRepo := repository.NewPostgresMatchRepository(s.db)
	predRepo := repository.NewPostgresPredictionRepository(s.db)
	stadiumRepo := repository.NewPostgresStadiumRepository(s.db)

	bus := s.buildBus(matchRepo, predRepo)
	matchHandler, predHandler := s.buildHandlers(bus, userRepo, matchRepo, predRepo, stadiumRepo)

	// Webhook endpoint — authenticated via Svix signature, not Clerk JWT.
	// Must be registered before the /api/v1 subrouter so it receives no auth middleware.
	webhookHandler := handler.NewWebhookHandler(userRepo, s.cfg.Clerk.WebhookSecret, s.log)
	r.Post("/webhooks/clerk", webhookHandler.HandleClerkWebhook)

	// Versioned API surface with Clerk JWT authentication.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(s.cfg.Clerk.JWKSURL, s.log))

		// Admin-only match mutations are guarded by RequireRole. Read endpoints
		// (List, Get) are accessible to all authenticated users.
		r.Route("/matches", func(r chi.Router) {
			r.Get("/", matchHandler.ListMatches)
			r.Get("/{id}", matchHandler.GetMatch)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Post("/", matchHandler.CreateMatch)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Patch("/{id}", matchHandler.UpdateResult)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Post("/{id}/start", matchHandler.StartMatch)
		})

		r.Route("/predictions", func(r chi.Router) {
			r.Post("/", predHandler.Submit)
			r.Get("/", predHandler.ListByUser)
			r.Patch("/{id}", predHandler.Update)
		})
	})

	return r
}

// buildBus creates the in-memory event bus and wires up the scoring subscriber.
// The provided repositories are reused from the caller's shared instances rather
// than constructed anew, preventing duplicate connection-pool handles.
func (s *Server) buildBus(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
) events.Bus {
	bus := messaging.NewInMemoryBus()
	scorer := service.NewScoringService(matchRepo, predRepo, s.log)

	// Subscribe ScoringService to MatchFinished so scores are calculated
	// automatically whenever a match result is confirmed.
	bus.Subscribe(events.EventMatchFinished, func(ctx context.Context, env events.Envelope) {
		mf, ok := env.Payload.(events.MatchFinished)
		if !ok {
			return
		}
		if err := scorer.ScoreMatch(ctx, mf.MatchID); err != nil {
			s.log.Error("scoring failed after MatchFinished event",
				zap.Int("match_id", mf.MatchID),
				zap.Error(err),
			)
		}
	})
	return bus
}

// buildHandlers constructs the service layer and returns the route handlers.
// The provided repositories are reused from the caller's shared instances.
func (s *Server) buildHandlers(
	bus events.Bus,
	userRepo repository.UserRepository,
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	_ repository.StadiumRepository, // reserved for future stadium endpoints
) (*handler.MatchHandler, *handler.PredictionHandler) {
	matchSvc := service.NewMatchService(matchRepo, bus, s.log)
	predSvc := service.NewPredictionService(predRepo, matchRepo, bus, s.log)

	return handler.NewMatchHandler(matchSvc, s.log),
		handler.NewPredictionHandler(predSvc, userRepo, s.log)
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
