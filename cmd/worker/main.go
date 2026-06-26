// Command worker runs background event processing tasks for the World Cup
// quiniela application.
//
// The worker consumes domain events from the Redis Streams event bus and
// reacts to them asynchronously. It handles two event types:
//
//   - EventMatchStarted: emitted when an admin transitions a match to Live.
//     The handler emits a structured audit log entry confirming that the
//     prediction window is now closed. Prediction enforcement itself is
//     synchronous in PredictionService; this handler exists for observability.
//
//   - EventMatchFinished: emitted when an admin confirms a match result.
//     The handler calls ScoringService to calculate and persist points for
//     every prediction on that match. On transient failure the bus retries
//     and, if all attempts are exhausted, routes the event to the dead-letter
//     queue for manual replay.
//
// Running scoring in the worker rather than inside the API server prevents
// background CPU work from competing with HTTP handlers for goroutines and
// database connections, and lets the two components be scaled independently
// based on their respective load profiles.
//
// The worker requires the Redis event bus driver (WCQ_EVENTBUS_DRIVER=redis).
// Starting it with the in-memory driver is rejected at startup: in-memory
// events are not visible across process boundaries and the worker would
// never receive any events.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/election"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/storage"
	infrapush "github.com/rede/world-cup-quiniela/internal/infrastructure/webpush"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/internal/notification/dispatcher"
	"github.com/rede/world-cup-quiniela/internal/notification/outbox"
	"github.com/rede/world-cup-quiniela/internal/notification/scheduler"
	"github.com/rede/world-cup-quiniela/internal/observability"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/config"
	"github.com/rede/world-cup-quiniela/pkg/dsem"
	"github.com/rede/world-cup-quiniela/pkg/footballprovider"
	"github.com/rede/world-cup-quiniela/pkg/health"
	"github.com/rede/world-cup-quiniela/pkg/logger"
)

// redisPubNotifier implements dispatcher.PgNotifier using Redis Pub/Sub.
//
// Publishing to a Redis channel is preferred over pg_notify for cross-process
// SSE delivery because:
//   - Redis reconnects are transparent and nearly instant (< 100 ms) vs the
//     1 s–30 s exponential backoff of the pg_notify LISTEN bridge.
//   - One fewer long-lived PostgreSQL connection per worker replica.
//   - The payload is not limited to PostgreSQL's 8 kB NOTIFY ceiling.
//
// The API server's Redis bridge subscribes to the same channel and broadcasts
// to its in-process SSE hub. The PgNotifier interface is kept as-is so that
// no changes are required in the dispatcher layer.
type redisPubNotifier struct{ client redis.Cmdable }

func (n *redisPubNotifier) Notify(ctx context.Context, channel, payload string) error {
	if err := n.client.Publish(ctx, channel, payload).Err(); err != nil {
		return fmt.Errorf("redis publish %q: %w", channel, err)
	}
	return nil
}

// localSnapshotSem wraps semaphore.Weighted to satisfy dsem.Semaphore.
// Used when Redis is not available and the global concurrency limit is
// enforced per-process only.
type localSnapshotSem struct{ w *semaphore.Weighted }

func (l *localSnapshotSem) Acquire(ctx context.Context) error {
	if err := l.w.Acquire(ctx, 1); err != nil {
		return fmt.Errorf("snapshot semaphore: %w", err)
	}
	return nil
}
func (l *localSnapshotSem) Release() { l.w.Release(1) }

// redisSnapshotLocker implements SnapshotLocker using Redis SET NX EX.  The key
// pattern is "worker:snap:{matchID}:{quinielaID}".  The TTL is a safety net for
// crash recovery — the lock is released explicitly via Unlock in the happy path.
type redisSnapshotLocker struct {
	client redis.Cmdable
	ttl    time.Duration
}

func (l *redisSnapshotLocker) TryLock(ctx context.Context, matchID, quinielaID int) (bool, error) {
	key := fmt.Sprintf("worker:snap:%d:%d", matchID, quinielaID)
	ok, err := l.client.SetNX(ctx, key, 1, l.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("snapshot lock %q: %w", key, err)
	}
	return ok, nil
}

func (l *redisSnapshotLocker) Unlock(ctx context.Context, matchID, quinielaID int) error {
	key := fmt.Sprintf("worker:snap:%d:%d", matchID, quinielaID)
	if err := l.client.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("snapshot unlock %q: %w", key, err)
	}
	return nil
}

// dlqMonitorInterval controls how often the DLQ monitoring goroutine logs
// the dead-letter queue state. Five minutes is frequent enough to surface a
// stuck queue within a reasonable SLA without spamming logs during normal
// operation. Declared as a var so tests can reduce it to a short duration
// without modifying production code.
var dlqMonitorInterval = 5 * time.Minute

// purgeTickInterval controls how often the purge goroutine runs. Daily is
// sufficient: soft-deleted rows accumulate slowly and exact timing is not
// critical. Declared as a var so tests can inject a shorter interval or a
// pre-loaded channel without modifying production code.
var purgeTickInterval = 24 * time.Hour

// snapshotKeepLatestCount is the number of most-recent leaderboard snapshots
// to retain per quiniela during each purge run. Overridden at startup from
// system_params (snapshot.keep_latest_count). Declared as a var so tests can
// set it without modifying production code.
var snapshotKeepLatestCount = domain.DefaultSnapshotKeepLatestCount

