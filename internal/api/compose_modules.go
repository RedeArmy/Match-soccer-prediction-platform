package api

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/api/handler"
	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
)

// buildFXModule constructs the exchange-rate service, registers its OTel
// instruments, pre-warms its cache, and returns the two route handlers.
// Extracted from buildHandlers to keep that method within the cognitive-
// complexity ceiling.
func (s *Server) buildFXModule(
	ctx context.Context,
	params service.SystemParamService,
	audit service.AuditLogger,
) (*handler.AdminExchangeRateHandler, *handler.ExchangeRateHandler, service.ExchangeRateService) {
	fxRepo := repository.NewPostgresExchangeRateRepository(s.db)
	fxFetcher := service.NewMultiSourceFetcher(s.log,
		service.NewBanguatFetcher(),
		service.NewExchangeRateAPIFetcher(s.cfg.ExchangeRate.ExchangeRateAPIKey),
		service.NewOpenExchangeFetcher(s.cfg.ExchangeRate.OpenExchangeRatesAppID),
	)
	fxImpl := service.NewExchangeRateServiceImpl(fxFetcher, fxRepo, params, s.notifier, s.log)
	fxSvc := service.ExchangeRateService(fxImpl)
	if err := fxImpl.RegisterMetrics(otel.GetMeterProvider().Meter("wcq")); err != nil {
		s.log.Warn("exchange_rate: RegisterMetrics failed", zap.Error(err))
	}
	// Pre-warm the cache from the most recent history row so the first
	// GET /api/exchange-rate request is served from cache, not from the DB.
	if err := fxSvc.WarmCache(ctx); err != nil {
		s.log.Warn("exchange_rate: cache warm failed (non-fatal, will warm on first request)", zap.Error(err))
	}
	return handler.NewAdminExchangeRateHandler(fxSvc, fxRepo, audit, s.log),
		handler.NewExchangeRateHandler(fxSvc, s.log),
		fxSvc
}

// wireFXDependencies late-wires the exchange-rate service and user repository
// into the webhook payment, group membership, and prize services after the FX
// module has been constructed. Extracted from buildHandlers to keep its
// cognitive complexity within the allowed limit.
func wireFXDependencies(
	webhookSvc service.WebhookPaymentService,
	memberSvc service.GroupMembershipService,
	prizeSvc service.PrizeCrediter,
	userRepo repository.UserRepository,
	fxSvc service.ExchangeRateService,
) {
	if wfx, ok := webhookSvc.(interface {
		SetExchangeRateService(service.ExchangeRateService)
	}); ok {
		wfx.SetExchangeRateService(fxSvc)
	}
	if wur, ok := webhookSvc.(interface {
		SetUserRepository(repository.UserRepository)
	}); ok {
		wur.SetUserRepository(userRepo)
	}
	if ms, ok := memberSvc.(interface {
		SetFxSvc(repository.UserRepository, service.ExchangeRateService)
	}); ok {
		ms.SetFxSvc(userRepo, fxSvc)
	}
	if pf, ok := prizeSvc.(interface {
		SetFxSvc(repository.UserRepository, service.ExchangeRateService)
	}); ok {
		pf.SetFxSvc(userRepo, fxSvc)
	}
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

// wirePrizeMetrics registers the GroupPrizeMetrics OTel instruments and wires
// them into svc via SetPrizeMetrics. Extracted from buildHandlers to keep its
// cognitive complexity within the project limit.
func (s *Server) wirePrizeMetrics(svc service.AdminGroupService) {
	prizeMetrics, err := service.RegisterGroupPrizeMetrics(otel.GetMeterProvider().Meter("wcq"))
	if err != nil {
		s.log.Warn("RegisterGroupPrizeMetrics failed (prize distribution failures will not be counted)", zap.Error(err))
	}
	if pm, ok := svc.(interface {
		SetPrizeMetrics(*service.GroupPrizeMetrics)
	}); ok {
		pm.SetPrizeMetrics(prizeMetrics)
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
	if slg, ok := deps.kycGate.(interface{ SetLogger(*zap.Logger) }); ok {
		slg.SetLogger(s.log)
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
	if so, ok := kycSvc.(interface {
		SetOutboxWriter(outbox.Writer)
	}); ok && deps.outboxWriter != nil {
		so.SetOutboxWriter(deps.outboxWriter)
	}

	return handler.NewKYCHandler(kycSvc, deps.fileStore, kycMaxUpload, s.log),
		handler.NewAdminKYCHandler(kycSvc, s.log),
		service.NewPrizeService(deps.ledgerRepo, deps.kycGate, kycSvc, deps.outboxWriter, s.notifier, s.log)
}

// wirePaymentNotifiers attaches the observability notifier to the three
// payment-path handlers that expose a SetNotifier interface. Extracted from
// buildHandlers to keep its cognitive complexity within the allowed limit.
func (s *Server) wirePaymentNotifiers(h *appHandlers) {
	if s.notifier == nil {
		return
	}
	h.bankTransfer.SetNotifier(s.notifier)
	h.paymentWebhook.SetNotifier(s.notifier)
	h.withdrawal.SetNotifier(s.notifier)
}
