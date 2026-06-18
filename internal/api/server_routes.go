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
	"go.opentelemetry.io/otel/attribute"
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
	routePredictions      = "/predictions"
	routeUsers            = "/users"
	routeBanks            = "/banks"
	routeBankAccountTypes = "/bank-account-types"
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

	if s.db == nil {
		// When the database is unavailable, register health and a catch-all 503
		// stub for the API surface. Infrastructure endpoints remain reachable so
		// load-balancer health checks work; IP rate limiting is not applied in
		// degraded mode (no business logic runs, so the risk is acceptable).
		// Wildcard coverage means new routes added to the happy path are
		// automatically covered without a second edit here.
		s.registerPublicRoutes(r)
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
	service.ConfigureAuditMaxInFlight(
		paramSvc.GetInt(ctx, domain.ParamKeyAuditMaxInFlight, domain.DefaultAuditMaxInFlight),
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
	sseEvictAfterDrops := paramSvc.GetInt(ctx, domain.ParamKeyNotifySSEEvictAfterDrops, domain.DefaultNotifySSEEvictAfterDrops)
	s.notifHub = hub.NewWithOptions(hub.Options{
		ChanBufSize:     sseChanBufSize,
		MaxConnsPerUser: sseMaxConns,
		EvictAfterDrops: sseEvictAfterDrops,
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
	// params read once at startup. is_runtime=FALSE: the stores are fixed at
	// construction time; a process restart is required for new param values.
	// Redis is used when available so limits are enforced cluster-wide across
	// replicas; falls back to in-process on Redis unavailability (fail-open).
	ipGlobalStore := s.buildIPRateStore(meter,
		float64(paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitGlobalRPS, domain.DefaultIPRateLimitGlobalRPS)),
		paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitGlobalBurst, domain.DefaultIPRateLimitGlobalBurst),
	)
	ipWebhookStore := s.buildIPRateStore(meter,
		float64(paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitWebhookRPS, domain.DefaultIPRateLimitWebhookRPS)),
		paramSvc.GetInt(ctx, domain.ParamKeyIPRateLimitWebhookBurst, domain.DefaultIPRateLimitWebhookBurst),
	)
	ipLimiter := middleware.NewIPRateLimiter(ipGlobalStore, ipWebhookStore, meter, s.log)
	s.recordIPRateStoreMode(ctx, meter)
	// L1 is registered on the root router so it covers every route: /health,
	// /webhooks/*, static assets, and all /api/v1/* endpoints.  TrustedClientIP
	// must run first (registered above) to normalise r.RemoteAddr before this
	// middleware reads it.  chi requires all r.Use() calls before any route
	// registrations on the same mux; health/static routes are registered below.
	r.Use(ipLimiter.Global())

	// Infrastructure endpoints — registered after r.Use(ipLimiter.Global()) so
	// they are covered by L1.  In degraded mode (s.db==nil, handled above)
	// these routes are registered by registerPublicRoutes without the IP limiter.
	s.registerPublicRoutes(r)

	// Public exchange rate display — no auth required, covered by L1 IP limiter.
	// Registered outside /api/v1 (which enforces RequireAuth) so unauthenticated
	// frontend clients can display current buy/sell rates.
	r.Get("/api/exchange-rate", h.exchangeRate.GetCurrent)

	// Public group leaderboard — no auth required, covered by L1 IP limiter.
	// Allows unauthenticated visitors to preview a group's standings by invite
	// code. Prize-winner flags and payment state are stripped by the handler.
	r.Get("/api/public/groups/leaderboard", h.publicGroup.GetPublicLeaderboard)

	// Public group-stage matches — no auth required, covered by L1 IP limiter.
	// Returns all group-stage fixtures (teams, scores, status) so the public
	// landing page can compute and display live FIFA standings tables.
	r.Get("/api/public/matches/group-stage", h.match.ListPublicGroupStageMatches)

	// Public system clock — returns the current application time (real or dev-overridden).
	// No auth required; the frontend uses this to determine "today" for the HOY tab
	// and match-locking logic.  L1 IP limiter is applied via the root router.
	r.Get("/api/v1/system/clock", h.systemClock.GetClock)

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

	// INTENTIONAL AUTH BYPASS — middleware map for this route:
	//
	//  Applied (root-router middleware, wired in the header block above):
	//    SecurityHeaders, RequestID (chi), TrustedClientIP, StoreClientIP,
	//    Recover, RequestLogger, CORS, IPRateLimiter.Global
	//
	//  NOT applied (all live inside the /api/v1 subrouter registered below):
	//    RequireAuth, RateLimitByUserID, ResolveUser, RequestBodyLimit,
	//    Idempotency, VersionHeader
	//
	//  Rationale: the unsubscribe URL is embedded in outgoing emails.  Requiring a
	//  Clerk JWT in the link would force the user to be logged in — impractical for
	//  CAN-SPAM one-click unsubscribe.  Authentication is replaced by a time-limited
	//  HMAC-SHA256 token (WCQ_EMAIL_UNSUBSCRIBESECRET); the handler calls
	//  unsubscribe.VerifyToken before taking any action.
	//
	//  L1 IP rate limiting is applied via the global r.Use(ipLimiter.Global())
	//  registered above; no per-route r.With() is needed here.
	//  RequestBodyLimit is unnecessary (GET with no body).
	//
	//  If middleware is added to the root router in the future, audit this route
	//  to confirm the new middleware is compatible with unauthenticated access.
	r.Get("/api/v1/notifications/unsubscribe", h.notification.Unsubscribe)

	// Idempotency store for payment write endpoints.
	// SetIdempotencyStore (called from cmd/api/main.go before Routes()) wires the
	// Redis-backed store when Redis is available so reservations are shared across
	// all replicas. Falls back to MemoryStore for single-process deployments.
	idemTTL := time.Duration(paramSvc.GetInt(ctx, domain.ParamKeyAPIIdempotencyTTLHours, domain.DefaultAPIIdempotencyTTLHours)) * time.Hour
	idemKeyMaxLen := paramSvc.GetInt(ctx, domain.ParamKeyAPIIdempotencyKeyMaxLen, domain.DefaultAPIIdempotencyKeyMaxLen)

	// ensureIdempotencyStore returns true when it fell back to MemoryStore.
	// Emit the degraded counter once at startup so WCQIdempotencyDegraded
	// (for:0m, severity:critical) fires immediately; the per-request counter
	// in the Idempotency middleware only fires on Redis errors, not on permanent
	// MemoryStore use. The fail-open decision is documented in ADR 0014.
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

	ratePerSec := float64(paramSvc.GetInt(ctx, domain.ParamKeyAPIRateLimitRatePerSec, domain.DefaultAPIRateLimitRatePerSec))
	rateBurst := paramSvc.GetInt(ctx, domain.ParamKeyAPIRateLimitBurst, domain.DefaultAPIRateLimitBurst)
	userRateStore := s.buildUserRateStore(meter, ratePerSec, rateBurst)

	// Admin-panel rate limit: a separate, tighter in-process token bucket for
	// /api/v1/admin so a single admin cannot hammer expensive bulk endpoints
	// (POST /admin/system-params/bulk, POST /admin/groups/{id}/distribute-prizes)
	// without consuming their general user quota. In-process (no Redis) is
	// deliberate: admin users are few and cross-replica coordination is
	// unnecessary; the goal is preventing accidental loops, not precise
	// cluster-wide accounting.
	adminRatePerSec := float64(paramSvc.GetInt(ctx, domain.ParamKeyAdminRateLimitRatePerSec, domain.DefaultAdminRateLimitRatePerSec))
	adminRateBurst := paramSvc.GetInt(ctx, domain.ParamKeyAdminRateLimitBurst, domain.DefaultAdminRateLimitBurst)
	adminRateStore := middleware.NewLimiterStore(adminRatePerSec, adminRateBurst)
	// Wire mutation hook so that an admin updating admin.rate_limit_rate_per_sec
	// or admin.rate_limit_burst takes effect immediately for the in-process store.
	// Per-user and IP rate limiters are Redis-backed in production and still
	// require a restart — documented in constants_api.go.
	s.wireAdminRateLimitHook(h.paramSvc, adminRateStore)

	// d bundles the shared values passed to every /api/v1 route group helper.
	// Ordering within r.Route is preserved by the explicit call sequence below.
	d := apiV1Deps{
		h:               h,
		repos:           repos,
		bodySizeLimit:   bodySizeLimit,
		uploadSizeLimit: uploadSizeLimit,
		idem:            idem,
	}

	r.Route("/api/v1", func(r chi.Router) {
		r.Use(VersionHeader("v1"))
		// L1 (ipLimiter.Global) is applied at the root router — not here.
		r.Use(middleware.RequireAuth(clerkProvider, s.log))
		r.Use(middleware.RateLimitByUserID(userRateStore, s.log))

		s.registerMatchRoutes(r, d)
		s.registerPredictionRoutes(r, d)
		s.registerGroupRoutes(r, d)
		s.registerTiebreakerAdminRoutes(r, d)
		s.registerTournamentRoutes(r, d)
		s.registerUserRoutes(r, d)
		s.registerPaymentRoutes(r, d)
		s.registerKYCRoutes(r, d)
		s.registerNotificationRoutes(r, d)
		s.registerAdminRoutes(r, d, adminRateStore)
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

// recordIPRateStoreMode emits a startup gauge that records which IP rate-limit
// store is active. mode=redis means cluster-wide limits are enforced via Redis;
// mode=local means per-replica in-process buckets are used (Redis not configured).
//
// The metric enables a Prometheus alert when the local fallback is active in
// a multi-replica deployment, where each replica enforces independent limits
// and the effective cluster burst is replica_count × per-replica burst.
// See docs/adr/0013-ip-rate-limiter-fail-open-policy.md for the decision record.
func (s *Server) recordIPRateStoreMode(ctx context.Context, meter metric.Meter) {
	mode := "redis"
	if s.redisClient == nil {
		mode = "local"
	}
	g, err := meter.Int64Gauge(
		"wcq_ip_ratelimit_store_mode",
		metric.WithDescription("Active IP rate-limit store: 1 for mode=redis (cluster-wide) "+
			"or mode=local (per-replica fallback). Alert on mode=local when replica_count > 1."),
		metric.WithUnit("{mode}"),
	)
	if err != nil {
		s.log.Warn("wcq_ip_ratelimit_store_mode: failed to register gauge", zap.Error(err))
		return
	}
	g.Record(ctx, 1, metric.WithAttributes(attribute.String("mode", mode)))
}

// buildIPRateStore returns the effective per-IP rate-limit store for L1 (global)
// and L2 (webhook) limiters. Selection priority:
//
//  1. Redis-backed RedisRateStore — preferred; enforces limits across all replicas
//     and fails open when Redis is unavailable (wcq_rate_limit_fail_open_total).
//  2. In-process LimiterStore — fallback when Redis is not configured; limits are
//     per-replica only and a warning is emitted to prompt the operator.
//
// Unlike buildUserRateStore there is no test-injected override: tests construct
// IPRateLimiter directly with stub IPAllower implementations.
func (s *Server) buildIPRateStore(meter metric.Meter, ratePerSec float64, burst int) middleware.IPAllower {
	if s.redisClient != nil {
		rds := middleware.NewRedisRateStore(s.redisClient, ratePerSec, burst, s.log)
		if err := rds.RegisterMetrics(meter); err != nil {
			s.log.Warn("RedisRateStore.RegisterMetrics failed (IP limiter fail-open metric unavailable)", zap.Error(err))
		}
		return rds
	}
	s.log.Warn("ip rate limiter: Redis not configured — using in-process store (limits not shared across replicas)",
		zap.String("remedy", "set WCQ_REDIS_ADDR to enforce IP limits cluster-wide"),
	)
	return middleware.NewLimiterStore(ratePerSec, burst)
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

// registerPublicRoutes registers health, swagger, and static-asset routes on r.
// It is called twice: once in degraded mode (s.db==nil, no IP rate limiter) and
// once in normal mode after r.Use(ipLimiter.Global()) so all routes are covered.
// Keeping this in a helper avoids duplicating the route list in both branches.
func (s *Server) registerPublicRoutes(r chi.Router) {
	r.Get("/health", s.handleHealth)
	r.Get("/health/ready", s.handleReadiness)
	// Swagger UI is only served in development. Exposing it in production
	// would advertise every admin endpoint, payment webhook, and KYC route
	// to any observer — a significant reconnaissance surface.
	if s.cfg.IsDevelopment() {
		r.Get("/swagger/*", httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"),
			httpSwagger.DeepLinking(true),
		))
	}
	// Service Worker and push assets must be served at the root scope so the
	// SW controls all pages. The embedded FS is compiled into the binary.
	staticSub, _ := fs.Sub(staticFiles, "static")
	staticServer := http.FileServer(http.FS(staticSub))
	r.Get("/sw.js", staticServer.ServeHTTP)
	r.Get("/push.js", staticServer.ServeHTTP)
	r.Get("/icons/*", staticServer.ServeHTTP)
}