func main() {
	cfg, err := config.LoadWorker()
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	log, err := logger.New(logger.Config{
		Level:    cfg.Logger.Level,
		Encoding: cfg.Logger.Encoding,
		InitialFields: []zap.Field{
			zap.String("service", cfg.Tracing.ServiceName+"-worker"),
			zap.String("env", cfg.Environment),
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger error: %v\n", err)
		os.Exit(1)
	}
	defer log.Sync() //nolint:errcheck

	// Wire the defensive logger for repository deferred rollback failures.
	repository.SetDefensiveLogger(log)

	logStartupBanner(cfg, log)

	for _, w := range config.Warnings(cfg) {
		log.Warn(w)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, log); err != nil {
		log.Sugar().Fatalf("worker: %v", err)
	}
}

// run bootstraps worker dependencies and starts the event loop. Extracted from
// main so tests can exercise the full startup path with a cancellable context.
// Redis is opened before the database: an unreachable event bus makes the worker
// useless regardless of DB state, so failing fast on it produces the clearer error.
func run(ctx context.Context, cfg *config.Config, log *zap.Logger) error {
	shutdownTracing, err := setupTracing(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("tracing: %w", err)
	}
	defer flushShutdown(ctx, shutdownTracing, "tracing", log)

	meter, metricsHandler, shutdownMetrics, err := setupMetrics(cfg, log)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	log = wireLogLevelCounters(log, meter)
	defer flushShutdown(ctx, shutdownMetrics, "metrics", log)

	// Validate the event bus driver before establishing any connections.
	// Failing here surfaces a misconfiguration error without incurring the
	// latency of any dial that would ultimately be useless.
	if cfg.EventBus.Driver != "redis" {
		return fmt.Errorf(
			"worker requires eventBus.driver=redis (got %q); set WCQ_EVENTBUS_DRIVER=redis",
			cfg.EventBus.Driver,
		)
	}

	bus, closeBus, err := setupEventBus(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("event bus: %w", err)
	}
	defer closeBus()

	db, err := setupDB(ctx, cfg, log)
	if err != nil {
		return fmt.Errorf("database: %w", err)
	}
	defer db.Close()

	// Wire the Postgres DLQ fallback before any Subscribe calls so that events
	// dead-lettered during match scoring are never silently lost even when
	// Redis itself is unavailable. (ATD-007)
	wirePgDLQFallback(bus, db, log)

	matchRepo := repository.NewPostgresMatchRepository(db)
	predRepo := repository.NewPostgresPredictionRepository(db)
	systemParamRepo := repository.NewPostgresSystemParamRepository(db)
	params := service.NewSystemParamService(systemParamRepo, nil, log)
	messaging.Configure(
		params.GetInt(ctx, domain.ParamKeyMessagingMaxRetries, domain.DefaultMessagingMaxRetries),
		int64(params.GetInt(ctx, domain.ParamKeyMessagingStreamMaxLen, domain.DefaultMessagingStreamMaxLen)),
		params.GetInt(ctx, domain.ParamKeyMessagingStreamWorkerCount, domain.DefaultMessagingStreamWorkerCount),
		params.GetInt(ctx, domain.ParamKeyMessagingStreamReadBlockSec, domain.DefaultMessagingStreamReadBlockSec),
		nil,
	)
	service.ConfigureAuditRetry(
		params.GetInt(ctx, domain.ParamKeyAuditMaxRetries, domain.DefaultAuditMaxRetries),
		params.GetInt(ctx, domain.ParamKeyAuditRetryDelayMs, domain.DefaultAuditRetryDelayMs),
	)
	service.ConfigureAuditMaxInFlight(
		params.GetInt(ctx, domain.ParamKeyAuditMaxInFlight, domain.DefaultAuditMaxInFlight),
	)
	snapshotConcurrency := params.GetInt(ctx, domain.ParamKeyWorkerSnapshotConcurrency, domain.DefaultWorkerSnapshotConcurrency)
	snapshotRetryBase := time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSnapshotRetryBaseMs, domain.DefaultWorkerSnapshotRetryBaseMs)) * time.Millisecond
	maxSnapshotAttempts := params.GetInt(ctx, domain.ParamKeyWorkerSnapshotMaxAttempts, domain.DefaultWorkerSnapshotMaxAttempts)
	dlqMonitorInterval = time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerDLQMonitorIntervalSec, domain.DefaultWorkerDLQMonitorIntervalSec)) * time.Second
	purgeTickInterval = time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerPurgeIntervalHours, domain.DefaultWorkerPurgeIntervalHours)) * time.Hour
	snapshotKeepLatestCount = params.GetInt(ctx, domain.ParamKeySnapshotKeepLatestCount, domain.DefaultSnapshotKeepLatestCount)
	ruleRepo := repository.NewPostgresScoringRuleRepository(db)
	scorer := service.NewScoringService(matchRepo, predRepo, ruleRepo, params, log,
		service.WithScoringMeter(meter),
	)

	// Match sync — build the provider client only when the API key is present.
	// When absent, matchSyncSvc.PollAndApply returns an error and the scheduler
	// job logs a warning and continues; sync is effectively disabled.
	var fpClient footballprovider.Client
	if cfg.FootballProvider.APIKey != "" {
		fpClient = footballprovider.NewAPIFootballClient(
			cfg.FootballProvider.APIKey,
			cfg.FootballProvider.BaseURL,
			nil,
		)
	}
	matchSvc := service.NewMatchService(matchRepo, bus, scorer, service.NoopAuditLogger{}, log)
	matchSyncSvc := service.NewMatchSyncService(matchRepo, matchSvc, fpClient, log)
	tournamentRepo := repository.NewPostgresTournamentRepository(db)
	teamRepo := repository.NewPostgresTeamRepository(db)
	tournamentSvc := service.NewTournamentService(matchRepo, tournamentRepo, teamRepo, params, service.NoopAuditLogger{}, log)

	quinielaRepo := repository.NewPostgresQuinielaRepository(db, repository.WithQuinielaLogger(log))
	memberRepo := repository.NewPostgresGroupMembershipRepository(db)
	userRepo := repository.NewPostgresUserRepository(db)
	tiebreakerRepo := repository.NewPostgresTiebreakerRepository(db)
	tiebreakerConfigRepo := repository.NewPostgresTiebreakerConfigRepository(db)
	snapRepo := repository.NewPostgresLeaderboardSnapshotRepository(db)
	ranker := service.NewRankingService(quinielaRepo, predRepo, userRepo, memberRepo, tiebreakerRepo, tiebreakerConfigRepo, log)
	snapshotter := service.NewLeaderboardSnapshotService(ranker, snapRepo)

	// A dedicated Redis client for health checks avoids sharing connections
	// with the event bus, whose long-lived XREADGROUP calls would otherwise
	// inflate the apparent latency of a ping. The same client is reused for
	// cache invalidation: DEL and SCAN+DEL are short-lived commands that do
	// not interfere with health-ping latency.
	rc := redis.NewClient(&redis.Options{
		Addr:     cfg.Redis.Addr,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	})
	defer rc.Close() //nolint:errcheck
	logWarnOnErr(log, "worker: redisotel: tracing instrumentation failed", redisotel.InstrumentTracing(rc))

	// snapshotSem: cluster-wide distributed semaphore via Redis.
	// The worker validates WCQ_EVENTBUS_DRIVER=redis at startup so rc is always
	// non-nil by the time we reach this point.
	const snapshotSemKey = "worker:snapshot:sem"
	const snapshotSemTTL = 5 * time.Minute
	snapshotSem := dsem.New(rc, snapshotSemKey, int64(snapshotConcurrency), snapshotSemTTL)
	log.Info("worker: snapshot semaphore: distributed Redis semaphore active",
		zap.String("key", snapshotSemKey),
		zap.Int("limit", snapshotConcurrency),
	)

	cacheStore := cache.NewRedisStore(rc)
	logWarnOnErr(log, "worker: cacheStore.RegisterMetrics failed", cacheStore.RegisterMetrics(meter))
	invalidators := []service.PostScoringInvalidator{
		service.NewPostScoringCacheFlush(cacheStore, log),
	}

	snapCfg := snapshotConfig{
		concurrency: snapshotConcurrency,
		retryBase:   snapshotRetryBase,
		maxAttempts: maxSnapshotAttempts,
		sem:         snapshotSem,
	}

	// Broadcast a leaderboard.updated SSE signal after scoring.
	// rc is always non-nil here: redis.NewClient never returns nil and the
	// worker rejects non-redis event bus drivers before reaching this point.
	// Concurrency matches the snapshot pool so SSE fan-out and DB fan-out share
	// the same budget, keeping total DB connections predictable under load.
	lbPublishMaxAttempts := params.GetInt(ctx, domain.ParamKeyWorkerLeaderboardPublishMaxAttempts, domain.DefaultWorkerLeaderboardPublishMaxAttempts)
	lbPublishBaseDelay := time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerLeaderboardPublishBaseDelayMs, domain.DefaultWorkerLeaderboardPublishBaseDelayMs)) * time.Millisecond
	broadcaster := LeaderboardBroadcaster(&redisPubLeaderboardBroadcaster{
		client:      rc,
		memberRepo:  memberRepo,
		concurrency: snapshotConcurrency,
		maxAttempts: lbPublishMaxAttempts,
		baseDelay:   lbPublishBaseDelay,
		log:         log,
	})

	purger := repository.NewPostgresPurger(db)
	retentionDays := params.GetInt(ctx, domain.ParamKeyPurgeRetentionDays, domain.DefaultPurgeRetentionDays)
	purgeRetention := time.Duration(retentionDays) * 24 * time.Hour
	paramHistoryRetentionDays := params.GetInt(ctx, domain.ParamKeySystemParamHistoryRetentionDays, domain.DefaultSystemParamHistoryRetentionDays)
	paramHistoryRetention := time.Duration(paramHistoryRetentionDays) * 24 * time.Hour
	fxHistoryRetentionDays := params.GetInt(ctx, domain.ParamKeyFXHistoryRetentionDays, domain.DefaultFXHistoryRetentionDays)
	fxHistoryRetention := time.Duration(fxHistoryRetentionDays) * 24 * time.Hour
	outboxRetentionDays := params.GetInt(ctx, domain.ParamKeyOutboxRetentionDays, domain.DefaultOutboxRetentionDays)
	outboxRetention := time.Duration(outboxRetentionDays) * 24 * time.Hour
	intentRetentionDays := params.GetInt(ctx, domain.ParamKeyPaymentIntentRetentionDays, domain.DefaultPaymentIntentRetentionDays)
	intentExpiredRetention := time.Duration(intentRetentionDays) * 24 * time.Hour

	// Leader election for the DLQ monitor via a PostgreSQL session-level
	// advisory lock. A 15-second timeout is added to ctx to bound the
	// connection handshake without preventing the lifecycle context from
	// cancelling the attempt if a shutdown signal arrives during startup.
	electionCtx, electionCancel := context.WithTimeout(ctx, 15*time.Second)
	defer electionCancel()
	dlqElection, err := election.NewPgLeaderElection(electionCtx, cfg.Database.DSN, dlqMonitorLockID, log)
	if err != nil {
		if ctx.Err() != nil {
			return nil // lifecycle context cancelled during startup — treat as clean shutdown
		}
		return fmt.Errorf("leader election: %w", err)
	}

	// ── Notification subsystem ────────────────────────────────────────────────

	outboxBatchSize := params.GetInt(ctx, domain.ParamKeyNotifyOutboxBatchSize, domain.DefaultNotifyOutboxBatchSize)
	outboxPollInterval := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxPollIntervalSec, domain.DefaultNotifyOutboxPollIntervalSec)) * time.Second
	outboxLockDuration := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxLockDurationSec, domain.DefaultNotifyOutboxLockDurationSec)) * time.Second
	outboxMaxAttempts := params.GetInt(ctx, domain.ParamKeyNotifyOutboxMaxAttempts, domain.DefaultNotifyOutboxMaxAttempts)
	outboxLagThreshold := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxLagAlertThresholdSec, domain.DefaultNotifyOutboxLagAlertThresholdSec)) * time.Second
	outboxLagCriticalThreshold := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxLagCriticalSec, domain.DefaultNotifyOutboxLagCriticalSec)) * time.Second

	staleLockThreshold := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxStaleLockThresholdSec, domain.DefaultNotifyOutboxStaleLockThresholdSec)) * time.Second
	outboxRepo := outbox.NewPostgresRepository(db, outbox.WithStaleLockThreshold(staleLockThreshold))
	outboxWriter := outbox.NewWriter(db)

	adminLogRepo := repository.NewPostgresAdminNotificationLogRepository(db)
	dlqRepo := repository.NewPostgresNotificationDLQRepository(db)

	mailer, mailerErr := buildMailer(cfg.Email.ResendAPIKey)
	logOrFatal(cfg, log, mailerErr, "worker: email sender degraded", "worker: email sender not configured for production")

	adminDispatcher := dispatcher.NewAdminDispatcher(dispatcher.Config{
		Params:    params,
		LogRepo:   adminLogRepo,
		DLQRepo:   dlqRepo,
		Mailer:    mailer,
		FromAddr:  cfg.Email.FromAddress,
		N8nURL:    cfg.N8n.WebhookURL,
		N8nSecret: cfg.N8n.WebhookSecret,
		Log:       log,
	})

	// Phase 2: User-facing in-app dispatcher with SSE/push/email channels.
	notifRepo := repository.NewPostgresUserNotificationRepository(db)
	prefRepo := repository.NewPostgresNotificationPreferenceRepository(db)
	pushRepo := repository.NewPostgresPushSubscriptionRepository(db)

	vapidPublicKey := params.GetString(ctx, domain.ParamKeyNotifyWebPushVAPIDPublicKey, "")
	vapidPrivateKey := cfg.WebPush.VAPIDPrivateKey
	vapidSubject := params.GetString(ctx, domain.ParamKeyNotifyWebPushVAPIDSubject, "")

	pusher, pusherErr := buildPusher(vapidPublicKey, vapidPrivateKey, vapidSubject)
	logOrFatal(cfg, log, pusherErr, "worker: push sender degraded", "worker: push sender not configured for production")

	tmplCacheTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyTemplateCacheTTLSec, domain.DefaultNotifyTemplateCacheTTLSec)) * time.Second
	tmplRepo := repository.NewPostgresNotificationTemplateRepository(db, tmplCacheTTL)

	digestWindowSec := int64(params.GetInt(ctx, domain.ParamKeyNotifyPushDigestWindowSec, domain.DefaultNotifyPushDigestWindowSec))
	digestThreshold := int32(params.GetInt(ctx, domain.ParamKeyNotifyPushDigestThreshold, domain.DefaultNotifyPushDigestThreshold))

	// The worker rejects non-redis event bus drivers before reaching this point,
	// so rc is always non-nil here. The cluster-aware digest gate enforces the
	// per-user push threshold across all worker replicas via Redis.
	digestGate := notification.Recorder(notification.NewRedisPushDigestGate(rc, digestWindowSec, digestThreshold))

	userDispatcher := dispatcher.NewUserDispatcher(dispatcher.UserDispatcherConfig{
		NotifRepo:         notifRepo,
		PrefRepo:          prefRepo,
		PushRepo:          pushRepo,
		DLQRepo:           dlqRepo,
		Hub:               nil, // hub lives in the API server; cross-process delivery via pg_notify
		Pusher:            pusher,
		Mailer:            mailer,
		EmailResolver:     &repoEmailResolver{userRepo: userRepo},
		FromAddr:          cfg.Email.FromAddress,
		UnsubscribeSecret: cfg.Email.UnsubscribeSecret,
		AppBaseURL:        cfg.Server.AppBaseURL,
		PgNotifier:        &redisPubNotifier{client: rc},
		Params:            params,
		TemplateRepo:      tmplRepo,
		Recorder:          digestGate,
		Log:               log,
	})

	compositeDispatcher := dispatcher.NewCompositeDispatcher(adminDispatcher, userDispatcher)

	// Register OTel instruments for the dispatchers and outbox backlog.
	logWarnOnErr(log, "adminDispatcher.RegisterMetrics failed", adminDispatcher.RegisterMetrics(meter))
	logWarnOnErr(log, "userDispatcher.RegisterMetrics failed", userDispatcher.RegisterMetrics(meter))
	logWarnOnErr(log, "outbox.RegisterPendingGauge failed", outbox.RegisterPendingGauge(meter, outboxRepo))
	logWarnOnErr(log, "outbox.RegisterDLQDepthGauge failed", outbox.RegisterDLQDepthGauge(meter, dlqRepo))
	logWarnOnErr(log, "outbox.RegisterOldestPendingAgeGauge failed", outbox.RegisterOldestPendingAgeGauge(meter, outboxRepo))

	// ATD-002: balance_ledger growth monitoring.  The gauge uses pg_class.reltuples
	// (fast, ±5% accurate) so it adds negligible DB load on every Prometheus scrape.
	// WCQLedgerRowCountWarning fires at 2M rows; the partitioning plan in
	// docs/adr/0008-balance-ledger-partitioning.md triggers at ~50M rows.
	ledgerRepo := repository.NewPostgresBalanceLedgerRepository(db)
	logWarnOnErr(log, "repository.RegisterLedgerRowCountGauge failed", repository.RegisterLedgerRowCountGauge(meter, ledgerRepo))
	intentRepo := repository.NewPostgresPaymentIntentRepository(db)

	// Scoring DLQ depth — exposes wcq_scoring_dlq_depth{event_type=...} so
	// Prometheus can alert when scoring events accumulate in the Redis DLQ.
	// The monitorDLQ goroutine already logs non-zero depths; this metric adds
	// independent Prometheus visibility for the WCQScoringDLQNonEmpty alert.
	logWarnOnErr(log, "register scoring DLQ gauge failed", registerScoringDLQGauge(meter, rc))

	dlqReplayWorker := outbox.NewDLQWorker(dlqRepo, outboxWriter, log,
		outbox.WithDLQBatchSize(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayBatchSize, domain.DefaultNotifyDLQReplayBatchSize)),
		outbox.WithDLQPollInterval(time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayPollIntervalSec, domain.DefaultNotifyDLQReplayPollIntervalSec))*time.Second),
		outbox.WithDLQMaxAttempts(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayMaxAttempts, domain.DefaultNotifyDLQReplayMaxAttempts)),
		outbox.WithDLQWarningThreshold(int64(params.GetInt(ctx, domain.ParamKeyNotifyDLQWarningThreshold, domain.DefaultNotifyDLQWarningThreshold))),
		outbox.WithDLQAlertThreshold(int64(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayAlertThreshold, domain.DefaultNotifyDLQReplayAlertThreshold))),
		outbox.WithDLQNotifier(setupObservabilityNotifier(cfg, log)))

	notifScheduler := buildNotifScheduler(ctx, cfg.Storage, params, db, outboxWriter, pushRepo, log)

	// Automated exchange rate refresh — Banguat publishes at ~10:00 AM Guatemala
	// time; 10:30 gives a 30-minute buffer for the feed to stabilise.
	// Falls back to ExchangeRate-API and OpenExchangeRates when Banguat is down.
	// On all-source failure: stale fallback + n8n /webhook/fx-rate-stale alert.
	fxRepo := repository.NewPostgresExchangeRateRepository(db)
	// FX source timeouts are is_runtime=FALSE: read once at worker startup;
	// changing them requires a worker restart.
	fxBanguatTimeout := time.Duration(params.GetInt(ctx, domain.ParamKeyFXBanguatTimeoutSec, domain.DefaultFXBanguatTimeoutSec)) * time.Second
	fxExchangeRateAPITimeout := time.Duration(params.GetInt(ctx, domain.ParamKeyFXExchangeRateAPITimeoutSec, domain.DefaultFXExchangeRateAPITimeoutSec)) * time.Second
	fxOpenExchangeTimeout := time.Duration(params.GetInt(ctx, domain.ParamKeyFXOpenExchangeTimeoutSec, domain.DefaultFXOpenExchangeTimeoutSec)) * time.Second
	fxFetcher := service.NewMultiSourceFetcher(log,
		service.NewBanguatFetcherWithTimeout(fxBanguatTimeout),
		service.NewExchangeRateAPIFetcherWithTimeout(cfg.ExchangeRate.ExchangeRateAPIKey, fxExchangeRateAPITimeout),
		service.NewOpenExchangeFetcherWithTimeout(cfg.ExchangeRate.OpenExchangeRatesAppID, fxOpenExchangeTimeout),
	)
	fxNotifier := setupObservabilityNotifier(cfg, log)
	fxImpl := service.NewExchangeRateServiceImpl(fxFetcher, fxRepo, params, fxNotifier, log)
	if err := fxImpl.RegisterMetrics(meter); err != nil {
		log.Warn("exchange_rate: RegisterMetrics failed", zap.Error(err))
	}
	notifScheduler.RegisterDaily("payment.exchange_rate_refresh", 10, 30,
		func(ctx context.Context) error {
			_, err := fxImpl.RefreshRate(ctx)
			return err
		})

	// Automated match result sync — polls API-Football for live scores and
	// transitions match state automatically.
	//
	// Two intervals are used:
	//   fast = match.sync.fast_poll_interval_sec (default 30 s) — while live matches exist
	//   slow = match.sync.slow_poll_interval_sec (default 300 s) — no live matches
	//
	// The scheduler only supports a fixed interval per job. We register the job
	// at the fast interval and guard the body with a runtime-readable enabled flag
	// so operators can disable sync without restarting the worker.
	// The fast/slow distinction is handled inside the closure: on a "slow tick"
	// the job body reads the last-observed live count from a package-level int32
	// (atomic) and skips the provider call when it is zero and fewer than
	// slowPollIntervalSec seconds have elapsed since the last successful call.
	matchSyncFastSec := params.GetInt(ctx, domain.ParamKeyMatchSyncFastPollIntervalSec, domain.DefaultMatchSyncFastPollIntervalSec)
	notifScheduler.RegisterInterval("match.sync",
		time.Duration(matchSyncFastSec)*time.Second,
		makeMatchSyncJob(params, matchSyncSvc, matchRepo, log))
	// Daily fixture sync — validates all linked scheduled/live matches against
	// API-Football once per day at match.dailysync.hour (Guatemala time).
	// On failure an n8n alert is fired; the job does not retry until tomorrow.
	dailySyncHour := params.GetInt(ctx, domain.ParamKeyMatchDailySyncHour, domain.DefaultMatchDailySyncHour)
	dailySyncNotifier := setupObservabilityNotifier(cfg, log)
	notifScheduler.RegisterDaily("match.daily_fixture_sync", dailySyncHour, 0,
		makeDailyFixtureSyncJob(params, matchSyncSvc, dailySyncNotifier, log))

	// Auto-cancel pending Recurrente payment intents older than 1 hour.
	// Runs every 5 minutes; the cutoff is computed fresh on each tick.
	notifScheduler.RegisterInterval("payment.cancel_stale_recurrente", 5*time.Minute,
		makeCancelStaleRecurrenteJob(intentRepo, log))

	// snapshotLockTTL covers the worst-case snapshot retry window with generous
	// headroom. The lock is also released explicitly by Unlock in the happy path;
	// the TTL only activates on process crash or context cancellation.
	const snapshotLockTTL = 2 * time.Minute

	// The scheduler health checker acts as a dead-man's switch: if any job has
	// not fired within 3× its expected interval the worker readiness probe fails,
	// triggering an on-call alert. A threshold of 3 tolerates two missed ticks
	// (e.g. brief Redis unavailability causing leader election to abstain) before
	// the probe degrades. See ADR 0002 for the synthetic-vs-persisted distinction.
	checkers := buildHealthCheckers(db, rc)
	checkers = append(checkers, notifScheduler.HealthChecker(3.0))
	logWarnOnErr(log, "scheduler: RegisterMetrics failed", notifScheduler.RegisterMetrics(meter, 3.0))

	return startWorker(ctx, workerDeps{
		cfg:            cfg,
		bus:            bus,
		scorer:         scorer,
		autoConfirmer:  tournamentSvc,
		snapshotter:    snapshotter,
		predRepo:       predRepo,
		invalidators:   invalidators,
		broadcaster:    broadcaster,
		snapshotLocker: &redisSnapshotLocker{client: rc, ttl: snapshotLockTTL},
		snapshotCfg:    snapCfg,
		purger:         purger,
		purgePol: purgePolicy{
			retention:              purgeRetention,
			paramHistoryRetention:  paramHistoryRetention,
			fxHistoryRetention:     fxHistoryRetention,
			outboxRetention:        outboxRetention,
			snapshotKeepCount:      snapshotKeepLatestCount,
			intentExpiredRetention: intentExpiredRetention,
		},
		rc:                         rc,
		checkers:                   checkers,
		metricsHandler:             metricsHandler,
		dlqElection:                dlqElection,
		outboxRepo:                 outboxRepo,
		outboxDispatcher:           compositeDispatcher,
		outboxNotifier:             setupObservabilityNotifier(cfg, log),
		outboxBatchSize:            outboxBatchSize,
		outboxPollInterval:         outboxPollInterval,
		outboxLockDuration:         outboxLockDuration,
		outboxMaxAttempts:          outboxMaxAttempts,
		outboxLagThreshold:         outboxLagThreshold,
		outboxLagCriticalThreshold: outboxLagCriticalThreshold,
		dlqReplayWorker:            dlqReplayWorker,
		notifScheduler:             notifScheduler,
	}, log)
}

