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
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"

	_ "github.com/rede/world-cup-quiniela/docs" // registers the Swagger spec at init time
	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

const (
	routePredictions = "/predictions"
	routeUsers       = "/users"
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
	// bus is the event bus used to publish and receive domain events.
	// The implementation is selected at startup via WCQ_EVENTBUS_DRIVER:
	//   "in_memory" — InMemoryBus, safe for single-replica / local dev.
	//   "redis"     — RedisBus, required for multi-replica production deployments.
	bus events.Bus
	// cache is the optional key-value store used to cache list responses and
	// leaderboard results. When nil (e.g. Redis is not configured), caching is
	// disabled and every request hits the database directly.
	cache    cache.Store
	checkers []health.Checker
	// dlqSvc is nil when EventBus.Driver != "redis". Admin DLQ endpoints
	// delegate to service.NoopDLQService when this field is nil.
	dlqSvc service.DLQService
}

// SetDLQService wires an optional DLQService for the admin /dlq endpoints.
// Call this after New() when the Redis event bus driver is active.
func (s *Server) SetDLQService(dlq service.DLQService) { s.dlqSvc = dlq }

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
	r.Get("/health/ready", s.handleReadiness)
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
			r.Use(middleware.RequestBodyLimit(64 * 1024)) // 64 KB — all API payloads are small JSON objects
			r.Use(middleware.RequireAuth(s.cfg.Clerk.JWKSURL, middleware.DefaultJWKSWarmupTimeout, s.log))
			r.Route("/matches", func(r chi.Router) {
				r.HandleFunc("/*", dbUnavailable)
				r.HandleFunc("/", dbUnavailable)
			})
			r.Route(routePredictions, func(r chi.Router) {
				r.HandleFunc("/*", dbUnavailable)
				r.HandleFunc("/", dbUnavailable)
			})
			r.Route("/groups", func(r chi.Router) {
				r.HandleFunc("/*", dbUnavailable)
				r.HandleFunc("/", dbUnavailable)
			})
			r.Route(routeUsers, func(r chi.Router) {
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
	memberRepo := repository.NewPostgresGroupMembershipRepository(s.db)
	systemParamRepo := repository.NewPostgresSystemParamRepository(s.db)

	paramSvc := service.NewSystemParamService(systemParamRepo, s.log)

	// Read infrastructure params once at startup. Changes require a process
	// restart (is_runtime=FALSE in system_params). Use context.Background()
	// because this is a one-time read at construction time, not a request path.
	infraCtx := context.Background()
	messaging.Configure(
		paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingMaxRetries, 3),
		int64(paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingStreamMaxLen, 600_000)),
		nil, // retain default RetryBackoff (1s, 2s); no array param defined
	)
	authWarmup := time.Duration(paramSvc.GetInt(infraCtx, domain.ParamKeyAuthValidationTimeout, 5)) * time.Second

	s.wireSubscribers(matchRepo, predRepo, paramSvc)
	matchHandler, predHandler, groupHandler, leaderboardHandler, userStatsHandler, tiebreakerHandler, tournamentHandler, adminUserH, adminGroupH, adminPaymentH, adminLeaderboardH, adminDLQH, adminAuditH, adminParamH, adminTiebreakerH, adminConflictH := s.buildHandlers(userRepo, matchRepo, predRepo, memberRepo, paramSvc)

	// Webhook endpoint — authenticated via Svix signature, not Clerk JWT.
	// Must be registered before the /api/v1 subrouter so it receives no auth middleware.
	webhookHandler := handler.NewWebhookHandler(userRepo, s.cfg.Clerk.WebhookSecret, s.log)
	r.Post("/webhooks/clerk", webhookHandler.HandleClerkWebhook)

	// Versioned API surface with Clerk JWT authentication.
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequestBodyLimit(64 * 1024)) // 64 KB — all API payloads are small JSON objects
		r.Use(middleware.RequireAuth(s.cfg.Clerk.JWKSURL, authWarmup, s.log))

		// Admin-only match mutations are guarded by RequireRole. Read endpoints
		// (List, Get) are accessible to all authenticated users.
		r.Route("/matches", func(r chi.Router) {
			r.Get("/", matchHandler.ListMatches)
			r.Get("/{id}", matchHandler.GetMatch)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Post("/", matchHandler.CreateMatch)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Patch("/{id}", matchHandler.UpdateResult)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Post("/{id}/start", matchHandler.StartMatch)
		})

		// ResolveUser is applied at the subrouter level so all prediction and
		// group endpoints can read the caller's domain.User from context without
		// each handler querying the database independently. GetByID and
		// ListMembers do not use the caller's identity but the cost of a single
		// indexed lookup (clerk_subject) is negligible compared to the handler
		// work that follows.
		r.Route(routePredictions, func(r chi.Router) {
			r.Use(middleware.ResolveUser(userRepo, s.log))
			r.Post("/", predHandler.Submit)
			r.Get("/", predHandler.ListByUser)
			r.Patch("/{id}", predHandler.Update)
		})

		r.Route("/groups", func(r chi.Router) {
			r.Use(middleware.ResolveUser(userRepo, s.log))
			r.Post("/", groupHandler.Create)
			r.Post("/join", groupHandler.Join)
			r.Get("/me", groupHandler.ListMyGroups)
			r.Get("/{id}", groupHandler.GetByID)
			// Only the CreateOwner (MembershipRoleCreateOwner) may rename the group.
			// Ownership is enforced inside the service layer — not via RequireRole
			// because it is resource-scoped, not system-role-scoped.
			r.Patch("/{id}", groupHandler.RenameGroup)
			r.Get("/{id}/members", groupHandler.ListMembers)
			r.Get("/{id}/leaderboard", leaderboardHandler.GetLeaderboard)
			// Any active member may approve a pending join request. The service
			// layer enforces the membership check — no role-based middleware needed.
			r.Post("/{id}/members/{membershipID}/approve", groupHandler.ApproveJoin)
			// Self-removal only: a user removes themselves from the group.
			r.Delete("/{id}/members/me", groupHandler.Leave)
			// Tiebreaker member routes: active members submit and view their prediction.
			r.Post("/{id}/tiebreaker", tiebreakerHandler.Submit)
			r.Get("/{id}/tiebreaker", tiebreakerHandler.GetMine)
		})

		// Tiebreaker admin routes: only the system administrator may set the
		// global question and confirm the result. RequireRole enforces this gate.
		r.Route("/tiebreaker", func(r chi.Router) {
			r.Use(middleware.ResolveUser(userRepo, s.log))
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Patch("/question", tiebreakerHandler.SetQuestion)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Patch("/result", tiebreakerHandler.ConfirmResult)
		})

		// Tournament: real-time standings (all authenticated users) and bracket
		// slot management (admin only).
		r.Route("/tournament", func(r chi.Router) {
			r.Use(middleware.ResolveUser(userRepo, s.log))
			r.Get("/standings", tournamentHandler.GetAllStandings)
			r.Get("/standings/{group}", tournamentHandler.GetGroupStanding)
			r.Get("/slots", tournamentHandler.ListSlots)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Post("/slots", tournamentHandler.CreateSlot)
			r.With(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin)).Patch("/slots/{id}", tournamentHandler.ConfirmSlot)
		})

		r.Route(routeUsers, func(r chi.Router) {
			r.Use(middleware.ResolveUser(userRepo, s.log))
			r.Get("/me/stats", userStatsHandler.GetMyStats)
		})

		// Admin panel — all routes require RoleAdmin. ResolveUser is applied so
		// handlers can read the caller's domain.User (for audit trail adminID).
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequireRole(userRepo, s.log, domain.RoleAdmin))
			r.Use(middleware.ResolveUser(userRepo, s.log))

			// Users
			r.Get(routeUsers, adminUserH.ListUsers)
			r.Get("/users/{id}", adminUserH.GetUserProfile)
			r.Post("/users/{id}/ban", adminUserH.BanUser)
			r.Delete("/users/{id}/ban", adminUserH.UnbanUser)
			r.Post("/users/bulk-ban", adminUserH.BulkBan)

			// Groups
			r.Delete("/groups/{id}", adminGroupH.DeleteGroup)
			r.Delete("/groups/{id}/members/{membershipID}", adminGroupH.RemoveMember)
			r.Patch("/groups/{id}/settings", adminGroupH.UpdateGroupSettings)
			r.Post("/groups/{id}/transfer-ownership", adminGroupH.TransferOwnership)
			r.Get("/groups/{id}/payments", adminPaymentH.ListByGroup)
			r.Get("/groups/{id}/leaderboard/history", adminLeaderboardH.SnapshotHistory)

			// Payments
			r.Get("/payments/pending", adminPaymentH.ListPending)
			r.Get("/payments", adminPaymentH.List)
			r.Post("/payments/{id}/validate", adminPaymentH.ValidateDeposit)
			r.Post("/payments/{id}/reject", adminPaymentH.RejectDeposit)

			// Leaderboard & Predictions
			r.Get("/leaderboard", adminLeaderboardH.GlobalLeaderboard)
			r.Get(routePredictions, adminLeaderboardH.ListPredictions)
			r.Get("/predictions/match/{matchID}", adminLeaderboardH.ListPredictionsByMatch)

			// DLQ
			r.Get("/dlq", adminDLQH.Stats)
			r.Post("/dlq/replay", adminDLQH.Replay)
			r.Delete("/dlq", adminDLQH.Purge)

			// Audit log
			r.Get("/audit-log", adminAuditH.List)
			r.Get("/audit-log/entity/{type}/{id}", adminAuditH.ListByEntity)

			// System parameters
			r.Get("/system-params", adminParamH.ListAll)
			r.Get("/system-params/{key}", adminParamH.Get)
			r.Patch("/system-params/{key}", adminParamH.Set)
			r.Post("/system-params/bulk", adminParamH.BulkSet)

			// Tiebreakers
			r.Get("/tiebreaker/submissions", adminTiebreakerH.ListSubmissions)

			// Conflicts
			r.Get("/conflicts", adminConflictH.ListConflicts)
			r.Post("/conflicts/{type}/{id}/resolve", adminConflictH.ResolveConflict)

			// Stats / observability
			r.Get("/stats/conflicts/summary", adminConflictH.ConflictSummary)
		})
	})

	return r
}

