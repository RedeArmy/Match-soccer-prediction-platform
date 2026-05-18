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
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/auth"
	"github.com/rede/world-cup-quiniela/pkg/clock"
	"github.com/rede/world-cup-quiniela/pkg/codegen"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/health"
)

const (
	routePredictions = "/predictions"
	routeUsers       = "/users"
)

// coreRepos bundles the five shared repository instances constructed once in
// Routes and forwarded to buildHandlers. Grouping them reduces the parameter
// count and makes future additions a single-field change.
type coreRepos struct {
	user     repository.UserRepository
	match    repository.MatchRepository
	pred     repository.PredictionRepository
	member   repository.GroupMembershipRepository
	sysParam repository.SystemParamRepository
}

// appHandlers groups all route handlers; fields are unexported and used only within Routes.
type appHandlers struct {
	match             *handler.MatchHandler
	prediction        *handler.PredictionHandler
	group             *handler.GroupHandler
	leaderboard       *handler.LeaderboardHandler
	userStats         *handler.UserStatsHandler
	tiebreaker        *handler.TiebreakerHandler
	tournament        *handler.TournamentHandler
	balance           *handler.BalanceHandler
	bankTransfer      *handler.BankTransferHandler
	withdrawal        *handler.WithdrawalHandler
	paymentIntent     *handler.PaymentIntentHandler
	paymentWebhook    *handler.PaymentWebhookHandler
	adminUser         *handler.AdminUserHandler
	adminGroup        *handler.AdminGroupHandler
	adminPayment      *handler.AdminPaymentHandler
	adminLeaderboard  *handler.AdminLeaderboardHandler
	adminDLQ          *handler.AdminDLQHandler
	adminAudit        *handler.AdminAuditHandler
	adminParam        *handler.AdminSystemParamHandler
	adminTiebreaker   *handler.AdminTiebreakerHandler
	adminConflict     *handler.AdminConflictHandler
	adminStats        *handler.AdminStatsHandler
	adminScoringRules *handler.AdminScoringRuleHandler
}

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
//  2. Webhook endpoints are mounted at the root without JWT authentication.
//     Each provider authenticates via its own signature scheme:
//     Clerk uses Svix HMAC-SHA256; Recurrente uses HMAC-SHA256; PayPal uses
//     RSA certificate-based verification. See middleware/webhook_*.go.
//
//  3. Business endpoints are mounted under /api/v1. The sub-router is the
//     correct place to attach RequireAuth so it applies to all business
//     routes without touching /health or /webhooks.
//
// Middleware is applied in declaration order. RequestID must be declared
// first so its value is available to every subsequent handler.
func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()

	// Global middleware - applied to every request.
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(middleware.Recover(s.log))
	r.Use(middleware.RequestLogger(s.log))
	r.Use(middleware.CORS(s.cfg.CORS.AllowedOrigins))

	// Infrastructure endpoints - not versioned, no authentication required.
	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleReadiness)
	r.Get("/swagger/*", httpSwagger.Handler(
		httpSwagger.URL("/swagger/doc.json"),
		httpSwagger.DeepLinking(true),
	))

	if s.db == nil {
		// When the database is unavailable, register the entire API surface with
		// two catch-all stubs that return 503. Wildcard coverage means new routes
		// added to the happy path are automatically covered without a second edit
		// here. Infrastructure endpoints (/health, /swagger) remain reachable
		// because they are registered above and are not part of /api/v1.
		r.Route("/api/v1", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(domain.DefaultAPIBodySizeLimitBytes))
			r.Use(middleware.RequireAuth(auth.NewJWKSProvider(s.cfg.Clerk.JWKSURL, auth.DefaultWarmupTimeout, s.log), s.log))
			dbUnavailable := func(w http.ResponseWriter, req *http.Request) {
				middleware.WriteError(w, req, s.log, apperrors.Internal(fmt.Errorf("database unavailable")))
			}
			r.HandleFunc("/*", dbUnavailable)
			r.HandleFunc("/", dbUnavailable)
		})
		return r
	}

	// Construct repository instances once and share them across the event bus,
	// webhook handler, and API handler layers.
	repos := coreRepos{
		user:     repository.NewPostgresUserRepository(s.db),
		match:    repository.NewPostgresMatchRepository(s.db),
		pred:     repository.NewPostgresPredictionRepository(s.db),
		member:   repository.NewPostgresGroupMembershipRepository(s.db),
		sysParam: repository.NewPostgresSystemParamRepository(s.db),
	}

	paramSvc := service.NewSystemParamService(repos.sysParam, nil, s.log)

	// Read infrastructure params once at startup. Changes require a process
	// restart (is_runtime=FALSE in system_params). Use context.Background()
	// because this is a one-time read at construction time, not a request path.
	infraCtx := context.Background()
	messaging.Configure(
		paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingMaxRetries, domain.DefaultMessagingMaxRetries),
		int64(paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingStreamMaxLen, domain.DefaultMessagingStreamMaxLen)),
		paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingStreamWorkerCount, domain.DefaultMessagingStreamWorkerCount),
		paramSvc.GetInt(infraCtx, domain.ParamKeyMessagingStreamReadBlockSec, domain.DefaultMessagingStreamReadBlockSec),
		nil, // retain default RetryBackoff (1s, 2s); no array param defined
	)
	service.ConfigureAuditRetry(
		paramSvc.GetInt(infraCtx, domain.ParamKeyAuditMaxRetries, domain.DefaultAuditMaxRetries),
		paramSvc.GetInt(infraCtx, domain.ParamKeyAuditRetryDelayMs, domain.DefaultAuditRetryDelayMs),
	)
	bodySizeLimit := int64(paramSvc.GetInt(infraCtx, domain.ParamKeyAPIBodySizeLimitBytes, domain.DefaultAPIBodySizeLimitBytes))
	uploadSizeLimit := int64(paramSvc.GetInt(infraCtx, domain.ParamKeyPaymentMaxUploadBytes, domain.DefaultPaymentMaxUploadBytes))
	authWarmup := time.Duration(paramSvc.GetInt(infraCtx, domain.ParamKeyAuthValidationTimeout, domain.DefaultAuthValidationTimeoutSeconds)) * time.Second

	// scorer is constructed once and shared: local event subscribers and the
	// match service both use the same stateless scoring logic. With the redis
	// driver, the worker process owns all event consumption; the API server
	// only publishes, so local subscription is skipped.
	ruleRepo := repository.NewPostgresScoringRuleRepository(s.db)
	scorer := service.NewScoringService(repos.match, repos.pred, ruleRepo, paramSvc, s.log)
	if s.cfg.EventBus.Driver != "redis" {
		s.registerLocalSubscribers(scorer)
	}
	h := s.buildHandlers(infraCtx, repos, paramSvc, scorer)

	// Webhook endpoints — authenticated via provider-specific signatures, not Clerk JWT.
	// Must be registered before the /api/v1 subrouter so they receive no auth middleware.
	// Each provider uses its own signature scheme:
	//   clerk:      Svix HMAC-SHA256 (verified inside WebhookHandler)
	//   recurrente: HMAC-SHA256 via RecurrenteWebhookAuth middleware
	//   paypal:     RSA certificate verification via PayPalWebhookAuth middleware
	clerkSyncer := service.NewClerkUserSyncService(repos.user, s.log)
	webhookHandler := handler.NewWebhookHandler(clerkSyncer, s.cfg.Clerk.WebhookSecret, s.log)
	r.Post("/webhooks/clerk", webhookHandler.HandleClerkWebhook)
	r.With(middleware.RecurrenteWebhookAuth(s.cfg.Payment.RecurrenteWebhookSecret, s.log)).
		Post("/webhooks/recurrente", h.paymentWebhook.HandleRecurrente)
	r.With(middleware.PayPalWebhookAuth(s.cfg.Payment.PayPalWebhookID, middleware.DefaultPayPalCertFetcher(), s.log)).
		Post("/webhooks/paypal", h.paymentWebhook.HandlePayPal)

	// Versioned API surface with Clerk JWT authentication.
	// Rate limit params are read once at startup (is_runtime=FALSE); a process
	// restart is required to change the rate or burst.
	clerkProvider := auth.NewJWKSProvider(s.cfg.Clerk.JWKSURL, authWarmup, s.log)
	userRateStore := s.limiterStore
	if userRateStore == nil {
		userRateStore = middleware.NewLimiterStore(
			float64(paramSvc.GetInt(infraCtx, domain.ParamKeyAPIRateLimitRatePerSec, domain.DefaultAPIRateLimitRatePerSec)),
			paramSvc.GetInt(infraCtx, domain.ParamKeyAPIRateLimitBurst, domain.DefaultAPIRateLimitBurst),
		)
	}
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(middleware.RequireAuth(clerkProvider, s.log))
		r.Use(middleware.RateLimitByUserID(userRateStore, s.log))

		// Admin-only match mutations are guarded by RequireRole. Read endpoints
		// (List, Get) are accessible to all authenticated users.
		r.Route("/matches", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Get("/", h.match.ListMatches)
			r.Get("/{id}", h.match.GetMatch)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Post("/", h.match.CreateMatch)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Patch("/{id}", h.match.UpdateResult)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Post("/{id}/start", h.match.StartMatch)
		})

		// ResolveUser is applied at the subrouter level so all prediction and
		// group endpoints can read the caller's domain.User from context without
		// each handler querying the database independently. GetByID and
		// ListMembers do not use the caller's identity but the cost of a single
		// indexed lookup (clerk_subject) is negligible compared to the handler
		// work that follows.
		r.Route(routePredictions, func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Post("/", h.prediction.Submit)
			r.Get("/me", h.prediction.GetMine)
			r.Patch("/{id}", h.prediction.Update)
		})

		r.Route("/groups", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Post("/", h.group.Create)
			r.Post("/join", h.group.Join)
			r.Post("/join-with-balance", h.group.JoinWithBalance)
			r.Get("/me", h.group.ListMyGroups)
			r.Get("/{id}", h.group.GetByID)
			// Only the CreateOwner (MembershipRoleCreateOwner) may rename the group.
			// Ownership is enforced inside the service layer - not via RequireRole
			// because it is resource-scoped, not system-role-scoped.
			r.Patch("/{id}", h.group.RenameGroup)
			r.Get("/{id}/members", h.group.ListMembers)
			r.Get("/{id}/leaderboard", h.leaderboard.GetLeaderboard)
			// Any active member may approve a pending join request. The service
			// layer enforces the membership check - no role-based middleware needed.
			r.Post("/{id}/members/{membershipID}/approve", h.group.ApproveJoin)
			// Self-removal only: a user removes themselves from the group.
			r.Delete("/{id}/members/me", h.group.Leave)
			// Tiebreaker member routes: active members submit and view their prediction.
			r.Post("/{id}/tiebreaker", h.tiebreaker.Submit)
			r.Get("/{id}/tiebreaker", h.tiebreaker.GetMine)
		})

		// Tiebreaker admin routes: only the system administrator may set the
		// global question and confirm the result. RequireRole enforces this gate
		// and stores the resolved user in context, so no separate ResolveUser
		// middleware is needed on this subrouter.
		r.Route("/tiebreaker", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Patch("/question", h.tiebreaker.SetQuestion)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Patch("/result", h.tiebreaker.ConfirmResult)
		})

		// Tournament: real-time standings (all authenticated users) and bracket
		// slot management (admin only). The GET endpoints do not require a
		// resolved domain.User. Admin mutations use RequireRole, which now stores
		// the resolved user in context so ConfirmSlot can read caller.ID without
		// an extra database query.
		r.Route("/tournament", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Get("/standings", h.tournament.GetAllStandings)
			r.Get("/standings/{group}", h.tournament.GetGroupStanding)
			r.Get("/slots", h.tournament.ListSlots)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Post("/slots", h.tournament.CreateSlot)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Patch("/slots/{id}", h.tournament.ConfirmSlot)
		})

		r.Route(routeUsers, func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/me/stats", h.userStats.GetMyStats)
			r.Get("/me/balance", h.balance.GetBalance)
			r.Get("/me/balance/ledger", h.balance.GetLedger)
		})

		r.Route("/payment-intents", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Post("/", h.paymentIntent.Create)
		})

		r.Route("/bank-transfers", func(r chi.Router) {
			r.Use(middleware.ResolveUser(repos.user, s.log))
			// POST receives a multipart upload (up to uploadSizeLimit); GET has no
			// body. Per-route limits avoid the MaxBytesReader stacking problem where
			// an outer smaller limit silently blocks the inner larger one.
			r.With(middleware.RequestBodyLimit(uploadSizeLimit)).Post("/", h.bankTransfer.Upload)
			r.With(middleware.RequestBodyLimit(bodySizeLimit)).Get("/", h.bankTransfer.ListMine)
		})

		r.Route("/withdrawals", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Post("/", h.withdrawal.Create)
			r.Get("/", h.withdrawal.ListMine)
		})

		// Admin panel - all routes require RoleAdmin. RequireRole now stores the
		// resolved domain.User in context after the role check, so handlers can
		// call UserFromContext (for audit trail adminID) without a second database
		// round-trip. No separate ResolveUser middleware is needed.
		r.Route("/admin", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin))

			// Users
			r.Get(routeUsers, h.adminUser.ListUsers)
			r.Get("/users/{id}", h.adminUser.GetUserProfile)
			r.Post("/users/{id}/ban", h.adminUser.BanUser)
			r.Delete("/users/{id}/ban", h.adminUser.UnbanUser)
			r.Post("/users/bulk-ban", h.adminUser.BulkBan)

			// Groups
			r.Delete("/groups/{id}", h.adminGroup.DeleteGroup)
			r.Delete("/groups/{id}/members/{membershipID}", h.adminGroup.RemoveMember)
			r.Patch("/groups/{id}/settings", h.adminGroup.UpdateGroupSettings)
			r.Post("/groups/{id}/transfer-ownership", h.adminGroup.TransferOwnership)
			r.Get("/groups/{id}/payments", h.adminPayment.ListByGroup)
			r.Get("/groups/{id}/leaderboard/history", h.adminLeaderboard.SnapshotHistory)

			// Payments
			r.Get("/payments/pending", h.adminPayment.ListPending)
			r.Get("/payments", h.adminPayment.List)
			r.Post("/payments/{id}/validate", h.adminPayment.ValidateDeposit)
			r.Post("/payments/{id}/reject", h.adminPayment.RejectDeposit)

			// Bank transfers
			r.Get("/bank-transfers/pending", h.bankTransfer.AdminListPending)
			r.Post("/bank-transfers/{id}/approve", h.bankTransfer.AdminApprove)
			r.Post("/bank-transfers/{id}/reject", h.bankTransfer.AdminReject)

			// Withdrawals
			r.Get("/withdrawals/pending", h.withdrawal.AdminListPending)
			r.Post("/withdrawals/{id}/approve", h.withdrawal.AdminApprove)
			r.Post("/withdrawals/{id}/reject", h.withdrawal.AdminReject)
			r.Post("/withdrawals/{id}/process", h.withdrawal.AdminProcess)

			// Leaderboard & Predictions
			r.Get("/leaderboard", h.adminLeaderboard.GlobalLeaderboard)
			r.Get(routePredictions, h.adminLeaderboard.ListPredictions)
			r.Get("/predictions/match/{matchID}", h.adminLeaderboard.ListPredictionsByMatch)

			// DLQ
			r.Get("/dlq", h.adminDLQ.Stats)
			r.Post("/dlq/replay", h.adminDLQ.Replay)
			r.Delete("/dlq", h.adminDLQ.Purge)

			// Audit log
			r.Get("/audit-log", h.adminAudit.List)
			r.Get("/audit-log/entity/{type}/{id}", h.adminAudit.ListByEntity)

			// System parameters
			r.Get("/system-params", h.adminParam.ListAll)
			r.Get("/system-params/{key}", h.adminParam.Get)
			r.Patch("/system-params/{key}", h.adminParam.Set)
			r.Post("/system-params/{key}/reset", h.adminParam.Reset)
			r.Post("/system-params/bulk", h.adminParam.BulkSet)

			// Tiebreakers
			r.Get("/tiebreaker/submissions", h.adminTiebreaker.ListSubmissions)

			// Conflicts
			r.Get("/conflicts", h.adminConflict.ListConflicts)
			r.Post("/conflicts/{type}/{id}/resolve", h.adminConflict.ResolveConflict)

			// Stats / observability
			r.Get("/stats", h.adminStats.GetDashboardStats)
			r.Get("/stats/conflicts/summary", h.adminConflict.ConflictSummary)

			// Bulk group operations
			r.Post("/groups/bulk-delete", h.adminGroup.BulkDeleteGroups)
			r.Post("/groups/{id}/members/bulk-remove", h.adminGroup.BulkRemoveMembers)
			r.Post("/groups/{id}/leaderboard/recalculate", h.adminGroup.RecalculateLeaderboard)

			// Scoring rules
			r.Get("/scoring-rules", h.adminScoringRules.List)
			r.Get("/scoring-rules/{phase}", h.adminScoringRules.GetByPhase)
			r.Patch("/scoring-rules/{phase}", h.adminScoringRules.Update)
		})
	})

	return r
}