// resolveSchedulerLocation parses tzName into a *time.Location. When the name
// is not recognised by the operating system's timezone database it logs a
// warning and returns time.UTC so the scheduler remains operational.
func resolveSchedulerLocation(tzName string, log *zap.Logger) *time.Location {
	loc, err := time.LoadLocation(tzName)
	if err != nil {
		log.Warn("notification scheduler: invalid timezone, falling back to UTC",
			zap.String("timezone", tzName),
			zap.Error(err),
		)
		return time.UTC
	}
	return loc
}

// logWarnOnErr logs err at Warn level when non-nil. Used to collapse repetitive
// if err != nil { log.Warn } blocks at metric registration call sites where
// a failed instrument is non-fatal.
func logWarnOnErr(log *zap.Logger, msg string, err error) {
	if err != nil {
		log.Warn(msg, zap.Error(err))
	}
}

// recoverGoroutine is deferred at the top of every background goroutine
// launched by startWorker. It catches any panic, logs it at Error level with
// a full stack trace, and allows the goroutine to exit cleanly. The process
// orchestrator (Fly.io restart policy) handles process-level recovery.
//
// Call order: place after WaitGroup.Done() defers so Done() always fires even
// when the goroutine panics. Example:
//
//	go func() {
//	    defer wg.Done()
//	    defer recoverGoroutine("name", log)
//	    work(ctx)
//	}()
func recoverGoroutine(name string, log *zap.Logger) {
	if rec := recover(); rec != nil {
		log.Error("worker: goroutine panicked — process orchestrator will restart",
			zap.String("goroutine", name),
			zap.Any("panic", rec),
			zap.ByteString("stack", debug.Stack()),
		)
	}
}

