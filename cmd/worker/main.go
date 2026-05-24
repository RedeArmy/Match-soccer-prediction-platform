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
	"sync"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
	"golang.org/x/sync/semaphore"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/election"
	infraemail "github.com/rede/world-cup-quiniela/internal/infrastructure/email"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
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
	defer func() {
		// context.WithoutCancel inherits the OTel span values from ctx without
		// being affected by the cancellation that already fired (SIGTERM).
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := shutdownTracing(flushCtx); err != nil {
			log.Sugar().Warnf("tracing flush: %v", err)
		}
	}()

	meter, metricsHandler, shutdownMetrics, err := setupMetrics(cfg, log)
	if err != nil {
		return fmt.Errorf("metrics: %w", err)
	}
	log = wireLogLevelCounters(log, meter)
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := shutdownMetrics(flushCtx); err != nil {
			log.Sugar().Warnf("metrics flush: %v", err)
		}
	}()

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
	snapshotConcurrency := params.GetInt(ctx, domain.ParamKeyWorkerSnapshotConcurrency, domain.DefaultWorkerSnapshotConcurrency)
	snapshotRetryBase := time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSnapshotRetryBaseMs, domain.DefaultWorkerSnapshotRetryBaseMs)) * time.Millisecond
	maxSnapshotAttempts := params.GetInt(ctx, domain.ParamKeyWorkerSnapshotMaxAttempts, domain.DefaultWorkerSnapshotMaxAttempts)
	dlqMonitorInterval = time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerDLQMonitorIntervalSec, domain.DefaultWorkerDLQMonitorIntervalSec)) * time.Second
	purgeTickInterval = time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerPurgeIntervalHours, domain.DefaultWorkerPurgeIntervalHours)) * time.Hour
	snapshotKeepLatestCount = params.GetInt(ctx, domain.ParamKeySnapshotKeepLatestCount, domain.DefaultSnapshotKeepLatestCount)
	ruleRepo := repository.NewPostgresScoringRuleRepository(db)
	scorer := service.NewScoringService(matchRepo, predRepo, ruleRepo, params, log)

	quinielaRepo := repository.NewPostgresQuinielaRepository(db)
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
	broadcaster := LeaderboardBroadcaster(&redisPubLeaderboardBroadcaster{
		client:      rc,
		memberRepo:  memberRepo,
		concurrency: snapshotConcurrency,
		log:         log,
	})

	purger := repository.NewPostgresPurger(db)
	retentionDays := params.GetInt(ctx, domain.ParamKeyPurgeRetentionDays, domain.DefaultPurgeRetentionDays)
	purgeRetention := time.Duration(retentionDays) * 24 * time.Hour
	paramHistoryRetentionDays := params.GetInt(ctx, domain.ParamKeySystemParamHistoryRetentionDays, domain.DefaultSystemParamHistoryRetentionDays)
	paramHistoryRetention := time.Duration(paramHistoryRetentionDays) * 24 * time.Hour

	// Leader election for the DLQ monitor via a PostgreSQL session-level
	// advisory lock. A 15-second timeout is added to ctx to bound the
	// connection handshake without preventing the lifecycle context from
	// cancelling the attempt if a shutdown signal arrives during startup.
	electionCtx, electionCancel := context.WithTimeout(ctx, 15*time.Second)
	defer electionCancel()
	dlqElection, err := election.NewPgLeaderElection(electionCtx, cfg.Database.DSN, dlqMonitorLockID, log)
	if err != nil {
		return fmt.Errorf("leader election: %w", err)
	}

	// ── Notification subsystem ────────────────────────────────────────────────

	outboxBatchSize := params.GetInt(ctx, domain.ParamKeyNotifyOutboxBatchSize, domain.DefaultNotifyOutboxBatchSize)
	outboxPollInterval := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxPollIntervalSec, domain.DefaultNotifyOutboxPollIntervalSec)) * time.Second
	outboxLockDuration := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxLockDurationSec, domain.DefaultNotifyOutboxLockDurationSec)) * time.Second
	outboxMaxAttempts := params.GetInt(ctx, domain.ParamKeyNotifyOutboxMaxAttempts, domain.DefaultNotifyOutboxMaxAttempts)
	outboxLagThreshold := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyOutboxLagAlertThresholdSec, domain.DefaultNotifyOutboxLagAlertThresholdSec)) * time.Second

	outboxRepo := outbox.NewPostgresRepository(db)
	outboxWriter := outbox.NewWriter(db)

	adminLogRepo := repository.NewPostgresAdminNotificationLogRepository(db)
	dlqRepo := repository.NewPostgresNotificationDLQRepository(db)

	mailer := buildMailer(cfg.Email.ResendAPIKey, log)

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

	pusher := buildPusher(vapidPublicKey, vapidPrivateKey, vapidSubject, log)

	tmplCacheTTL := time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyTemplateCacheTTLSec, domain.DefaultNotifyTemplateCacheTTLSec)) * time.Second
	tmplRepo := repository.NewPostgresNotificationTemplateRepository(db, tmplCacheTTL)

	digestWindowSec := int64(params.GetInt(ctx, domain.ParamKeyNotifyPushDigestWindowSec, domain.DefaultNotifyPushDigestWindowSec))
	digestThreshold := int32(params.GetInt(ctx, domain.ParamKeyNotifyPushDigestThreshold, domain.DefaultNotifyPushDigestThreshold))

	// Wire the cluster-aware digest gate when Redis is available so that the
	// per-user push threshold is enforced across all worker replicas. Fall back
	// to the in-memory gate for single-process deployments (development, CI).
	var digestGate notification.Recorder
	if rc != nil {
		digestGate = notification.NewRedisPushDigestGate(rc, digestWindowSec, digestThreshold)
	} else {
		digestGate = notification.NewPushDigestGate(digestWindowSec, digestThreshold)
	}

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
	if err := adminDispatcher.RegisterMetrics(meter); err != nil {
		log.Warn("adminDispatcher.RegisterMetrics failed", zap.Error(err))
	}
	if err := userDispatcher.RegisterMetrics(meter); err != nil {
		log.Warn("userDispatcher.RegisterMetrics failed", zap.Error(err))
	}
	if err := outbox.RegisterPendingGauge(meter, outboxRepo); err != nil {
		log.Warn("outbox.RegisterPendingGauge failed", zap.Error(err))
	}
	if err := outbox.RegisterDLQDepthGauge(meter, dlqRepo); err != nil {
		log.Warn("outbox.RegisterDLQDepthGauge failed", zap.Error(err))
	}
	if err := outbox.RegisterOldestPendingAgeGauge(meter, outboxRepo); err != nil {
		log.Warn("outbox.RegisterOldestPendingAgeGauge failed", zap.Error(err))
	}

	dlqReplayWorker := outbox.NewDLQWorker(dlqRepo, outboxWriter, log,
		outbox.WithDLQBatchSize(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayBatchSize, domain.DefaultNotifyDLQReplayBatchSize)),
		outbox.WithDLQPollInterval(time.Duration(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayPollIntervalSec, domain.DefaultNotifyDLQReplayPollIntervalSec))*time.Second),
		outbox.WithDLQMaxAttempts(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayMaxAttempts, domain.DefaultNotifyDLQReplayMaxAttempts)),
		outbox.WithDLQAlertThreshold(int64(params.GetInt(ctx, domain.ParamKeyNotifyDLQReplayAlertThreshold, domain.DefaultNotifyDLQReplayAlertThreshold))),
		outbox.WithDLQNotifier(setupObservabilityNotifier(cfg, log)))

	// Notification scheduler: prediction deadline reminders, admin digests,
	// match result alerts, and stale-operation escalation.
	tzName := params.GetString(ctx, domain.ParamKeyNotifySchedulerTimezone, domain.DefaultNotifySchedulerTimezone)
	schedulerLoc, tzErr := time.LoadLocation(tzName)
	if tzErr != nil {
		log.Warn("notification scheduler: invalid timezone, falling back to UTC",
			zap.String("timezone", tzName),
			zap.Error(tzErr),
		)
		schedulerLoc = time.UTC
	}
	schedulerStore := repository.NewPostgresSchedulerStore(db)
	jobs := scheduler.NewJobs(scheduler.JobsConfig{
		Store:  schedulerStore,
		Writer: outboxWriter,
		Params: params,
		Log:    log,
	})
	notifScheduler := scheduler.New(scheduler.Config{
		Location: schedulerLoc,
		Log:      log,
	})
	notifScheduler.RegisterInterval("prediction.deadline_approaching",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPredDeadlineIntervalSec, domain.DefaultWorkerSchedPredDeadlineIntervalSec))*time.Second,
		jobs.PredictionDeadlineApproaching)
	notifScheduler.RegisterInterval("admin.match_result_pending",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedMatchResultIntervalSec, domain.DefaultWorkerSchedMatchResultIntervalSec))*time.Second,
		jobs.AdminMatchResultPending)
	notifScheduler.RegisterInterval("admin.pending_reminder",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPendingReminderIntervalSec, domain.DefaultWorkerSchedPendingReminderIntervalSec))*time.Second,
		jobs.AdminPendingReminder)
	notifScheduler.RegisterInterval("admin.stale_escalation",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedStaleEscalationIntervalSec, domain.DefaultWorkerSchedStaleEscalationIntervalSec))*time.Second,
		jobs.StaleEscalation)
	notifScheduler.RegisterDaily("admin.daily_summary", 8, 0, jobs.AdminDailySummary)
	notifScheduler.RegisterWeekly("admin.weekly_report", time.Monday, 8, 0, jobs.AdminWeeklyReport)
	notifScheduler.RegisterInterval("push.subscription_prune",
		time.Duration(params.GetInt(ctx, domain.ParamKeyWorkerSchedPushPruneIntervalSec, domain.DefaultWorkerSchedPushPruneIntervalSec))*time.Second,
		makePushPruneJob(params, pushRepo, log))

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

	return startWorker(ctx, workerDeps{
		cfg:                   cfg,
		bus:                   bus,
		scorer:                scorer,
		snapshotter:           snapshotter,
		predRepo:              predRepo,
		invalidators:          invalidators,
		broadcaster:           broadcaster,
		snapshotLocker:        &redisSnapshotLocker{client: rc, ttl: snapshotLockTTL},
		snapshotCfg:           snapCfg,
		purger:                purger,
		purgeRetention:        purgeRetention,
		paramHistoryRetention: paramHistoryRetention,
		snapshotKeepCount:     snapshotKeepLatestCount,
		rc:                    rc,
		checkers:              checkers,
		metricsHandler:        metricsHandler,
		dlqElection:           dlqElection,
		outboxRepo:            outboxRepo,
		outboxDispatcher:      compositeDispatcher,
		outboxNotifier:        setupObservabilityNotifier(cfg, log),
		outboxBatchSize:       outboxBatchSize,
		outboxPollInterval:    outboxPollInterval,
		outboxLockDuration:    outboxLockDuration,
		outboxMaxAttempts:     outboxMaxAttempts,
		outboxLagThreshold:    outboxLagThreshold,
		dlqReplayWorker:       dlqReplayWorker,
		notifScheduler:        notifScheduler,
	}, log)
}

