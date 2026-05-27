package api

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/clock"
	"github.com/rede/world-cup-quiniela/pkg/promclient"
	"github.com/rede/world-cup-quiniela/pkg/randcode"
	"github.com/rede/world-cup-quiniela/pkg/tempoclient"
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

// kycModuleDeps groups the shared dependencies forwarded from buildHandlers to
// buildKYCModule, following the same pattern as coreRepos.
type kycModuleDeps struct {
	params            service.SystemParamService
	paramSvcWithAudit service.SystemParamService
	auditSvc          service.AuditLogger
	kycGate           service.KYCGate
	ledgerRepo        repository.BalanceLedgerRepository
	outboxWriter      outbox.Writer
	fileStore         storage.FileStore
}

// appHandlers groups all route handlers; fields are unexported and used only within Routes.
type appHandlers struct {
	match              *handler.MatchHandler
	prediction         *handler.PredictionHandler
	group              *handler.GroupHandler
	leaderboard        *handler.LeaderboardHandler
	userStats          *handler.UserStatsHandler
	tiebreaker         *handler.TiebreakerHandler
	tournament         *handler.TournamentHandler
	balance            *handler.BalanceHandler
	bankTransfer       *handler.BankTransferHandler
	withdrawal         *handler.WithdrawalHandler
	paymentIntent      *handler.PaymentIntentHandler
	paymentWebhook     *handler.PaymentWebhookHandler
	notification       *handler.NotificationHandler
	adminUser          *handler.AdminUserHandler
	adminGroup         *handler.AdminGroupHandler
	adminPayment       *handler.AdminPaymentHandler
	adminLeaderboard   *handler.AdminLeaderboardHandler
	adminDLQ           *handler.AdminDLQHandler
	adminAudit         *handler.AdminAuditHandler
	adminParam         *handler.AdminSystemParamHandler
	adminTiebreaker    *handler.AdminTiebreakerHandler
	adminConflict      *handler.AdminConflictHandler
	adminStats         *handler.AdminStatsHandler
	adminScoringRules  *handler.AdminScoringRuleHandler
	adminNotifTemplate *handler.AdminNotificationTemplateHandler
	adminNotifDLQ      *handler.AdminNotificationDLQHandler
	adminSSEStats      *handler.AdminSSEStatsHandler
	adminObservability *handler.AdminObservabilityHandler
	adminObsCircuit    *handler.AdminCircuitBreakersHandler
	adminObsDLQ        *handler.AdminObservabilityDLQHandler
	adminN8n           *handler.AdminN8nHandler
	kyc                *handler.KYCHandler
	adminKYC           *handler.AdminKYCHandler
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

	// Wrap the Redis cache with a circuit breaker so that repeated Redis errors
	// (network partition, OOM kill) degrade cache operations silently to miss/no-op,
	// keeping the service layer functional against PostgreSQL. The wrapping happens
	// here, before any service that uses s.cache, so all decorators share the same
	// resilient view.
	cacheStore := s.buildResilientCache(ctx, params)

	auditSvc := service.NewAuditService(auditLogRepo, auditTimeout, s.log)
	// Store auditSvc on the server so the shutdown path can call Drain() to
	// wait for in-flight audit writes before closing the database pool.
	s.auditSvc = auditSvc

	// Re-wire paramSvc with the now-available audit service so that Set/BulkSet
	// calls from admin handlers are recorded in the audit trail, and with a
	// history repository so every mutation appends to system_params_history.
	paramHistoryRepo := repository.NewPostgresSystemParamHistoryRepository(s.db)
	paramSvcWithAudit := service.NewSystemParamService(repos.sysParam, auditSvc, s.log,
		service.WithParamHistory(paramHistoryRepo),
	)

	matchSvc := service.NewMatchService(repos.match, s.bus, scorer, auditSvc, s.log)
	if cacheStore != nil {
		matchSvc = service.NewCachedMatchService(matchSvc, cacheStore, matchTTL, s.log)
	}

	outboxWriter := outbox.NewWriter(s.db)

	predSvc := service.NewPredictionService(repos.pred, repos.match, params, clock.Real{}, s.log)
	groupAuthz := service.NewGroupAuthzService(repos.member)
	quinielaSvc := service.NewQuinielaService(quinielaRepo, groupAuthz, params, auditSvc, randcode.Crypto{})
	paymentSvc := service.NewPaymentService(paymentRepo, auditSvc, s.log)
	memberSvc := service.NewGroupMembershipService(quinielaRepo, repos.member, params, auditSvc, paymentSvc, s.log,
		service.WithGroupMembershipOutboxWriter(outboxWriter))

	ranker := service.NewRankingService(quinielaRepo, repos.pred, repos.user, repos.member, tiebreakerRepo, tiebreakerConfigRepo, s.log)
	if cacheStore != nil {
		cachedRanker := service.NewCachedRankingService(ranker, cacheStore, leaderboardTTL, s.log)
		s.wireLeaderboardTTLHook(paramSvcWithAudit, cachedRanker)
		ranker = cachedRanker
	}

	userStatsSvc := service.NewUserStatsService(repos.pred)
	tiebreakerSvc := service.NewTiebreakerService(tiebreakerConfigRepo, groupAuthz, tiebreakerRepo, auditSvc, s.log)
	tournamentSvc := service.NewTournamentService(repos.match, tournamentRepo, params, auditSvc, s.log)
	snapshotter := service.NewLeaderboardSnapshotService(ranker, snapRepo)
	adminGroupSvc := service.NewAdminGroupService(quinielaRepo, repos.member, snapshotter, ranker, auditSvc, s.log)
	adminUserSvc := service.NewAdminUserService(repos.user, repos.member, paymentRepo, auditSvc, s.log)
	adminReadSvc := service.NewAdminReadService(
		service.AdminReadRepos{
			Pred: repos.pred, User: repos.user, Quiniela: quinielaRepo,
			Payment: paymentRepo, Tiebreaker: tiebreakerRepo, Snapshot: snapRepo,
			GlobalCache: cacheStore,
		},
		params, s.log,
	)
	conflictSvc := service.NewConflictService(quinielaRepo, repos.member, paymentRepo, params, auditSvc, s.log)
	scoringRuleSvc := service.NewScoringRuleService(scoringRuleRepo, auditSvc, s.log)

	dlqSvc := s.dlqSvc
	if dlqSvc == nil {
		dlqSvc = service.NoopDLQService{}
	}

	fileStore, err := storage.New(ctx, storage.Config{
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
		fileStore, _ = storage.New(ctx, storage.Config{Driver: "local", LocalDir: "uploads"})
	}
	// Wrap the file store with a circuit breaker. Failure threshold and cooldown
	// are read from system_params at startup (is_runtime=FALSE: restart required).
	// The breaker name includes the active driver so per-backend metrics and alert
	// labels are unambiguous when the storage driver is changed between deploys.
	fileBreakerName := "file-store-" + s.cfg.Storage.Driver
	fileStoreBreaker := breaker.New(
		fileBreakerName,
		params.GetInt(ctx, domain.ParamKeyBreakerFileStoreMaxFails, domain.DefaultBreakerFileStoreMaxFails),
		time.Duration(params.GetInt(ctx, domain.ParamKeyBreakerFileStoreCooldownSec, domain.DefaultBreakerFileStoreCooldownSec))*time.Second,
	)
	if s.notifier != nil {
		fileStoreBreaker.SetOnStateChange(func(name string, from, to breaker.State, openedAt time.Time) {
			if to == breaker.StateOpen {
				s.notifier.NotifyCircuitBreakerOpen(context.Background(), name, to.String(), openedAt)
			}
		})
	}
	resiStore := storage.NewResilientFileStore(fileStore, fileStoreBreaker, s.log)
	fileStore = resiStore
	if err := breaker.RegisterGauge(otel.GetMeterProvider().Meter("wcq"), fileStoreBreaker); err != nil {
		s.log.Warn("breaker.RegisterGauge(file-store) failed", zap.Error(err))
	}
	// Register a health checker so /health/ready reflects live storage state.
	// The checker bypasses the circuit breaker to probe the provider directly
	// when the circuit is closed, and reports "degraded" immediately when open.
	s.checkers = append(s.checkers, storage.NewChecker(fileBreakerName, resiStore))
	maxUploadBytes := int64(params.GetInt(ctx, domain.ParamKeyPaymentMaxUploadBytes, domain.DefaultPaymentMaxUploadBytes))
	minTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMinAmountCents, domain.DefaultBankTransferMinAmountCents)
	maxTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMaxAmountCents, domain.DefaultBankTransferMaxAmountCents)

	tmplCacheTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyTemplateCacheTTLSec, domain.DefaultNotifyTemplateCacheTTLSec)) * time.Second
	tmplRepo := repository.NewPostgresNotificationTemplateRepository(s.db, tmplCacheTTL)
	notifRepo := repository.NewPostgresUserNotificationRepository(s.db)
	prefRepo := repository.NewPostgresNotificationPreferenceRepository(s.db)
	pushRepo := repository.NewPostgresPushSubscriptionRepository(s.db)

	balanceSvc := service.NewBalanceService(repos.user, ledgerRepo, s.log)
	kycGate := service.NewKYCGate(repos.user, paramSvcWithAudit)
	bankTransferSvc := service.NewBankTransferService(proofRepo, kycGate, outboxWriter, auditSvc, s.log)
	// prizeSvc is constructed after kycSvc (below). The variable is declared here
	// so that any future handler needing prize credits can reference it.
	var prizeSvc service.PrizeCrediter
	paymentIntentSvc := service.NewPaymentIntentService(intentRepo, params, s.log)
	webhookPaymentSvc := service.NewWebhookPaymentService(ledgerRepo, intentRepo, auditSvc, s.log)
	withdrawalSvc := service.NewWithdrawalService(withdrawalRepo, repos.sysParam, kycGate, outboxWriter, auditSvc, s.log)

	h := appHandlers{
		notification: handler.NewNotificationHandler(handler.NotificationHandlerConfig{
			NotifRepo:         notifRepo,
			PrefRepo:          prefRepo,
			PushRepo:          pushRepo,
			Hub:               s.notifHub,
			Params:            params,
			UnsubscribeSecret: s.cfg.Email.UnsubscribeSecret,
			Log:               s.log,
		}),
		match:              handler.NewMatchHandler(matchSvc, s.log),
		prediction:         handler.NewPredictionHandler(predSvc, s.log),
		group:              handler.NewGroupHandler(quinielaSvc, memberSvc, s.log),
		leaderboard:        handler.NewLeaderboardHandler(ranker, s.log),
		userStats:          handler.NewUserStatsHandler(userStatsSvc, s.log),
		tiebreaker:         handler.NewTiebreakerHandler(tiebreakerSvc, s.log),
		tournament:         handler.NewTournamentHandler(tournamentSvc, s.log),
		balance:            handler.NewBalanceHandler(balanceSvc, s.log),
		bankTransfer:       handler.NewBankTransferHandler(bankTransferSvc, fileStore, maxUploadBytes, minTransferCents, maxTransferCents, s.log),
		withdrawal:         handler.NewWithdrawalHandler(withdrawalSvc, s.log),
		paymentIntent:      handler.NewPaymentIntentHandler(paymentIntentSvc, s.log),
		paymentWebhook:     handler.NewPaymentWebhookHandler(webhookPaymentSvc, s.log),
		adminUser:          handler.NewAdminUserHandler(adminUserSvc, s.log),
		adminGroup:         handler.NewAdminGroupHandler(adminGroupSvc, params, s.log),
		adminPayment:       handler.NewAdminPaymentHandler(paymentSvc, s.log),
		adminLeaderboard:   handler.NewAdminLeaderboardHandler(adminReadSvc, params, s.log),
		adminDLQ:           handler.NewAdminDLQHandler(dlqSvc, s.log),
		adminAudit:         handler.NewAdminAuditHandler(auditSvc, s.log),
		adminParam:         handler.NewAdminSystemParamHandler(paramSvcWithAudit, s.log),
		adminTiebreaker:    handler.NewAdminTiebreakerHandler(adminReadSvc, s.log),
		adminConflict:      handler.NewAdminConflictHandler(conflictSvc, s.log),
		adminStats:         handler.NewAdminStatsHandler(adminReadSvc, s.log),
		adminScoringRules:  handler.NewAdminScoringRuleHandler(scoringRuleSvc, s.log),
		adminNotifTemplate: handler.NewAdminNotificationTemplateHandler(tmplRepo, s.log),
		adminNotifDLQ:      handler.NewAdminNotificationDLQHandler(repository.NewPostgresNotificationDLQRepository(s.db), s.log),
		adminSSEStats:      handler.NewAdminSSEStatsHandler(s.notifHub, s.log),
	}

	// ── Phase 9 observability handlers ───────────────────────────────────────
	notifDLQRepo := repository.NewPostgresNotificationDLQRepository(s.db)
	h.adminObsDLQ = handler.NewAdminObservabilityDLQHandler(dlqSvc, notifDLQRepo, s.log)

	var promQ handler.PromQuerier
	if s.cfg.Observability.PrometheusURL != "" {
		promQ = promclient.New(s.cfg.Observability.PrometheusURL)
	}
	var tempoQ handler.TempoQuerier
	if s.cfg.Observability.TempoURL != "" {
		tempoQ = tempoclient.New(s.cfg.Observability.TempoURL)
	}
	h.adminObservability = handler.NewAdminObservabilityHandler(promQ, tempoQ, s.notifHub, s.log)

	if s.breakerRegistry == nil {
		s.breakerRegistry = breaker.NewRegistry()
	}
	s.breakerRegistry.Register(fileStoreBreaker)
	h.adminObsCircuit = handler.NewAdminCircuitBreakersHandler(s.breakerRegistry, s.log)

	h.adminN8n = handler.NewAdminN8nHandler(s.cfg.N8n.BaseURL, s.cfg.N8n.APIKey, s.log)

	// ── KYC module ───────────────────────────────────────────────────────────
	h.kyc, h.adminKYC, prizeSvc = s.buildKYCModule(ctx, kycModuleDeps{
		params:            params,
		paramSvcWithAudit: paramSvcWithAudit,
		auditSvc:          auditSvc,
		kycGate:           kycGate,
		ledgerRepo:        ledgerRepo,
		outboxWriter:      outboxWriter,
		fileStore:         fileStore,
	})
	if pc, ok := adminGroupSvc.(interface{ SetPrizeCrediter(service.PrizeCrediter) }); ok {
		pc.SetPrizeCrediter(prizeSvc)
	}
	if wg, ok := webhookPaymentSvc.(interface{ SetKYCGate(service.KYCGate) }); ok {
		wg.SetKYCGate(kycGate)
	}

	// Wire observability notifier into payment-path handlers. Each handler
	// defines its own narrow interface so the import graph stays acyclic.
	if s.notifier != nil {
		h.bankTransfer.SetNotifier(s.notifier)
		h.paymentWebhook.SetNotifier(s.notifier)
		h.withdrawal.SetNotifier(s.notifier)
	}

	return h
}