// safeEventHandler wraps h in a panic-catching envelope so a panicking handler
// does not crash the worker process. When the handler panics the panic is
// logged and a non-nil error is returned, causing the bus to apply its normal
// retry / DLQ logic rather than silently losing the event.
func safeEventHandler(name string, h func(context.Context, events.Envelope) error, log *zap.Logger) func(context.Context, events.Envelope) error {
	return func(ctx context.Context, env events.Envelope) (retErr error) {
		defer func() {
			if rec := recover(); rec != nil {
				log.Error("worker: event handler panicked",
					zap.String("handler", name),
					zap.String("event_type", string(env.Type)),
					zap.Any("panic", rec),
					zap.ByteString("stack", debug.Stack()),
				)
				retErr = fmt.Errorf("panic in handler %s: %v", name, rec)
			}
		}()
		return h(ctx, env)
	}
}

// logOrFatal logs err at WARN level in development environments and at Fatal
// level (os.Exit) in production. It is a no-op when err is nil.
// This eliminates repeated nested if/else blocks at each sender construction
// site, keeping cognitive complexity within the gocognit limit.
func logOrFatal(cfg *config.Config, log *zap.Logger, err error, warnMsg, fatalMsg string) {
	if err == nil {
		return
	}
	if cfg.IsDevelopment() {
		log.Warn(warnMsg, zap.Error(err))
	} else {
		log.Fatal(fatalMsg, zap.Error(err))
	}
}

