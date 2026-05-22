package api

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/clock"
	"github.com/rede/world-cup-quiniela/pkg/codegen"
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
	// calls from admin handlers are recorded in the audit trail, and with a
	// history repository so every mutation appends to system_params_history.
	paramHistoryRepo := repository.NewPostgresSystemParamHistoryRepository(s.db)
	paramSvcWithAudit := service.NewSystemParamService(repos.sysParam, auditSvc, s.log,
		service.WithParamHistory(paramHistoryRepo),
	)

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
	resiStore := storage.NewResilientFileStore(
		fileStore,
		breaker.New(
			"file-store",
			params.GetInt(ctx, domain.ParamKeyBreakerFileStoreMaxFails, domain.DefaultBreakerFileStoreMaxFails),
			time.Duration(params.GetInt(ctx, domain.ParamKeyBreakerFileStoreCooldownSec, domain.DefaultBreakerFileStoreCooldownSec))*time.Second,
		),
		s.log,
	)
	fileStore = resiStore
	// Register a health checker so /health/ready reflects live storage state.
	// The checker bypasses the circuit breaker to probe the provider directly
	// when the circuit is closed, and reports "degraded" immediately when open.
	s.checkers = append(s.checkers, storage.NewChecker("file-store", resiStore))
	maxUploadBytes := int64(params.GetInt(ctx, domain.ParamKeyPaymentMaxUploadBytes, domain.DefaultPaymentMaxUploadBytes))
	minTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMinAmountCents, domain.DefaultBankTransferMinAmountCents)
	maxTransferCents := params.GetInt(ctx, domain.ParamKeyBankTransferMaxAmountCents, domain.DefaultBankTransferMaxAmountCents)

	outboxWriter := outbox.NewWriter(s.db)

	tmplCacheTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyTemplateCacheTTLSec, domain.DefaultNotifyTemplateCacheTTLSec)) * time.Second
	tmplRepo := repository.NewPostgresNotificationTemplateRepository(s.db, tmplCacheTTL)
	notifRepo := repository.NewPostgresUserNotificationRepository(s.db)
	prefRepo := repository.NewPostgresNotificationPreferenceRepository(s.db)
	pushRepo := repository.NewPostgresPushSubscriptionRepository(s.db)

	balanceSvc := service.NewBalanceService(repos.user, ledgerRepo, s.log)
	bankTransferSvc := service.NewBankTransferService(proofRepo, outboxWriter, auditSvc, s.log)
	paymentIntentSvc := service.NewPaymentIntentService(intentRepo, params, s.log)
	webhookPaymentSvc := service.NewWebhookPaymentService(ledgerRepo, intentRepo, auditSvc, s.log)
	withdrawalSvc := service.NewWithdrawalService(withdrawalRepo, repos.sysParam, outboxWriter, auditSvc, s.log)

	return appHandlers{
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
}