// wireLeaderboardTTLHook registers a mutation hook so that when an admin
// changes cache.leaderboard_ttl_seconds, the active TTL is updated and all
// existing leaderboard cache entries are flushed immediately.
func (s *Server) wireLeaderboardTTLHook(paramSvc service.SystemParamService, ranker *service.CachedRankingService) {
	if mh, ok := paramSvc.(service.MutationHookRegisterer); ok {
		mh.RegisterMutationHook(domain.ParamKeyCacheLeaderboardTTL,
			leaderboardTTLHook(paramSvc, ranker))
	}
}

// buildKYCModule wires all KYC repositories, services, OTel instruments, and
// handlers, then returns the user-facing handler, the admin handler, and the
// prize-crediting service. Extracted from buildHandlers to keep its cognitive
// complexity within the project limit.
func (s *Server) buildKYCModule(ctx context.Context, deps kycModuleDeps) (*handler.KYCHandler, *handler.AdminKYCHandler, service.PrizeCrediter) {
	kycProfileRepo := repository.NewPostgresKYCProfileRepository(s.db)
	kycDocRepo := repository.NewPostgresKYCDocumentRepository(s.db)
	kycEventRepo := repository.NewPostgresKYCEventRepository(s.db)
	kycMaxUpload := int64(deps.params.GetInt(ctx, domain.ParamKeyKYCMaxDocUploadBytes, domain.DefaultKYCMaxDocUploadBytes))

	kycMetrics, err := service.RegisterKYCMetrics(otel.GetMeterProvider().Meter("wcq"), kycProfileRepo, kycProfileRepo)
	if err != nil {
		s.log.Warn("KYC OTel metrics registration failed", zap.Error(err))
		kycMetrics = nil
	}
	if cg, ok := deps.kycGate.(interface{ SetMetrics(*service.KYCMetrics) }); ok {
		cg.SetMetrics(kycMetrics)
	}
	if sl, ok := deps.kycGate.(interface {
		SetLedger(repository.BalanceLedgerRepository)
	}); ok {
		sl.SetLedger(deps.ledgerRepo)
	}
	if sp, ok := deps.kycGate.(interface {
		SetProfileRepo(repository.KYCProfileRepository)
	}); ok {
		sp.SetProfileRepo(kycProfileRepo)
	}

	kycSvc := service.NewKYCService(kycProfileRepo, kycDocRepo, kycEventRepo, deps.paramSvcWithAudit, deps.auditSvc, s.log, kycMetrics)
	if sc, ok := kycSvc.(interface{ SetCache(cache.Store) }); ok {
		sc.SetCache(s.cache)
	}
	if sl, ok := kycSvc.(interface {
		SetLedger(repository.BalanceLedgerRepository)
	}); ok {
		sl.SetLedger(deps.ledgerRepo)
	}
	if sg, ok := kycSvc.(interface {
		SetGate(service.KYCGate)
	}); ok {
		sg.SetGate(deps.kycGate)
	}

	return handler.NewKYCHandler(kycSvc, deps.fileStore, kycMaxUpload, s.log),
		handler.NewAdminKYCHandler(kycSvc, s.log),
		service.NewPrizeService(deps.ledgerRepo, deps.kycGate, kycSvc, deps.outboxWriter, s.notifier, s.log)
}