// buildMailer returns an email sender and a non-nil error when the Resend API
// key is absent. The returned sender is always a valid (non-nil) value: when
// the key is missing it is a NoopClient so the caller can decide whether to
// degrade gracefully or treat the missing config as fatal.
//
// Callers in production environments should treat a non-nil error as fatal and
// call log.Fatal; development environments may log a warning and continue.
func buildMailer(resendAPIKey string) (infraemail.Sender, error) {
	if resendAPIKey == "" {
		return infraemail.NoopClient{}, fmt.Errorf(
			"WCQ_EMAIL_RESENDAPIKEY is not set — admin emails will be discarded; " +
				"set WCQ_EMAIL_RESENDAPIKEY to a valid Resend API key in production",
		)
	}
	return infraemail.NewResendClient(resendAPIKey), nil
}

// buildPusher returns a web-push sender and a non-nil error when any required
// VAPID credential is absent. The returned sender is always valid: when keys
// are missing it is a NoopSender. Callers decide whether the error is fatal.
//
// Required env vars: WCQ_WEBPUSH_VAPIDPRIVATEKEY (secret), plus
// ParamKeyNotifyWebPushVAPIDPublicKey and ParamKeyNotifyWebPushVAPIDSubject
// from system_params.
func buildPusher(pubKey, privKey, subject string) (infrapush.Sender, error) {
	var missing []string
	if pubKey == "" {
		missing = append(missing, "ParamKeyNotifyWebPushVAPIDPublicKey (system_params)")
	}
	if privKey == "" {
		missing = append(missing, "WCQ_WEBPUSH_VAPIDPRIVATEKEY")
	}
	if len(missing) > 0 {
		return infrapush.NoopSender{}, fmt.Errorf(
			"web push disabled — missing VAPID credentials: %s",
			strings.Join(missing, ", "),
		)
	}
	return infrapush.NewVAPIDClient(pubKey, privKey, subject), nil
}

// makePushPruneJob returns the scheduler job that deletes inactive push
// subscriptions older than the configured retention window.
func makePushPruneJob(params service.SystemParamService, pushRepo repository.PushSubscriptionRepository, log *zap.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		retentionDays := params.GetInt(ctx, domain.ParamKeyNotifyPushSubRetentionDays, domain.DefaultNotifyPushSubRetentionDays)
		cutoff := time.Now().Add(-time.Duration(retentionDays) * 24 * time.Hour)
		n, err := pushRepo.DeleteInactive(ctx, cutoff)
		if err != nil {
			log.Warn("push subscription prune: failed", zap.Error(err))
			return err
		}
		if n > 0 {
			log.Info("push subscription prune: deleted stale inactive subscriptions", zap.Int64("count", n))
		}
		return nil
	}
}

// purgeKYCDocument deletes the physical file from fileStore and the metadata row
// from docRepo for a single KYC document. Returns counters ready to be summed
// by the caller: deleted=1 on full success, fileStoreFailed=1 when the storage
// delete failed (metadata row is preserved for retry), dbFailed=1 when the
// metadata delete failed after a successful storage delete.
//
// When fileStore is nil physical deletion is skipped and the metadata row is
// still removed so the database does not accumulate stale rows.
func purgeKYCDocument(
	ctx context.Context,
	doc *domain.KYCDocument,
	fileStore storage.FileStore,
	docRepo repository.KYCDocumentRepository,
	log *zap.Logger,
) (deleted, fileStoreFailed, dbFailed int) {
	if fileStore != nil {
		if err := fileStore.Delete(ctx, doc.StorageKey); err != nil {
			log.Warn("kyc.document_purge: FileStore delete failed — skipping metadata delete, will retry next run",
				zap.Int64("doc_id", doc.ID),
				zap.String("storage_key", doc.StorageKey),
				zap.Error(err),
			)
			return 0, 1, 0
		}
	} else {
		log.Warn("kyc.document_purge: FileStore unavailable — metadata row will be removed without physical file deletion",
			zap.Int64("doc_id", doc.ID),
			zap.String("storage_key", doc.StorageKey),
		)
	}
	if err := docRepo.DeleteByID(ctx, doc.ID); err != nil {
		log.Warn("kyc.document_purge: metadata delete failed",
			zap.Int64("doc_id", doc.ID),
			zap.Error(err),
		)
		return 0, 0, 1
	}
	return 1, 0, 0
}

// makeKYCDocumentPurgeJob returns the weekly scheduler job that deletes KYC
// identity documents (DPI scans, selfies) for user accounts soft-deleted
// beyond the configured retention window.
//
// Deletion order per document:
//  1. fileStore.Delete(storageKey)   — removes the physical file from S3/OneDrive/GDrive.
//  2. docRepo.DeleteByID(id)         — removes the metadata row from the database.
//
// This ordering guarantees that the metadata row always outlives its backing
// file.  If the FileStore call fails, the metadata row is preserved so the
// next weekly run can retry — the file is never permanently orphaned.  If the
// FileStore call succeeds but the DB delete fails, the row is left with a
// dangling storage key; the next run calls fileStore.Delete again, which
// succeeds silently because the file is already gone, then retries the DB
// delete.
//
// When fileStore is nil (storage driver failed to initialise at startup), the
// job skips physical deletion for every document, logs a warning per document,
// and still removes the metadata row so the DB does not accumulate stale rows
// indefinitely.  Operators should investigate the FileStore error and restart
// the worker to restore full purge behaviour.
func makeKYCDocumentPurgeJob(
	params service.SystemParamService,
	docRepo repository.KYCDocumentRepository,
	fileStore storage.FileStore,
	log *zap.Logger,
) func(context.Context) error {
	return func(ctx context.Context) error {
		retentionYears := params.GetInt(ctx, domain.ParamKeyKYCDocRetentionYears, domain.DefaultKYCDocRetentionYears)
		cutoff := time.Now().AddDate(-retentionYears, 0, 0)

		const batchSize = 100
		docs, err := docRepo.ListExpiredDocuments(ctx, cutoff, batchSize)
		if err != nil {
			log.Warn("kyc.document_purge: list failed", zap.Error(err))
			return err
		}
		if len(docs) == 0 {
			return nil
		}

		var deleted, fileStoreFailed, dbFailed int
		for _, doc := range docs {
			d, fsf, dbf := purgeKYCDocument(ctx, doc, fileStore, docRepo, log)
			deleted += d
			fileStoreFailed += fsf
			dbFailed += dbf
		}

		log.Info("kyc.document_purge: completed",
			zap.Int("deleted", deleted),
			zap.Int("file_store_failed", fileStoreFailed),
			zap.Int("db_failed", dbFailed),
			zap.Int("retention_years", retentionYears),
		)
		return nil
	}
}

