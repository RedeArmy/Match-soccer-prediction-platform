package api

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	_ "github.com/rede/world-cup-quiniela/docs" // registers the Swagger spec at init time
	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/middleware"
	"github.com/rede/world-cup-quiniela/internal/notification/hub"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
	"github.com/rede/world-cup-quiniela/pkg/auth"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/idempotency"
)

const (
	routePredictions = "/predictions"
	routeUsers       = "/users"
)

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

	// Static assets — Service Worker must be served at the root scope so that
	// it controls all pages. The embedded FS is built into the binary at compile
	// time; no separate asset deployment step is required.
	staticSub, _ := fs.Sub(staticFiles, "static")
	staticServer := http.FileServer(http.FS(staticSub))
	r.Get("/sw.js", staticServer.ServeHTTP)
	r.Get("/push.js", staticServer.ServeHTTP)
	r.Get("/icons/*", staticServer.ServeHTTP)

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
		return otelhttp.NewHandler(r, "world-cup-quiniela.api",
			otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
		)
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
	repository.InitRetryPolicy(
		paramSvc.GetInt(infraCtx, domain.ParamKeyTxRetryMaxAttempts, domain.DefaultTxRetryMaxAttempts),
		paramSvc.GetInt(infraCtx, domain.ParamKeyTxRetryBaseDelayMs, domain.DefaultTxRetryBaseDelayMs),
		paramSvc.GetInt(infraCtx, domain.ParamKeyTxRetryMaxDelayMs, domain.DefaultTxRetryMaxDelayMs),
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

	// SSE hub — created once and shared by the notification handler and the
	// pg_notify bridge goroutine.  The bridge itself is started explicitly via
	// StartPgNotifyBridge(), called from cmd/api/main.go after Routes() returns.
	// Keeping the start out of Routes() prevents goroutine leaks in tests that
	// call Routes() on a Server they then discard without a matching Stop call.
	s.notifHub = hub.New()

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
	// Wrap the PayPal cert fetcher with a circuit breaker. If PayPal's certificate
	// endpoint is repeatedly unavailable, the breaker opens and subsequent webhook
	// deliveries return 500 immediately (no network timeout wait), prompting PayPal
	// to retry the delivery later when the endpoint has recovered.
	paypalCertBreaker := breaker.New(
		"paypal-cert",
		paramSvc.GetInt(infraCtx, domain.ParamKeyBreakerPaypalCertMaxFails, domain.DefaultBreakerPaypalCertMaxFails),
		time.Duration(paramSvc.GetInt(infraCtx, domain.ParamKeyBreakerPaypalCertCooldownSec, domain.DefaultBreakerPaypalCertCooldownSec))*time.Second,
	)
	certFetcher := middleware.BreakerCertFetcher(middleware.DefaultPayPalCertFetcher(), paypalCertBreaker, s.log)
	r.With(middleware.PayPalWebhookAuth(s.cfg.Payment.PayPalWebhookID, certFetcher, s.log)).
		Post("/webhooks/paypal", h.paymentWebhook.HandlePayPal)

	// One-click email unsubscribe — no Clerk auth required; the signed token
	// provides its own authentication (see internal/notification/unsubscribe).
	// Registered on the root router before /api/v1 so it bypasses RequireAuth.
	r.Get("/api/v1/notifications/unsubscribe", h.notification.Unsubscribe)

	// Idempotency store for payment write endpoints. The in-memory store is
	// correct for single-process deployments; replace with a Redis-backed store
	// for multi-replica deployments where reservations must be visible across
	// all instances.
	idemTTL := time.Duration(paramSvc.GetInt(infraCtx, domain.ParamKeyAPIIdempotencyTTLHours, domain.DefaultAPIIdempotencyTTLHours)) * time.Hour
	idemKeyMaxLen := paramSvc.GetInt(infraCtx, domain.ParamKeyAPIIdempotencyKeyMaxLen, domain.DefaultAPIIdempotencyKeyMaxLen)
	idemStore := idempotency.NewMemoryStore()
	idem := middleware.Idempotency(idemStore, s.log, idemTTL, idemKeyMaxLen)

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
		r.Use(VersionHeader("v1"))
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
			r.With(idem).Post("/", h.paymentIntent.Create)
		})

		r.Route("/bank-transfers", func(r chi.Router) {
			r.Use(middleware.ResolveUser(repos.user, s.log))
			// POST receives a multipart upload (up to uploadSizeLimit); GET has no
			// body. Per-route limits avoid the MaxBytesReader stacking problem where
			// an outer smaller limit silently blocks the inner larger one.
			r.With(middleware.RequestBodyLimit(uploadSizeLimit), idem).Post("/", h.bankTransfer.Upload)
			r.With(middleware.RequestBodyLimit(bodySizeLimit)).Get("/", h.bankTransfer.ListMine)
		})

		r.Route("/withdrawals", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.With(idem).Post("/", h.withdrawal.Create)
			r.Get("/", h.withdrawal.ListMine)
		})

		r.Route("/notifications", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/", h.notification.GetInbox)
			r.Get("/stream", h.notification.GetStream)
			r.Post("/mark-read", h.notification.MarkRead)
			r.Get("/preferences", h.notification.GetPreferences)
			r.Patch("/preferences", h.notification.UpdatePreferences)
		})

		r.Route("/push", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/vapid-public-key", h.notification.GetVAPIDPublicKey)
			r.Post("/subscribe", h.notification.SubscribePush)
			r.Delete("/subscribe", h.notification.UnsubscribePush)
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

			// DLQ — Redis Streams (event processing failures)
			r.Get("/dlq", h.adminDLQ.Stats)
			r.Post("/dlq/replay", h.adminDLQ.Replay)
			r.Delete("/dlq", h.adminDLQ.Purge)

			// Notification DLQ — PostgreSQL (delivery failures: email, push, in-app)
			r.Get("/notification-dlq", h.adminNotifDLQ.Stats)
			r.Post("/notification-dlq/{id}/resolve", h.adminNotifDLQ.Resolve)

			// Audit log
			r.Get("/audit-log", h.adminAudit.List)
			r.Get("/audit-log/entity/{type}/{id}", h.adminAudit.ListByEntity)

			// System parameters
			r.Get("/system-params", h.adminParam.ListAll)
			r.Get("/system-params/{key}", h.adminParam.Get)
			r.Patch("/system-params/{key}", h.adminParam.Set)
			r.Post("/system-params/{key}/reset", h.adminParam.Reset)
			r.Get("/system-params/{key}/history", h.adminParam.History)
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

			// SSE hub observability — per-replica counters; aggregate in Prometheus for cluster totals
			r.Get("/notifications/sse/stats", h.adminSSEStats.Stats)

			// Notification content templates (DB-backed, operator-editable)
			const tmplByKey = "/notification-templates/{event_type}/{locale}"
			r.Get("/notification-templates", h.adminNotifTemplate.List)
			r.Get(tmplByKey, h.adminNotifTemplate.Get)
			r.Put(tmplByKey, h.adminNotifTemplate.Upsert)
			r.Delete(tmplByKey, h.adminNotifTemplate.Delete)
			r.Post(tmplByKey+"/preview", h.adminNotifTemplate.Preview)
			r.Get(tmplByKey+"/history", h.adminNotifTemplate.History)
			r.Post(tmplByKey+"/rollback", h.adminNotifTemplate.Rollback)
		})
	})

	return otelhttp.NewHandler(r, "world-cup-quiniela.api",
		otelhttp.WithMessageEvents(otelhttp.ReadEvents, otelhttp.WriteEvents),
	)
}

// registerLocalSubscribers wires domain event handlers onto the in-process bus.
// It is only called when EventBus.Driver != "redis"; with the Redis driver, the
// worker process owns all event consumption exclusively and the API server only
// publishes. scorer is passed in - not re-constructed here - so the same
// stateless scoring instance is shared with the match service.
func (s *Server) registerLocalSubscribers(scorer service.MatchScorer) {
	s.bus.Subscribe(context.Background(), events.EventMatchFinished, func(ctx context.Context, env events.Envelope) error {
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