// buildResilientCache wraps s.cache with a circuit breaker when the underlying
// store is a *cache.RedisStore. If Redis is unavailable the breaker opens and
// all cache operations degrade to cache-miss / silent no-op, so the service
// layer continues to work directly against PostgreSQL.
//
// When s.cache is not a RedisStore (e.g. MemoryStore in tests or single-node
// deployments without Redis) the original store is returned unchanged.
func (s *Server) buildResilientCache(ctx context.Context, params service.SystemParamService) cache.Store {
	rs, ok := s.cache.(*cache.RedisStore)
	if !ok {
		return s.cache
	}

	cb := breaker.New(
		"redis-cache",
		params.GetInt(ctx, domain.ParamKeyBreakerCacheMaxFails, domain.DefaultBreakerCacheMaxFails),
		time.Duration(params.GetInt(ctx, domain.ParamKeyBreakerCacheCooldownSec, domain.DefaultBreakerCacheCooldownSec))*time.Second,
	)
	if s.notifier != nil {
		cb.SetOnStateChange(func(name string, from, to breaker.State, openedAt time.Time) {
			if to == breaker.StateOpen {
				s.notifier.NotifyCircuitBreakerOpen(context.Background(), name, to.String(), openedAt)
			}
		})
	}
	if err := breaker.RegisterGauge(otel.GetMeterProvider().Meter("wcq"), cb); err != nil {
		s.log.Warn("breaker.RegisterGauge(redis-cache) failed", zap.Error(err))
	}

	if s.breakerRegistry == nil {
		s.breakerRegistry = breaker.NewRegistry()
	}
	s.breakerRegistry.Register(cb)

	return cache.NewResilientStore(rs, cb, s.log)
}