// makeCancelStaleRecurrenteJob returns the scheduler job that cancels pending
// Recurrente payment intents that have been waiting longer than 1 hour.
func makeCancelStaleRecurrenteJob(intentRepo repository.PaymentIntentRepository, log *zap.Logger) func(context.Context) error {
	return func(ctx context.Context) error {
		cutoff := time.Now().Add(-time.Hour)
		n, err := intentRepo.CancelStalePendingCardIntents(ctx, cutoff)
		if err != nil {
			log.Warn("payment.cancel_stale_recurrente: failed", zap.Error(err))
			return err
		}
		if n > 0 {
			log.Info("payment.cancel_stale_recurrente: cancelled stale pending intents", zap.Int64("count", n))
		}
		return nil
	}
}

// startWorker wires event subscribers, starts the health HTTP server, starts
// the DLQ monitoring goroutine, and blocks until ctx is cancelled (i.e. until
// an OS signal is received).
//
// workerConsumerGroup is the Redis Streams consumer group name used by this
// worker process. Both the event bus and the stream-pending health checker
// must reference the same name to correctly report consumer lag.
const workerConsumerGroup = "quiniela-workers"

// buildNotifScheduler constructs the notification Scheduler and registers all
// periodic notification jobs. The FX rate refresh job is NOT registered here —
// it is added by the caller after fxImpl is constructed so that the scheduler
// and the FX service can be built independently.
//
// KYC FileStore initialisation failures are non-fatal: the returned scheduler
// still registers the purge job, but physical file deletion is disabled until
// the next restart (the DB metadata rows are still removed).
func buildNotifScheduler(
	ctx context.Context,
	storageCfg config.StorageConfig,
	params service.SystemParamService,
	db *pgxpool.Pool,
	outboxWriter outbox.Writer,
	pushRepo repository.PushSubscriptionRepository,
	log *zap.Logger,
) *scheduler.Scheduler {
	tzName := params.GetString(ctx, domain.ParamKeyNotifySchedulerTimezone, domain.DefaultNotifySchedulerTimezone)
	schedulerLoc := resolveSchedulerLocation(tzName, log)
	schedulerStore := repository.NewPostgresSchedulerStore(db)
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  schedulerStore,
		Writer: outboxWriter,
		Params: params,
		Log:    log,
	})
	s := scheduler.New(scheduler.Config{
		Location: schedulerLoc,
		Log:      log,
	})
	s.RegisterInterval("prediction.match_reminder",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPredDeadlineIntervalSec, domain.DefaultWorkerSchedPredDeadlineIntervalSec))*time.Second,
		jobs.PredictionDailyReminder)
	s.RegisterInterval("admin.match_result_pending",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedMatchResultIntervalSec, domain.DefaultWorkerSchedMatchResultIntervalSec))*time.Second,
		jobs.AdminMatchResultPending)
	s.RegisterInterval("admin.pending_reminder",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPendingReminderIntervalSec, domain.DefaultWorkerSchedPendingReminderIntervalSec))*time.Second,
		jobs.AdminPendingReminder)
	s.RegisterInterval("admin.stale_escalation",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedStaleEscalationIntervalSec, domain.DefaultWorkerSchedStaleEscalationIntervalSec))*time.Second,
		jobs.StaleEscalation)
	s.RegisterInterval("user.daily_summary",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedDailySummaryIntervalSec, domain.DefaultWorkerSchedDailySummaryIntervalSec))*time.Second,
		jobs.UserDailySummary)
	s.RegisterDaily("admin.daily_summary", 8, 0, jobs.AdminDailySummary)
	s.RegisterWeekly("admin.weekly_report", time.Monday, 8, 0, jobs.AdminWeeklyReport)
	s.RegisterInterval("push.subscription_prune",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPushPruneIntervalSec, domain.DefaultWorkerSchedPushPruneIntervalSec))*time.Second,
		makePushPruneJob(params, pushRepo, log))

	// KYC document lifecycle — purge document metadata and physical files for
	// accounts deleted beyond the configured retention window.  Runs weekly at
	// 03:00 on Sunday (low-traffic window).
	kycDocRepo := repository.NewPostgresKYCDocumentRepository(db)
	kycFileStore, kycStoreErr := storage.New(ctx, storage.Config{
		Driver:                storageCfg.Driver,
		LocalDir:              storageCfg.LocalDir,
		S3Bucket:              storageCfg.S3Bucket,
		S3Endpoint:            storageCfg.S3Endpoint,
		S3Region:              storageCfg.S3Region,
		S3AccessKeyID:         storageCfg.S3AccessKeyID,
		S3SecretKey:           storageCfg.S3SecretKey,
		OneDriveTenantID:      storageCfg.OneDriveTenantID,
		OneDriveClientID:      storageCfg.OneDriveClientID,
		OneDriveClientSecret:  storageCfg.OneDriveClientSecret,
		OneDriveDriveID:       storageCfg.OneDriveDriveID,
		GDriveCredentialsJSON: storageCfg.GDriveCredentialsJSON,
		GDriveFolderID:        storageCfg.GDriveFolderID,
	})
	if kycStoreErr != nil {
		log.Warn("kyc.document_purge: FileStore unavailable — physical file deletion disabled until next restart",
			zap.Error(kycStoreErr))
	}
	s.RegisterWeekly("kyc.document_purge", time.Sunday, 3, 0,
		makeKYCDocumentPurgeJob(params, kycDocRepo, kycFileStore, log))

	return s
}

// buildHealthCheckers constructs the full set of readiness checkers for the
// worker process. Extracting this into its own function keeps run() readable
// and makes the checker list independently testable without needing a live
// database or Redis connection - the constructors are pure and only perform
// I/O when Check() is called.
func buildHealthCheckers(db *pgxpool.Pool, rc *redis.Client) []health.Checker {
	return []health.Checker{
		health.NewDBChecker(db),
		health.NewRedisChecker(rc),
		health.NewDLQChecker(rc, string(events.EventMatchStarted)),
		health.NewDLQChecker(rc, string(events.EventMatchFinished)),
		health.NewStreamPendingChecker(rc, "stream:"+string(events.EventMatchStarted), workerConsumerGroup),
		health.NewStreamPendingChecker(rc, "stream:"+string(events.EventMatchFinished), workerConsumerGroup),
	}
}

// registerScoringDLQGauge registers the wcq_scoring_dlq_depth observable gauge.
// One series is emitted per monitored event type (label: event_type). The gauge
// reads the Redis DLQ list length on every Prometheus scrape so the metric
// stays current between monitorDLQ ticks.
//
// Redis errors are fail-open: a connectivity problem reports 0 rather than
// returning an error that would suppress the gauge entirely. The existing
// monitorDLQ log lines (dlq_size) complement this metric — the gauge gives
// Prometheus visibility; the log gives a timestamped event trail.
func registerScoringDLQGauge(meter metric.Meter, rc *redis.Client) error {
	g, err := meter.Int64ObservableGauge("wcq_scoring_dlq_depth",
		metric.WithDescription("Number of scoring events in the Redis dead-letter queue by event type. "+
			"Non-zero means failed handler retries that require manual replay via POST /admin/dlq/replay."),
	)
	if err != nil {
		return fmt.Errorf("register wcq_scoring_dlq_depth: %w", err)
	}
	monitoredEvents := []events.EventType{events.EventMatchStarted, events.EventMatchFinished}
	_, err = meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		for _, et := range monitoredEvents {
			n, redisErr := rc.LLen(ctx, "dlq:"+string(et)).Result()
			if redisErr != nil {
				n = 0 // fail-open: prefer under-alerting to spurious fires on Redis unavailability
			}
			o.ObserveInt64(g, n, metric.WithAttributes(attribute.String("event_type", string(et))))
		}
		return nil
	}, g)
	if err != nil {
		return fmt.Errorf("register wcq_scoring_dlq_depth callback: %w", err)
	}
	return nil
}