// wireSubscribers registers all domain event handlers on s.bus.
//
// This method is intentionally separate from bus construction: the bus
// implementation (InMemory vs Redis) is selected at the composition root in
// main.go and injected via New(). wireSubscribers only adds subscribers,
// keeping routing logic in one place and bus lifecycle management in another.
//
// With the Redis driver the worker process owns all event processing
// exclusively. Registering a scoring subscriber here as well would place
// both the API server and the worker in the same consumer group, causing
// every MatchFinished event to be scored twice. When driver=redis this
// method is therefore a no-op: the API server only publishes events; it
// never consumes them.
func (s *Server) wireSubscribers(
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	params service.SystemParamService,
) {
	if s.cfg.EventBus.Driver == "redis" {
		return
	}

	scorer := service.NewScoringService(matchRepo, predRepo, params, s.log)

	// MatchFinished → ScoringService: calculate points for every prediction
	// on the finished match. The handler runs inside a fresh background context
	// so a cancelled HTTP request context does not abort the scoring job.
	s.bus.Subscribe(events.EventMatchFinished, func(ctx context.Context, env events.Envelope) error {
		mf, ok := env.Payload.(events.MatchFinished)
		if !ok {
			// Malformed payload: retrying will not help, so return nil to
			// prevent the bus from routing this to the dead-letter queue.
			s.log.Error("scoring: unexpected payload type for MatchFinished event",
				zap.String("event_type", string(env.Type)),
			)
			return nil
		}
		if err := scorer.ScoreMatch(ctx, mf.MatchID); err != nil {
			s.log.Error("scoring failed after MatchFinished event",
				zap.Int("match_id", mf.MatchID),
				zap.Error(err),
			)
			// Return the error so the bus can retry and, if all attempts
			// fail, push the event to the dead-letter queue for manual replay.
			return err
		}
		return nil
	})
}

