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
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
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
)

const (
	routePredictions = "/predictions"
	routeUsers       = "/users"
)

// Routes returns the fully configured http.Handler for the application.
//
// ctx is used for one-time startup reads (parameter loads, JWKS warmup). Pass
// context.WithoutCancel(lifecycleCtx) from cmd/api so that OTel trace values
// are propagated to DB queries without inheriting the process shutdown signal.
// Tests pass context.Background().
//
// The routing table is arranged in three tiers:
//
//  1. Infrastructure endpoints (/health, /swagger) — not versioned, no auth.
//  2. Webhook endpoints — provider-specific signature auth (Svix, HMAC, RSA).
//  3. Business endpoints under /api/v1 — JWT auth via RequireAuth middleware.
//
// Middleware is applied in declaration order; RequestID must come first so its
// value is available to every subsequent handler and logger.
func (s *Server) Routes(ctx context.Context) http.Handler {
	r := chi.NewRouter()

	// Global middleware - applied to every request.
	r.Use(middleware.SecurityHeaders) // outermost: headers present on every response
	r.Use(chimiddleware.RequestID)
	r.Use(middleware.TrustedClientIP) // replaces chi's RealIP; reads Fly-Client-IP to prevent header injection
	r.Use(middleware.StoreClientIP)
	r.Use(middleware.Recover(s.log))
	r.Use(middleware.RequestLogger(s.log))
	r.Use(middleware.CORS(s.cfg.CORS.AllowedOrigins))
	r.Use(middleware.NewMetrics(otel.GetMeterProvider().Meter("wcq")))

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
			r.Use(middleware.RequireAuth(auth.NewJWKSProvider(ctx, s.cfg.Clerk.JWKSURL, auth.DefaultWarmupTimeout, s.log), s.log))
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
	// restart (is_runtime=FALSE in system_params).
	messaging.Configure(
		paramSvc.GetInt(ctx, domain.ParamKeyMessagingMaxRetries, domain.DefaultMessagingMaxRetries),
		int64(paramSvc.GetInt(ctx, domain.ParamKeyMessagingStreamMaxLen, domain.DefaultMessagingStreamMaxLen)),
		paramSvc.GetInt(ctx, domain.ParamKeyMessagingStreamWorkerCount, domain.DefaultMessagingStreamWorkerCount),
		paramSvc.GetInt(ctx, domain.ParamKeyMessagingStreamReadBlockSec, domain.DefaultMessagingStreamReadBlockSec),
		nil, // retain default RetryBackoff (1s, 2s); no array param defined
	)
	service.ConfigureAuditRetry(
		paramSvc.GetInt(ctx, domain.ParamKeyAuditMaxRetries, domain.DefaultAuditMaxRetries),
		paramSvc.GetInt(ctx, domain.ParamKeyAuditRetryDelayMs, domain.DefaultAuditRetryDelayMs),
	)
	repository.InitRetryPolicy(
		paramSvc.GetInt(ctx, domain.ParamKeyTxRetryMaxAttempts, domain.DefaultTxRetryMaxAttempts),
		paramSvc.GetInt(ctx, domain.ParamKeyTxRetryBaseDelayMs, domain.DefaultTxRetryBaseDelayMs),
		paramSvc.GetInt(ctx, domain.ParamKeyTxRetryMaxDelayMs, domain.DefaultTxRetryMaxDelayMs),
	)
	bodySizeLimit := int64(paramSvc.GetInt(ctx, domain.ParamKeyAPIBodySizeLimitBytes, domain.DefaultAPIBodySizeLimitBytes))
	uploadSizeLimit := int64(paramSvc.GetInt(ctx, domain.ParamKeyPaymentMaxUploadBytes, domain.DefaultPaymentMaxUploadBytes))
	authWarmup := time.Duration(paramSvc.GetInt(ctx, domain.ParamKeyAuthValidationTimeout, domain.DefaultAuthValidationTimeoutSeconds)) * time.Second

	// scorer is constructed once and shared: local event subscribers and the
	// match service both use the same stateless scoring logic. With the redis
	// driver, the worker process owns all event consumption; the API server
	// only publishes, so local subscription is skipped.
	ruleRepo := repository.NewPostgresScoringRuleRepository(s.db)
	scorer := service.NewScoringService(repos.match, repos.pred, ruleRepo, paramSvc, s.log,
		service.WithScoringMeter(otel.GetMeterProvider().Meter("wcq")),
	)
	if s.cfg.EventBus.Driver != "redis" {
		s.registerLocalSubscribers(ctx, scorer)
	}

	// SSE hub — created once and shared by the notification handler and the
	// pg_notify bridge goroutine.  The bridge itself is started explicitly via
	// StartPgNotifyBridge(), called from cmd/api/main.go after Routes() returns.
	// Keeping the start out of Routes() prevents goroutine leaks in tests that
	// call Routes() on a Server they then discard without a matching Stop call.
	sseChanBufSize := paramSvc.GetInt(ctx, domain.ParamKeyNotifySSEChanBufSize, domain.DefaultNotifySSEChanBufSize)
	sseMaxConns := paramSvc.GetInt(ctx, domain.ParamKeyNotifySSEMaxConnsPerUser, domain.DefaultNotifySSEMaxConnsPerUser)
	s.notifHub = hub.NewWithOptions(hub.Options{
		ChanBufSize:     sseChanBufSize,
		MaxConnsPerUser: sseMaxConns,
	})
	if err := s.notifHub.RegisterMetrics(otel.GetMeterProvider().Meter("wcq")); err != nil {
		s.log.Warn("hub.RegisterMetrics failed (metrics may be unavailable)", zap.Error(err))
	}

	h := s.buildHandlers(ctx, repos, paramSvc, scorer)

	// Register per-handler metrics after construction so every handler that
	// exposes OTel instruments is wired to the global meter.
	meter := otel.GetMeterProvider().Meter("wcq")

	if err := h.paymentWebhook.RegisterMetrics(meter); err != nil {
		s.log.Warn("paymentWebhook.RegisterMetrics failed", zap.Error(err))
	}

	// IP-based rate limiter (L1 global + L2 webhook).
	// Constructed here — after paramSvc and meter are available — with the system
	// params read once at startup. is_runtime=FALSE: the LimiterStores are fixed
	// at construction time; a process restart is required for new param values.
	ipGlobalStore := middleware.NewLimiterStore(
		float64(paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitGlobalRPS, domain.DefaultIPRateLimitGlobalRPS)),
		paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitGlobalBurst, domain.DefaultIPRateLimitGlobalBurst),
	)
	ipWebhookStore := middleware.NewLimiterStore(
		float64(paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitWebhookRPS, domain.DefaultIPRateLimitWebhookRPS)),
		paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitWebhookBurst, domain.DefaultIPRateLimitWebhookBurst),
	)
	ipLimiter := middleware.NewIPRateLimiter(ipGlobalStore, ipWebhookStore, meter, s.log)

	// Webhook endpoints — authenticated via provider-specific signatures, not Clerk JWT.
	// Grouped under /webhooks so the L2 IP rate limiter can be applied once to the
	// entire group. This protects CPU-expensive RSA verification (PayPal) from
	// replay floods before signature verification runs.
	// Signature schemes per provider:
	//   clerk:      Svix HMAC-SHA256 (verified inside WebhookHandler)
	//   recurrente: HMAC-SHA256 via RecurrenteWebhookAuth middleware
	//   paypal:     RSA certificate verification via PayPalWebhookAuth middleware
	clerkSyncer := service.NewClerkUserSyncService(repos.user, repository.NewPostgresKYCProfileRepository(s.db), s.log)
	webhookHandler := handler.NewWebhookHandler(clerkSyncer, s.cfg.Clerk.WebhookSecret, s.log)
	// The Clerk webhook carries no money movement and uses Svix replay protection,
	// so it is exempt from the L2 IP rate limiter. Payment webhooks receive the
	// stricter L2 bucket to block replay attacks cycling fake source IPs.
	r.Post("/webhooks/clerk", webhookHandler.HandleClerkWebhook)
	r.With(ipLimiter.Webhook(), middleware.RecurrenteWebhookAuth(s.cfg.Payment.RecurrenteWebhookSecret, s.log)).
		Post("/webhooks/recurrente", h.paymentWebhook.HandleRecurrente)
	// Wrap the PayPal cert fetcher with a circuit breaker. If PayPal's certificate
	// endpoint is repeatedly unavailable, the breaker opens and subsequent webhook
	// deliveries return 500 immediately (no network timeout wait), prompting PayPal
	// to retry the delivery later when the endpoint has recovered.
	paypalCertBreaker := breaker.New(
		"paypal-cert",
		paramSvc.GetInt(ctx, domain.ParamKeyBreakerPaypalCertMaxFails, domain.DefaultBreakerPaypalCertMaxFails),
		time.Duration(paramSvc.GetInt(ctx, domain.ParamKeyBreakerPaypalCertCooldownSec, domain.DefaultBreakerPaypalCertCooldownSec))*time.Second,
	)
	certFetcher := middleware.BreakerCertFetcher(middleware.DefaultPayPalCertFetcher(), paypalCertBreaker, s.log)
	r.With(ipLimiter.Webhook(), middleware.PayPalWebhookAuth(s.cfg.Payment.PayPalWebhookID, certFetcher, s.log)).
		Post("/webhooks/paypal", h.paymentWebhook.HandlePayPal)
	if err := breaker.RegisterGauge(meter, paypalCertBreaker); err != nil {
		s.log.Warn("breaker.RegisterGauge(paypal-cert) failed", zap.Error(err))
	}
	s.breakerRegistry.Register(paypalCertBreaker)

	// INTENTIONAL AUTH BYPASS: This endpoint authenticates via a time-limited
	// HMAC-signed token embedded in the unsubscribe URL (signed with
	// WCQ_EMAIL_UNSUBSCRIBESECRET). Do NOT move it inside the /api/v1 subrouter
	// below — it must remain on the root router so it is reached before
	// RequireAuth. Token validation happens inside the Unsubscribe handler.
	r.Get("/api/v1/notifications/unsubscribe", h.notification.Unsubscribe)

	// Idempotency store for payment write endpoints.
	// SetIdempotencyStore (called from cmd/api/main.go before Routes()) wires the
	// Redis-backed store when Redis is available so reservations are shared across
	// all replicas. Falls back to MemoryStore for single-process deployments.
	idemTTL := time.Duration(paramSvc.GetInt(ctx, domain.ParamKeyAPIIdempotencyTTLHours, domain.DefaultAPIIdempotencyTTLHours)) * time.Hour
	idemKeyMaxLen := paramSvc.GetInt(ctx, domain.ParamKeyAPIIdempotencyKeyMaxLen, domain.DefaultAPIIdempotencyKeyMaxLen)

	// ensureIdempotencyStore returns true when it fell back to MemoryStore.
	// Emit the degraded counter once at startup so WCQIdempotencyDegraded
	// (for:0m) fires immediately; the per-request counter in the Idempotency
	// middleware only fires on Redis errors, not on permanent MemoryStore use.
	if s.ensureIdempotencyStore() {
		if c, err := meter.Int64Counter("wcq_idempotency_degraded_total"); err == nil {
			c.Add(ctx, 1)
		}
	}
	idem := middleware.Idempotency(s.idemStore, meter, s.log, idemTTL, idemKeyMaxLen)

	// Versioned API surface with Clerk JWT authentication.
	// Rate limit params are read once at startup (is_runtime=FALSE); a process
	// restart is required to change the rate or burst.
	clerkProvider := auth.NewJWKSProvider(ctx, s.cfg.Clerk.JWKSURL, authWarmup, s.log)

	// /metrics is admin-only: Prometheus scrape targets must present a valid
	// Clerk JWT with RoleAdmin. Registered here (after repos and clerkProvider
	// are available) rather than in the early infrastructure block above.
	if s.metricsHandler != nil {
		r.With(
			middleware.RequireAuth(clerkProvider, s.log),
			middleware.RequireRole(repos.user, s.log, domain.RoleAdmin),
		).Handle("/metrics", s.metricsHandler)
	}

	ratePerSec := float64(paramSvc.GetInt(ctx, domain.ParamKeyAPIRateLimitRatePerSec, domain.DefaultAPIRateLimitRatePerSec))
	rateBurst := paramSvc.GetInt(ctx, domain.ParamKeyAPIRateLimitBurst, domain.DefaultAPIRateLimitBurst)
	userRateStore := s.buildUserRateStore(meter, ratePerSec, rateBurst)
	r.Route("/api/v1", func(r chi.Router) {
		r.Use(VersionHeader("v1"))
		r.Use(ipLimiter.Global()) // L1: per-IP limit before auth, blocks unauthenticated scans
		r.Use(middleware.RequireAuth(clerkProvider, s.log))
		r.Use(middleware.RateLimitByUserID(userRateStore, s.log))

		// Admin-only match mutations are guarded by RequireRole. Read endpoints
		// (List, Get) require ResolveUser so banned users are rejected before
		// reaching the handler; the resolved domain.User is available in context
		// for any future handler that needs caller identity.
		r.Route("/matches", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
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
		// slot management (admin only). ResolveUser is applied at the subrouter
		// level so banned users are rejected on GET routes as well as admin
		// mutations. resolveRequestUser caches the user in context, so admin
		// mutations that use RequireRole do not pay a second database round-trip.
		r.Route("/tournament", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/standings", h.tournament.GetAllStandings)
			r.Get("/standings/{group}", h.tournament.GetGroupStanding)
			r.Get("/slots", h.tournament.ListSlots)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Post("/slots", h.tournament.CreateSlot)
			r.With(middleware.RequireRole(repos.user, s.log, domain.RoleAdmin)).Patch("/slots/{id}", h.tournament.ConfirmSlot)
		})

		r.Route(routeUsers, func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/me", h.user.GetMe)
			r.Patch("/me", h.user.UpdateMe)
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

		r.Route("/kyc", func(r chi.Router) {
			r.Use(middleware.RequestBodyLimit(bodySizeLimit))
			r.Use(middleware.ResolveUser(repos.user, s.log))
			r.Get("/status", h.kyc.GetStatus)
			r.Post("/submit", h.kyc.Submit)
			r.Get("/requirements", h.kyc.GetRequirements)
			r.Get("/documents", h.kyc.ListDocuments)
			r.Post("/documents", h.kyc.UploadDocument)
			r.Get("/events", h.kyc.ListEvents)
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

			// KYC review
			r.Get("/kyc/risk-dashboard", h.adminKYC.RiskDashboard)
			r.Get("/kyc/queue", h.adminKYC.ListQueue)
			r.Get("/kyc/profiles/{profileID}", h.adminKYC.GetProfile)
			r.Get("/kyc/profiles/{profileID}/documents", h.adminKYC.ListDocumentsForProfile)
			r.Get("/kyc/profiles/{profileID}/events", h.adminKYC.ListProfileEvents)
			r.Post("/kyc/profiles/{profileID}/approve", h.adminKYC.Approve)
			r.Post("/kyc/profiles/{profileID}/reject", h.adminKYC.Reject)
			r.Post("/kyc/profiles/{profileID}/escalate", h.adminKYC.Escalate)
			r.Post("/kyc/profiles/{profileID}/request-doc", h.adminKYC.RequestDocument)
			r.Post("/kyc/documents/{docID}/verify", h.adminKYC.VerifyDocument)
			r.Get("/kyc/frozen-balances", h.adminKYC.ListFrozenBalances)
			r.Post("/kyc/users/{userID}/release-freeze", h.adminKYC.ReleaseFrozenBalance)

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
			r.Post("/groups/{id}/distribute-prizes", h.adminGroup.DistributePrizes)

			// Scoring rules
			r.Get("/scoring-rules", h.adminScoringRules.List)
			r.Get("/scoring-rules/{phase}", h.adminScoringRules.GetByPhase)
			r.Patch("/scoring-rules/{phase}", h.adminScoringRules.Update)

			// SSE hub observability — per-replica counters; aggregate in Prometheus for cluster totals
			r.Get("/notifications/sse/stats", h.adminSSEStats.Stats)

			// Observability dashboard endpoints
			r.Route("/observability", func(r chi.Router) {
				r.Get("/metrics/summary", h.adminObservability.MetricsSummary)
				r.Get("/tracing/recent-errors", h.adminObservability.TracingRecentErrors)
				r.Get("/active-connections", h.adminObservability.ActiveConnections)
				r.Get("/dlq", h.adminObsDLQ.Stats)
				r.Get("/audit-log", h.adminAudit.List)
				r.Get("/circuit-breakers", h.adminObsCircuit.List)
				r.Get("/n8n/workflows", h.adminN8n.Workflows)
				r.Get("/n8n/executions/recent", h.adminN8n.RecentExecutions)
			})

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

// buildUserRateStore returns the effective per-user rate-limit store for the
// /api/v1 subrouter. Selection priority:
//
//  1. s.limiterStore — injected via SetLimiterStore / SetRateStore (tests only).
//  2. Redis-backed RedisRateStore — preferred in production; enforces limits
//     across all replicas and fails open when Redis is unavailable.
//  3. In-process LimiterStore — fallback when Redis is not configured; limits
//     are per-replica only and a warning is emitted to prompt the operator.
func (s *Server) buildUserRateStore(meter metric.Meter, ratePerSec float64, rateBurst int) middleware.Allower {
	if s.limiterStore != nil {
		return s.limiterStore
	}
	if s.redisClient != nil {
		rds := middleware.NewRedisRateStore(s.redisClient, ratePerSec, rateBurst, s.log)
		if err := rds.RegisterMetrics(meter); err != nil {
			s.log.Warn("RedisRateStore.RegisterMetrics failed (metrics may be unavailable)", zap.Error(err))
		}
		return rds
	}
	s.log.Warn("rate limiter: Redis not configured — using in-process store (limits not shared across replicas)",
		zap.String("remedy", "set WCQ_REDIS_ADDR to enforce limits cluster-wide"),
	)
	return middleware.NewLimiterStore(ratePerSec, rateBurst)
}

// registerLocalSubscribers wires domain event handlers onto the in-process bus.
// It is only called when EventBus.Driver != "redis"; with the Redis driver, the
// worker process owns all event consumption exclusively and the API server only
// publishes. scorer is passed in - not re-constructed here - so the same
// stateless scoring instance is shared with the match service.
func (s *Server) registerLocalSubscribers(ctx context.Context, scorer service.MatchScorer) {
	s.bus.Subscribe(ctx, events.EventMatchFinished, func(ctx context.Context, env events.Envelope) error {
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