// dlqMonitorLockID is the PostgreSQL advisory lock identifier for DLQ monitor
// leader election. Each worker replica calls pg_try_advisory_lock on this ID;
// only one replica acquires it per session lifetime.
const dlqMonitorLockID int64 = 1

// workerDeps bundles the injected dependencies for startWorker, keeping its
// parameter list within the 7-param lint limit while remaining easy to
// extend without changing the function signature.
type workerDeps struct {
	cfg                        *config.Config
	bus                        events.Bus
	scorer                     service.MatchScorer
	autoConfirmer              SlotAutoConfirmer
	snapshotter                service.Snapshotter
	predRepo                   repository.PredictionRepository
	invalidators               []service.PostScoringInvalidator
	broadcaster                LeaderboardBroadcaster
	snapshotLocker             SnapshotLocker
	snapshotCfg                snapshotConfig
	purger                     repository.Purger
	purgePol                   purgePolicy
	rc                         *redis.Client
	checkers                   []health.Checker
	metricsHandler             http.Handler // nil when metrics are disabled
	dlqElection                election.LeaderElection
	outboxRepo                 outbox.Repository
	outboxDispatcher           outbox.Dispatcher
	outboxNotifier             *observability.Notifier
	outboxBatchSize            int
	outboxPollInterval         time.Duration
	outboxLockDuration         time.Duration
	outboxMaxAttempts          int
	outboxLagThreshold         time.Duration
	outboxLagCriticalThreshold time.Duration
	dlqReplayWorker            *outbox.DLQWorker
	notifScheduler             *scheduler.Scheduler
}

// All parameters are already constructed so this function has no I/O of its
// own. This makes it the boundary between infrastructure setup (run) and
// lifecycle management - and the part that can be exercised in unit tests
// by injecting an InMemoryBus, a stub scorer, and a pre-cancelled context.
func startWorker(ctx context.Context, deps workerDeps, log *zap.Logger) error {
	if deps.autoConfirmer != nil {
		if err := deps.autoConfirmer.BackfillSlots(ctx); err != nil {
			log.Warn("worker: slot backfill failed", zap.Error(err))
		} else {
			log.Info("worker: slot backfill complete")
		}
	}

	deps.bus.Subscribe(ctx, events.EventMatchStarted,
		safeEventHandler("matchStarted", newMatchStartedHandler(log), log))
	log.Sugar().Info("worker: subscribed to MatchStarted events")

	deps.bus.Subscribe(ctx, events.EventMatchFinished,
		safeEventHandler("matchFinished", newMatchFinishedHandler(deps.scorer, postScoringDeps{
			snapshotter:   deps.snapshotter,
			predRepo:      deps.predRepo,
			invalidators:  deps.invalidators,
			broadcaster:   deps.broadcaster,
			locker:        deps.snapshotLocker,
			snapshot:      deps.snapshotCfg,
			autoConfirmer: deps.autoConfirmer,
		}, log), log))
	log.Sugar().Info("worker: subscribed to MatchFinished events")

	healthSrv := newHealthServer(deps.cfg.Worker.HealthPort, deps.checkers, deps.metricsHandler, log)

	// srvErr receives a non-nil value only when the health server exits with an
	// unexpected error. Buffered so the goroutine never blocks if startWorker
	// returns before draining the channel (e.g. ctx already cancelled).
	srvErr := make(chan error, 1)
	go func() {
		defer recoverGoroutine("healthServer", log)
		log.Sugar().Infof("worker health server listening on :%s", deps.cfg.Worker.HealthPort)
		if err := healthSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			srvErr <- err
		}
	}()

	// DLQ monitoring goroutine: logs the size of each dead-letter queue at a
	// fixed interval. Operators should configure an alert on log lines that
	// contain "dead-letter queue is non-empty" to be notified within one
	// dlqMonitorInterval of a scoring failure.
	// The ticker is created here and its Stop is deferred inside the goroutine
	// so tests can inject a pre-loaded channel without touching dlqMonitorInterval.
	// A WaitGroup ensures the goroutine has fully exited before startWorker
	// returns, preventing a data race in tests that run under -race.
	var dlqDone sync.WaitGroup
	dlqDone.Add(1)
	ticker := time.NewTicker(dlqMonitorInterval)
	go func() {
		defer dlqDone.Done()
		defer recoverGoroutine("dlqMonitor", log)
		defer ticker.Stop()
		monitorDLQ(ctx, deps.rc, deps.dlqElection, ticker.C, deps.outboxNotifier, log)
	}()

	var purgeDone sync.WaitGroup
	purgeDone.Add(1)
	purgeTicker := time.NewTicker(purgeTickInterval)
	go func() {
		defer purgeDone.Done()
		defer recoverGoroutine("purgeDaemon", log)
		defer purgeTicker.Stop()
		monitorPurge(ctx, deps.purger, deps.purgePol, purgeTicker.C, log)
	}()

	// Outbox worker — polls domain_outbox and dispatches admin/system notifications.
	var outboxDone sync.WaitGroup
	if deps.outboxRepo != nil && deps.outboxDispatcher != nil {
		outboxWorker := outbox.NewWorker(deps.outboxRepo, deps.outboxDispatcher, log,
			outbox.WithBatchSize(deps.outboxBatchSize),
			outbox.WithPollInterval(deps.outboxPollInterval),
			outbox.WithLockDuration(deps.outboxLockDuration),
			outbox.WithMaxAttempts(deps.outboxMaxAttempts),
			outbox.WithOutboxLagThreshold(deps.outboxLagThreshold),
			outbox.WithOutboxCriticalLagThreshold(deps.outboxLagCriticalThreshold),
			outbox.WithOutboxNotifier(deps.outboxNotifier),
		)
		outboxDone.Add(1)
		go func() {
			defer outboxDone.Done()
			defer recoverGoroutine("outboxWorker", log)
			outboxWorker.Run(ctx)
		}()
		log.Info("outbox worker started (admin email dispatcher active)")
	}

	// DLQ replay worker — replays failed notification_dlq entries into domain_outbox.
	var dlqReplayDone sync.WaitGroup
	if deps.dlqReplayWorker != nil {
		dlqReplayDone.Add(1)
		go func() {
			defer dlqReplayDone.Done()
			defer recoverGoroutine("dlqReplayWorker", log)
			deps.dlqReplayWorker.Run(ctx)
		}()
		log.Info("dlq replay worker started")
	}

	// Notification scheduler — prediction deadline reminders, admin digests, match result alerts.
	var notifSchedDone sync.WaitGroup
	if deps.notifScheduler != nil {
		notifSchedDone.Add(1)
		go func() {
			defer notifSchedDone.Done()
			defer recoverGoroutine("notifScheduler", log)
			deps.notifScheduler.Run(ctx)
		}()
		log.Info("notification scheduler started")
	}

	var runErr error
	select {
	case <-ctx.Done():
		log.Sugar().Info("worker: shutdown signal received, stopping...")
	case err := <-srvErr:
		// Health server failed before a shutdown signal arrived. Log and proceed
		// to the graceful shutdown path so all defers in run() still execute.
		log.Sugar().Errorf("worker: health server failed: %v", err)
		runErr = err
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), deps.cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		log.Sugar().Errorf("worker: health server shutdown failed: %v", err)
	}
	dlqDone.Wait()
	if deps.dlqElection != nil {
		deps.dlqElection.Close(shutdownCtx)
	}
	purgeDone.Wait()
	outboxDone.Wait()
	dlqReplayDone.Wait()
	notifSchedDone.Wait()
	log.Sugar().Info("worker stopped")
	return runErr
}

// purgePolicy bundles all retention windows and limits used by the purge daemon.
// Grouping them into one value keeps monitorPurge and executePurgeTick within
// the seven-parameter limit enforced by the linter.
type purgePolicy struct {
	retention              time.Duration
	paramHistoryRetention  time.Duration
	fxHistoryRetention     time.Duration
	outboxRetention        time.Duration
	snapshotKeepCount      int
	intentExpiredRetention time.Duration // window after expiry before deletion
}