// registerLocalSubscribers wires domain event handlers onto the in-process bus.
// It is only called when EventBus.Driver != "redis"; with the Redis driver, the
// worker process owns all event consumption exclusively and the API server only
// publishes. scorer is passed in - not re-constructed here - so the same
// stateless scoring instance is shared with the match service.
func (s *Server) registerLocalSubscribers(scorer service.MatchScorer) {
	s.bus.Subscribe(events.EventMatchFinished, func(ctx context.Context, env events.Envelope) error {
		mf, ok := env.Payload.(events.MatchFinished)
		if !ok {
			// Malformed payload: retrying will not help.
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
			return err
		}
		return nil
	})
}

// buildHandlers constructs the service layer (with optional cache decorators)
// and returns the route handlers. The provided repositories are reused from
// the caller's shared instances. scorer is the pre-built MatchScorer shared
// with registerLocalSubscribers so it is not constructed twice. s.bus is
// passed to services so they can publish domain events. When s.cache is
// non-nil, list-heavy services are wrapped with read-through / write-invalidation
// cache decorators.
func (s *Server) buildHandlers(
	ctx context.Context,
	repos coreRepos,
	params service.SystemParamService,
	scorer service.MatchScorer,
) appHandlers {
	quinielaRepo := repository.NewPostgresQuinielaRepository(s.db)
	tiebreakerRepo := repository.NewPostgresTiebreakerRepository(s.db)
	tiebreakerConfigRepo := repository.NewPostgresTiebreakerConfigRepository(s.db)
	tournamentRepo := repository.NewPostgresTournamentRepository(s.db)
	auditLogRepo := repository.NewPostgresAuditLogRepository(s.db)
	paymentRepo := repository.NewPostgresPaymentRecordRepository(s.db)
	snapRepo := repository.NewPostgresLeaderboardSnapshotRepository(s.db)
	scoringRuleRepo := repository.NewPostgresScoringRuleRepository(s.db)
	ledgerRepo := repository.NewPostgresBalanceLedgerRepository(s.db)
	proofRepo := repository.NewPostgresBankTransferProofRepository(s.db)
	withdrawalRepo := repository.NewPostgresWithdrawalRequestRepository(s.db)
	intentRepo := repository.NewPostgresPaymentIntentRepository(s.db)

	// Read infrastructure params (startup-time, not per-request).
	// ctx is the shared startup context created once in Routes() and passed here
	// to avoid redundant context.Background() calls and to enable timeout injection in tests.
	auditTimeout := time.Duration(params.GetInt(ctx, domain.ParamKeyAuditWriteTimeout, domain.DefaultAuditWriteTimeoutSeconds)) * time.Second
	matchTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyCacheMatchTTL, domain.DefaultCacheMatchTTLSeconds)) * time.Second
	leaderboardTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyCacheLeaderboardTTL, domain.DefaultCacheLeaderboardTTLSeconds)) * time.Second

	auditSvc := service.NewAuditService(auditLogRepo, auditTimeout, s.log)
	// Store auditSvc on the server so the shutdown path can call Drain() to
	// wait for in-flight audit writes before closing the database pool.
	s.auditSvc = auditSvc

	// Re-wire paramSvc with the now-available audit service so that Set/BulkSet
	// calls from admin handlers are recorded in the audit trail.
	paramSvcWithAudit := service.NewSystemParamService(repos.sysParam, auditSvc, s.log)

	matchSvc := service.NewMatchService(repos.match, s.bus, scorer, auditSvc, s.log)
	if s.cache != nil {
		matchSvc = service.NewCachedMatchService(matchSvc, s.cache, matchTTL, s.log)
	}

	predSvc := service.NewPredictionService(repos.pred, repos.match, params, clock.Real{}, s.log)
	groupAuthz := service.NewGroupAuthzService(repos.member)
	quinielaSvc := service.NewQuinielaService(quinielaRepo, groupAuthz, params, auditSvc, codegen.Crypto{})
	paymentSvc := service.NewPaymentService(paymentRepo, auditSvc, s.log)
	memberSvc := service.NewGroupMembershipService(quinielaRepo, repos.member, params, auditSvc, paymentSvc, clock.Real{}, s.log)

	ranker := service.NewRankingService(quinielaRepo, repos.pred, repos.user, repos.member, tiebreakerRepo, tiebreakerConfigRepo, s.log)
	if s.cache != nil {
		cachedRanker := service.NewCachedRankingService(ranker, s.cache, leaderboardTTL, s.log)
		// When an admin changes cache.leaderboard_ttl_seconds, update the active
		// TTL for future cache writes and flush all existing leaderboard entries so
		// the change takes effect immediately rather than after natural expiry.
		if mh, ok := paramSvcWithAudit.(service.MutationHookRegisterer); ok {
			mh.RegisterMutationHook(domain.ParamKeyCacheLeaderboardTTL,
				leaderboardTTLHook(paramSvcWithAudit, cachedRanker))
		}
		ranker = cachedRanker
	}

	userStatsSvc := service.NewUserStatsService(repos.pred)
	tiebreakerSvc := service.NewTiebreakerService(tiebreakerConfigRepo, groupAuthz, tiebreakerRepo, auditSvc, s.log)
	tournamentSvc := service.NewTournamentService(repos.match, tournamentRepo, params, auditSvc, s.log)
	snapshotter := service.NewLeaderboardSnapshotService(ranker, snapRepo)
	adminGroupSvc := service.NewAdminGroupService(quinielaRepo, repos.member, snapshotter, auditSvc, s.log)
	adminUserSvc := service.NewAdminUserService(repos.user, repos.member, paymentRepo, auditSvc, s.log)
	adminReadSvc := service.NewAdminReadService(
		service.AdminReadRepos{
			Pred: repos.pred, User: repos.user, Quiniela: quinielaRepo,
			Payment: paymentRepo, Tiebreaker: tiebreakerRepo, Snapshot: snapRepo,
			GlobalCache: s.cache,
		},
		params, s.log,
	)
	conflictSvc := service.NewConflictService(quinielaRepo, repos.member, paymentRepo, params, auditSvc, s.log)
	scoringRuleSvc := service.NewScoringRuleService(scoringRuleRepo, auditSvc, s.log)

	dlqSvc := s.dlqSvc
	if dlqSvc == nil {
		dlqSvc = service.NoopDLQService{}
	}

	fileStore, err := storage.New(storage.Config{
		Driver:   s.cfg.Storage.Driver,
		LocalDir: s.cfg.Storage.LocalDir,
		// s3
		S3Bucket:      s.cfg.Storage.S3Bucket,
		S3Endpoint:    s.cfg.Storage.S3Endpoint,
		S3Region:      s.cfg.Storage.S3Region,
		S3AccessKeyID: s.cfg.Storage.S3AccessKeyID,
		S3SecretKey:   s.cfg.Storage.S3SecretKey,
		// onedrive
		OneDriveTenantID:     s.cfg.Storage.OneDriveTenantID,
		OneDriveClientID:     s.cfg.Storage.OneDriveClientID,
		OneDriveClientSecret: s.cfg.Storage.OneDriveClientSecret,
		OneDriveDriveID:      s.cfg.Storage.OneDriveDriveID,
		// gdrive
		GDriveCredentialsJSON: s.cfg.Storage.GDriveCredentialsJSON,
		GDriveFolderID:        s.cfg.Storage.GDriveFolderID,
	})
	if err != nil {
		s.log.Warn("storage: falling back to local driver", zap.Error(err))
		fileStore, _ = storage.New(storage.Config{Driver: "local", LocalDir: "uploads"})
	}
	maxUploadBytes := int64(params.GetInt(ctx, domain.ParamKeyPaymentMaxUploadBytes, domain.DefaultPaymentMaxUploadBytes))
	minTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMinAmountCents, domain.DefaultBankTransferMinAmountCents)
	maxTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMaxAmountCents, domain.DefaultBankTransferMaxAmountCents)

	balanceSvc := service.NewBalanceService(repos.user, ledgerRepo, s.log)
	bankTransferSvc := service.NewBankTransferService(proofRepo, auditSvc, s.log)
	paymentIntentSvc := service.NewPaymentIntentService(intentRepo, params, s.log)
	webhookPaymentSvc := service.NewWebhookPaymentService(ledgerRepo, intentRepo, auditSvc, s.log)
	withdrawalSvc := service.NewWithdrawalService(withdrawalRepo, repos.sysParam, auditSvc, s.log)

	return appHandlers{
		match:             handler.NewMatchHandler(matchSvc, s.log),
		prediction:        handler.NewPredictionHandler(predSvc, s.log),
		group:             handler.NewGroupHandler(quinielaSvc, memberSvc, s.log),
		leaderboard:       handler.NewLeaderboardHandler(ranker, s.log),
		userStats:         handler.NewUserStatsHandler(userStatsSvc, s.log),
		tiebreaker:        handler.NewTiebreakerHandler(tiebreakerSvc, s.log),
		tournament:        handler.NewTournamentHandler(tournamentSvc, s.log),
		balance:           handler.NewBalanceHandler(balanceSvc, s.log),
		bankTransfer:      handler.NewBankTransferHandler(bankTransferSvc, fileStore, maxUploadBytes, minTransferCents, maxTransferCents, s.log),
		withdrawal:        handler.NewWithdrawalHandler(withdrawalSvc, s.log),
		paymentIntent:     handler.NewPaymentIntentHandler(paymentIntentSvc, s.log),
		paymentWebhook:    handler.NewPaymentWebhookHandler(webhookPaymentSvc, s.log),
		adminUser:         handler.NewAdminUserHandler(adminUserSvc, s.log),
		adminGroup:        handler.NewAdminGroupHandler(adminGroupSvc, params, s.log),
		adminPayment:      handler.NewAdminPaymentHandler(paymentSvc, s.log),
		adminLeaderboard:  handler.NewAdminLeaderboardHandler(adminReadSvc, params, s.log),
		adminDLQ:          handler.NewAdminDLQHandler(dlqSvc, s.log),
		adminAudit:        handler.NewAdminAuditHandler(auditSvc, s.log),
		adminParam:        handler.NewAdminSystemParamHandler(paramSvcWithAudit, s.log),
		adminTiebreaker:   handler.NewAdminTiebreakerHandler(adminReadSvc, s.log),
		adminConflict:     handler.NewAdminConflictHandler(conflictSvc, s.log),
		adminStats:        handler.NewAdminStatsHandler(adminReadSvc, s.log),
		adminScoringRules: handler.NewAdminScoringRuleHandler(scoringRuleSvc, s.log),
	}
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

// leaderboardTTLHook returns the mutation hook registered for
// ParamKeyCacheLeaderboardTTL. When the admin updates that param, the hook
// reads the fresh value and propagates it to ranker so the change takes effect
// immediately without a process restart.
func leaderboardTTLHook(paramSvc service.SystemParamService, ranker *service.CachedRankingService) func(context.Context) {
	return func(ctx context.Context) {
		newTTL := time.Duration(paramSvc.GetInt(
			ctx, domain.ParamKeyCacheLeaderboardTTL, domain.DefaultCacheLeaderboardTTLSeconds,
		)) * time.Second
		ranker.UpdateTTL(newTTL)
		ranker.InvalidateAll(ctx)
	}
}