// buildMailer returns an email sender: a real Resend client when the API key is
// configured, or a no-op client that discards all messages otherwise.
func buildMailer(resendAPIKey string, log *zap.Logger) infraemail.Sender {
	if resendAPIKey == "" {
		log.Warn("WCQ_EMAIL_RESENDAPIKEY is not set — admin emails will be discarded (NoopClient)")
		return infraemail.NoopClient{}
	}
	return infraemail.NewResendClient(resendAPIKey)
}

// buildPusher returns a web-push sender: a real VAPID client when both keys are
// present, or a no-op sender that silently drops pushes otherwise.
func buildPusher(pubKey, privKey, subject string, log *zap.Logger) infrapush.Sender {
	if pubKey == "" || privKey == "" {
		log.Warn("web push: VAPID keys not configured — push notifications disabled (NoopSender)")
		return infrapush.NoopSender{}
	}
	log.Info("web push: VAPID client active")
	return infrapush.NewVAPIDClient(pubKey, privKey, subject)
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

// startWorker wires event subscribers, starts the health HTTP server, starts
// the DLQ monitoring goroutine, and blocks until ctx is cancelled (i.e. until
// an OS signal is received).
//
// workerConsumerGroup is the Redis Streams consumer group name used by this
// worker process. Both the event bus and the stream-pending health checker
// must reference the same name to correctly report consumer lag.
const workerConsumerGroup = "quiniela-workers"

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

// dlqMonitorLockID is the PostgreSQL advisory lock identifier for DLQ monitor
// leader election. Each worker replica calls pg_try_advisory_lock on this ID;
// only one replica acquires it per session lifetime.
const dlqMonitorLockID int64 = 1

// workerDeps bundles the injected dependencies for startWorker, keeping its
// parameter list within the 7-param lint limit while remaining easy to
// extend without changing the function signature.
type workerDeps struct {
	cfg                   *config.Config
	bus                   events.Bus
	scorer                service.MatchScorer
	snapshotter           service.Snapshotter
	predRepo              repository.PredictionRepository
	invalidators          []service.PostScoringInvalidator
	broadcaster           LeaderboardBroadcaster
	snapshotLocker        SnapshotLocker
	snapshotCfg           snapshotConfig
	purger                repository.Purger
	purgeRetention        time.Duration
	paramHistoryRetention time.Duration
	snapshotKeepCount     int
	rc                    *redis.Client
	checkers              []health.Checker
	metricsHandler        http.Handler // nil when metrics are disabled
	dlqElection           election.LeaderElection
	outboxRepo            outbox.Repository
	outboxDispatcher      outbox.Dispatcher
	outboxNotifier        *observability.Notifier
	outboxBatchSize       int
	outboxPollInterval    time.Duration
	outboxLockDuration    time.Duration
	outboxMaxAttempts     int
	outboxLagThreshold    time.Duration
	dlqReplayWorker       *outbox.DLQWorker
	notifScheduler        *scheduler.Scheduler
}

// All parameters are already constructed so this function has no I/O of its
// own. This makes it the boundary between infrastructure setup (run) and
// lifecycle management - and the part that can be exercised in unit tests
// by injecting an InMemoryBus, a stub scorer, and a pre-cancelled context.
func startWorker(ctx context.Context, deps workerDeps, log *zap.Logger) error {
	deps.bus.Subscribe(ctx, events.EventMatchStarted, newMatchStartedHandler(log))
	log.Sugar().Info("worker: subscribed to MatchStarted events")

	deps.bus.Subscribe(ctx, events.EventMatchFinished,
		newMatchFinishedHandler(deps.scorer, postScoringDeps{
			snapshotter:  deps.snapshotter,
			predRepo:     deps.predRepo,
			invalidators: deps.invalidators,
			broadcaster:  deps.broadcaster,
			locker:       deps.snapshotLocker,
			snapshot:     deps.snapshotCfg,
		}, log))
	log.Sugar().Info("worker: subscribed to MatchFinished events")

	healthSrv := newHealthServer(deps.cfg.Worker.HealthPort, deps.checkers, deps.metricsHandler, log)

	// srvErr receives a non-nil value only when the health server exits with an
	// unexpected error. Buffered so the goroutine never blocks if startWorker
	// returns before draining the channel (e.g. ctx already cancelled).
	srvErr := make(chan error, 1)
	go func() {
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
		defer ticker.Stop()
		monitorDLQ(ctx, deps.rc, deps.dlqElection, ticker.C, log)
	}()

	var purgeDone sync.WaitGroup
	purgeDone.Add(1)
	purgeTicker := time.NewTicker(purgeTickInterval)
	go func() {
		defer purgeDone.Done()
		defer purgeTicker.Stop()
		monitorPurge(ctx, deps.purger, deps.purgeRetention, deps.paramHistoryRetention, deps.snapshotKeepCount, purgeTicker.C, log)
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
			outbox.WithOutboxNotifier(deps.outboxNotifier),
		)
		outboxDone.Add(1)
		go func() {
			defer outboxDone.Done()
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

// monitorPurge runs until ctx is cancelled, permanently removing soft-deleted
// users and quinielas older than retention on each tick received from tickC.
// Errors are logged at Warn level and swallowed: a failed purge tick is retried
// on the next interval, so transient DB hiccups do not stop the worker.
//
// If purger is nil (e.g. in unit tests where the database is not available),
// the function returns immediately.
func monitorPurge(ctx context.Context, purger repository.Purger, retention, paramHistoryRetention time.Duration, snapshotKeepCount int, tickC <-chan time.Time, log *zap.Logger) {
	if purger == nil {
		return
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-tickC:
			olderThan := time.Now().Add(-retention)
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
			if n, err := purger.PurgeOldSnapshots(ctx, snapshotKeepCount); err != nil {
				log.Warn("worker: purge old snapshots failed", zap.Error(err))
			} else if n > 0 {
				log.Info("worker: purged old leaderboard snapshots", zap.Int64("count", n))
			}
			historyBefore := time.Now().Add(-paramHistoryRetention)
			if n, err := purger.PurgeOldParamHistory(ctx, historyBefore); err != nil {
				log.Warn("worker: purge param history failed", zap.Error(err))
			} else if n > 0 {
				log.Info("worker: purged old param history rows", zap.Int64("count", n))
			}
		}
	}
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
func monitorDLQ(ctx context.Context, rc *redis.Client, e election.LeaderElection, tickC <-chan time.Time, log *zap.Logger) {
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
				dlqKey := "dlq:" + string(et)
				n, err := rc.LLen(ctx, dlqKey).Result()
				if err != nil {
					log.Warn("worker: DLQ monitor: LLEN failed",
						zap.String("dlq_key", dlqKey),
						zap.Error(err),
					)
					continue
				}
				if n > 0 {
					log.Error("worker: dead-letter queue is non-empty - manual replay required",
						zap.String("dlq_key", dlqKey),
						zap.String("event_type", string(et)),
						zap.Int64("dlq_size", n),
					)
				} else {
					log.Debug("worker: DLQ monitor: queue is empty",
						zap.String("dlq_key", dlqKey),
					)
				}
			}
		}
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