// monitorPurge runs until ctx is cancelled, permanently removing soft-deleted
// users and quinielas older than pol.retention on each tick received from tickC.
// Errors are logged at Warn level and swallowed: a failed purge tick is retried
// on the next interval, so transient DB hiccups do not stop the worker.
//
// If purger is nil (e.g. in unit tests where the database is not available),
// the function returns immediately.
func monitorPurge(ctx context.Context, purger repository.Purger, pol purgePolicy, tickC <-chan time.Time, log *zap.Logger) {
	if purger == nil {
		return
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			executePurgeTick(ctx, purger, pol, log)
		}
	}
}

// purgeExpiredIntents deletes expired pending payment intents when the policy
// retention window is set. Extracted from executePurgeTick to keep that
// function's cognitive complexity within the project ceiling.
func purgeExpiredIntents(ctx context.Context, purger repository.Purger, pol purgePolicy, log *zap.Logger) {
	if pol.intentExpiredRetention <= 0 {
		return
	}
	intentBefore := time.Now().Add(-pol.intentExpiredRetention)
	if n, err := purger.PurgeExpiredPaymentIntents(ctx, intentBefore); err != nil {
		log.Warn("worker: purge expired payment intents failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged expired pending payment intents", zap.Int64("count", n))
	}
}

// executePurgeTick runs one purge cycle: soft-deleted rows, snapshots, param
// history, FX history, and terminal outbox entries. Each operation is
// independent and non-fatal — a failure is logged and the next tick retries.
func executePurgeTick(ctx context.Context, purger repository.Purger, pol purgePolicy, log *zap.Logger) {
	olderThan := time.Now().Add(-pol.retention)
	if n, err := purger.PurgeDeletedUsers(ctx, olderThan); err != nil {
		log.Warn("worker: purge deleted users failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged soft-deleted users", zap.Int64("count", n))
	}
	if n, err := purger.PurgeDeletedQuinielas(ctx, olderThan); err != nil {
		log.Warn("worker: purge deleted quinielas failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged soft-deleted quinielas", zap.Int64("count", n))
	}
	if n, err := purger.PurgeOldSnapshots(ctx, pol.snapshotKeepCount); err != nil {
		log.Warn("worker: purge old snapshots failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged old leaderboard snapshots", zap.Int64("count", n))
	}
	historyBefore := time.Now().Add(-pol.paramHistoryRetention)
	if n, err := purger.PurgeOldParamHistory(ctx, historyBefore); err != nil {
		log.Warn("worker: purge param history failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged old param history rows", zap.Int64("count", n))
	}
	fxBefore := time.Now().Add(-pol.fxHistoryRetention)
	if n, err := purger.PurgeOldFXHistory(ctx, fxBefore); err != nil {
		log.Warn("worker: purge fx history failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged old exchange rate history rows", zap.Int64("count", n))
	}
	outboxBefore := time.Now().Add(-pol.outboxRetention)
	if n, err := purger.PurgeOldOutboxEntries(ctx, outboxBefore); err != nil {
		log.Warn("worker: purge old outbox entries failed", zap.Error(err))
	} else if n > 0 {
		log.Info("worker: purged old terminal outbox entries", zap.Int64("count", n))
	}
	// Purge expired pending payment intents. Zero retention (unset or zero-value
	// policy) disables this step — safe default for tests without a DB.
	purgeExpiredIntents(ctx, purger, pol, log)
}

// monitorDLQ runs until ctx is cancelled, logging the size of every
// event-type dead-letter queue on each tick received from tickC. A non-zero
// count indicates events that exhausted all handler retry attempts and require
// manual operator replay. The log line is structured so log-based alerting
// systems (Datadog, CloudWatch Logs Insights, Loki) can match on "dlq_size".
//
// Leader election via e ensures that in a multi-replica deployment only one
// worker emits DLQ log lines per interval. Each tick is a fresh competition:
// the replica that wins the Redis SET NX lock performs the scan; the others
// skip that tick silently. If e is nil the function degrades gracefully and
// all replicas log (original behaviour — safe for single-replica setups).
//
// tickC is injected by the caller so tests can pass a pre-loaded buffered
// channel without mutating any global state. In production startWorker passes
// time.NewTicker(dlqMonitorInterval).C.
//
// If rc is nil (e.g. in unit tests where Redis is not available), the function
// returns immediately - DLQ monitoring is best-effort and must not block startup.
func monitorDLQ(ctx context.Context, rc *redis.Client, e election.LeaderElection, tickC <-chan time.Time, notifier *observability.Notifier, log *zap.Logger) {
	if rc == nil {
		return
	}

	// The event types whose DLQ keys this worker is responsible for.
	monitoredEvents := []events.EventType{events.EventMatchStarted, events.EventMatchFinished}

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			if e != nil && !e.TryAcquire(ctx) {
				log.Debug("worker: DLQ monitor: not leader this tick, skipping")
				continue
			}
			for _, et := range monitoredEvents {
				checkDLQEvent(ctx, rc, et, notifier, log)
			}
		}
	}
}

// checkDLQEvent reads the length of one DLQ key and logs (and notifies) when
// it is non-empty. Extracted from monitorDLQ to keep that function's cognitive
// complexity within the project ceiling.
func checkDLQEvent(ctx context.Context, rc *redis.Client, et events.EventType, notifier *observability.Notifier, log *zap.Logger) {
	dlqKey := "dlq:" + string(et)
	n, err := rc.LLen(ctx, dlqKey).Result()
	if err != nil {
		log.Warn("worker: DLQ monitor: LLEN failed",
			zap.String("dlq_key", dlqKey),
			zap.Error(err),
		)
		return
	}
	if n > 0 {
		log.Error("worker: dead-letter queue is non-empty - manual replay required",
			zap.String("dlq_key", dlqKey),
			zap.String("event_type", string(et)),
			zap.Int64("dlq_size", n),
		)
		if notifier != nil {
			notifier.NotifyDLQOverflow(ctx, n, 0)
		}
	} else {
		log.Debug("worker: DLQ monitor: queue is empty",
			zap.String("dlq_key", dlqKey),
		)
	}
}

// newHealthServer constructs the lightweight HTTP server that exposes liveness
// and readiness probes for the worker process.
func newHealthServer(port string, checkers []health.Checker, metricsHandler http.Handler, log *zap.Logger) *http.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", handleLiveness)
	mux.HandleFunc("/health/ready", health.ReadinessHandler(checkers))
	if metricsHandler != nil {
		mux.Handle("/metrics", metricsHandler)
	}

	return &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  30 * time.Second,
	}
}

// handleLiveness responds to liveness probes. It only verifies that the
// process is alive - not that its dependencies are reachable.
func handleLiveness(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","service":"world-cup-quiniela-worker"}`)
}

// repoEmailResolver adapts repository.UserRepository to the
// dispatcher.UserEmailResolver interface so UserDispatcher can resolve user
// email addresses when delivering transactional emails.
type repoEmailResolver struct {
	userRepo repository.UserRepository
}

func (r *repoEmailResolver) ResolveEmailByID(ctx context.Context, userID int) (string, string, error) {
	u, err := r.userRepo.GetByID(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if u == nil {
		return "", "", fmt.Errorf("user %d not found", userID)
	}
	return u.Email, u.Name, nil
}

// wirePgDLQFallback attaches a Postgres-backed DeadLetterRecorder to the event
// bus so that scoring events cannot be silently lost when the Redis DLQ itself
// is unavailable. The worker unconditionally validates the redis driver before
// reaching this call, so the type assertion always succeeds in production; a
// failed assertion is treated as a silent no-op so tests using an InMemoryBus
// are unaffected.
func wirePgDLQFallback(bus events.Bus, db *pgxpool.Pool, log *zap.Logger) {
	redisBus, ok := bus.(*messaging.RedisBus)
	if !ok {
		return
	}
	redisBus.SetDLQFallback(repository.NewPostgresEventDLQRepository(db))
	log.Info("worker: Postgres event DLQ fallback wired")
}