// buildHandlers constructs the service layer (with optional cache decorators)
// and returns the route handlers. The provided repositories are reused from
// the caller's shared instances. s.bus is passed to services so they can
// publish domain events. When s.cache is non-nil, list-heavy services are
// wrapped with read-through / write-invalidation cache decorators.
func (s *Server) buildHandlers(
	userRepo repository.UserRepository,
	matchRepo repository.MatchRepository,
	predRepo repository.PredictionRepository,
	memberRepo repository.GroupMembershipRepository,
	params service.SystemParamService,
) (
	*handler.MatchHandler,
	*handler.PredictionHandler,
	*handler.GroupHandler,
	*handler.LeaderboardHandler,
	*handler.UserStatsHandler,
	*handler.TiebreakerHandler,
	*handler.TournamentHandler,
	*handler.AdminUserHandler,
	*handler.AdminGroupHandler,
	*handler.AdminPaymentHandler,
	*handler.AdminLeaderboardHandler,
	*handler.AdminDLQHandler,
	*handler.AdminAuditHandler,
	*handler.AdminSystemParamHandler,
	*handler.AdminTiebreakerHandler,
	*handler.AdminConflictHandler,
) {
	quinielaRepo := repository.NewPostgresQuinielaRepository(s.db)
	tiebreakerRepo := repository.NewPostgresTiebreakerRepository(s.db)
	tiebreakerConfigRepo := repository.NewPostgresTiebreakerConfigRepository(s.db)
	tournamentRepo := repository.NewPostgresTournamentRepository(s.db)
	auditLogRepo := repository.NewPostgresAuditLogRepository(s.db)
	paymentRepo := repository.NewPostgresPaymentRecordRepository(s.db)
	snapRepo := repository.NewPostgresLeaderboardSnapshotRepository(s.db)

	// Read infrastructure params (startup-time, not per-request).
	infraCtx := context.Background()
	auditTimeout := time.Duration(params.GetInt(infraCtx, domain.ParamKeyAuditWriteTimeout, 5)) * time.Second
	matchTTL := time.Duration(params.GetInt(infraCtx, domain.ParamKeyCacheMatchTTL, 300)) * time.Second
	leaderboardTTL := time.Duration(params.GetInt(infraCtx, domain.ParamKeyCacheLeaderboardTTL, 60)) * time.Second
	dlqSampleSize := params.GetInt(infraCtx, domain.ParamKeyDLQSampleSize, 5)
	dlqReplayLimit := params.GetInt(infraCtx, domain.ParamKeyDLQReplayDefaultLimit, 10)

	auditSvc := service.NewAuditService(auditLogRepo, auditTimeout, s.log)

	scorer := service.NewScoringService(matchRepo, predRepo, params, s.log)
	matchSvc := service.NewMatchService(matchRepo, s.bus, scorer, auditSvc, s.log)
	if s.cache != nil {
		matchSvc = service.NewCachedMatchService(matchSvc, s.cache, matchTTL, s.log)
	}

	predSvc := service.NewPredictionService(predRepo, matchRepo, params, s.log)
	quinielaSvc := service.NewQuinielaService(quinielaRepo, memberRepo, params)
	paymentSvc := service.NewPaymentService(paymentRepo, auditSvc, s.log)
	memberSvc := service.NewGroupMembershipService(quinielaRepo, memberRepo, params, auditSvc, paymentSvc, s.log)

	ranker := service.NewRankingService(quinielaRepo, predRepo, userRepo, tiebreakerRepo, tiebreakerConfigRepo, params, s.log)
	if s.cache != nil {
		ranker = service.NewCachedRankingService(ranker, s.cache, leaderboardTTL, s.log)
	}

	userStatsSvc := service.NewUserStatsService(predRepo)
	tiebreakerSvc := service.NewTiebreakerService(tiebreakerConfigRepo, memberRepo, tiebreakerRepo, auditSvc, s.log)
	tournamentSvc := service.NewTournamentService(matchRepo, tournamentRepo, params, auditSvc, s.log)
	adminGroupSvc := service.NewAdminGroupService(quinielaRepo, memberRepo, auditSvc, s.log)
	adminUserSvc := service.NewAdminUserService(userRepo, memberRepo, paymentRepo, auditSvc, s.log)
	adminReadSvc := service.NewAdminReadService(predRepo, userRepo, tiebreakerRepo, snapRepo, s.log)
	conflictSvc := service.NewConflictService(quinielaRepo, memberRepo, paymentRepo, params, auditSvc, s.log)

	dlqSvc := s.dlqSvc
	if dlqSvc == nil {
		dlqSvc = service.NoopDLQService{}
	}
	_ = dlqSampleSize // consumed by NewRedisDLQService when wired externally via SetDLQService
	_ = dlqReplayLimit

	return handler.NewMatchHandler(matchSvc, s.log),
		handler.NewPredictionHandler(predSvc, s.log),
		handler.NewGroupHandler(quinielaSvc, memberSvc, s.log),
		handler.NewLeaderboardHandler(ranker, s.log),
		handler.NewUserStatsHandler(userStatsSvc, s.log),
		handler.NewTiebreakerHandler(tiebreakerSvc, s.log),
		handler.NewTournamentHandler(tournamentSvc, s.log),
		handler.NewAdminUserHandler(adminUserSvc, s.log),
		handler.NewAdminGroupHandler(adminGroupSvc, s.log),
		handler.NewAdminPaymentHandler(paymentSvc, s.log),
		handler.NewAdminLeaderboardHandler(adminReadSvc, params, s.log),
		handler.NewAdminDLQHandler(dlqSvc, s.log),
		handler.NewAdminAuditHandler(auditSvc, s.log),
		handler.NewAdminSystemParamHandler(params, s.log),
		handler.NewAdminTiebreakerHandler(adminReadSvc, s.log),
		handler.NewAdminConflictHandler(conflictSvc, s.log)
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
