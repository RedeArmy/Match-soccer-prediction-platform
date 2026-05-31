package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

// paramSpec defines the expected metadata for a system parameter.
//
// isRuntime controls the service's cache TTL for this key:
//   - true  → 30 s runtime TTL; changes propagate within one window, no restart required.
//   - false → 5 min infrastructure TTL; a process restart is required to guarantee the
//     new value takes effect. The longer TTL also reflects that these params are
//     read once at startup and cached for the life of the process.
type paramSpec struct {
	key          string
	defaultValue string
	paramType    string
	category     string
	isRuntime    bool
}

// allParams is the authoritative list of every system parameter that must exist
// in the database. This list is derived from domain/constants.go and must stay
// synchronised with the migrations that seed system_params:
//   - 000051_sync_system_params_canonical        (22 base params)
//   - 000055_add_worker_messaging_audit_params   (+10)
//   - 000056_add_snapshot_keep_latest_param      (+1)
//   - 000058_seed_group_max_size_param           (+1)
//   - 000066_seed_scoring_bonus_params           (+2)
//   - 000073_add_payment_webhook_secrets_params  (+3)
//   - 000074_add_bank_transfer_amount_params     (+2)
//   - 000076_seed_payment_intent_ttl_param       (+1)
//   - 000078_sync_system_params_is_runtime       (canonical is_runtime sync)
//   - 000079_seed_rate_limit_params              (+2)
//   - 000080_seed_reliability_params             (+9)
//   - 000087_seed_notify_params                  (+11)
//   - 000088_seed_notify_sse_push_params         (+2)
//   - 000089_seed_notify_push_asset_params       (+2)
//   - 000090_seed_scheduler_timezone_param       (+1)
//   - 000092_seed_queue_depth_param              (+1)
//   - 000094_seed_locale_param                   (+1)
//   - 000097_seed_missing_system_params          (+18)
//   - 000099_seed_notify_template_push_params    (+3)
//   - 000102_seed_push_sub_retention_param       (+1)
//   - 000103_seed_notify_from_address_param      (+1)
//   - 000105_seed_push_digest_params             (+2)
//   - 000106_system_params_history              (+1)
//   - 000107_seed_scheduler_interval_params     (+5)
//   - 000108_seed_render_timeout_param          (+1)
//   - 000110_seed_notify_dlq_replay_params      (+4)
//   - 000111_seed_notify_outbox_params          (+5)
//   - 000112_seed_observability_alert_params    (+2)
//   - 000113_seed_phase7_infra_params           (+2)
//   - 000114_seed_cache_breaker_params          (+2)
//   - 000115_seed_param_history_retention_param (+1)
//   - 000121_seed_kyc_params                    (+10)
//   - 000124_seed_kyc_velocity_params           (+4)
//   - 000125_seed_kyc_cache_ttl_param           (+1)
//   - 000129_seed_kyc_ip_velocity_params        (+2)
//   - 000138_seed_scoring_chunk_size_param      (+1)
//   - 000142_seed_ip_rate_limit_params          (+4)
//   - 000144_seed_kyc_doc_retention_param       (+1)
var allParams = []paramSpec{
	// Scoring — runtime: re-read on every ScoreMatch call.
	{key: domain.ParamKeyScoringExactScore, defaultValue: strconv.Itoa(domain.PointsExactScore), paramType: "int", category: "scoring", isRuntime: true},
	{key: domain.ParamKeyScoringCorrectOutcome, defaultValue: strconv.Itoa(domain.PointsCorrectOutcome), paramType: "int", category: "scoring", isRuntime: true},
	{key: domain.ParamKeyScoringGoalDiff, defaultValue: strconv.Itoa(domain.PointsGoalDifference), paramType: "int", category: "scoring", isRuntime: true},
	{key: domain.ParamKeyScoringExtraTimeBonus, defaultValue: strconv.Itoa(domain.DefaultScoringExtraTimeBonus), paramType: "int", category: "scoring", isRuntime: true},
	{key: domain.ParamKeyScoringPenaltiesBonus, defaultValue: strconv.Itoa(domain.DefaultScoringPenaltiesBonus), paramType: "int", category: "scoring", isRuntime: true},
	// isRuntime=FALSE: read per ScoreMatch call but not hot-patched; restart required for new value.
	{key: domain.ParamKeyScoringUpdateChunkSize, defaultValue: strconv.Itoa(domain.DefaultScoringUpdateChunkSize), paramType: "int", category: "scoring", isRuntime: false},

	// Prediction — runtime: re-read on every prediction submit/update call.
	{key: domain.ParamKeyPredictionDeadlineMin, defaultValue: strconv.Itoa(int(domain.PredictionDeadlineOffset / time.Minute)), paramType: "int", category: "prediction", isRuntime: true},

	// Group — runtime: enforced at request time; tunable without restart.
	{key: domain.ParamKeyGroupMinMembers, defaultValue: strconv.Itoa(domain.MinMembersForActive), paramType: "int", category: "group", isRuntime: true},
	{key: domain.ParamKeyGroupMaxSize, defaultValue: strconv.Itoa(domain.MaxMembersPerGroup), paramType: "int", category: "group", isRuntime: true},
	{key: domain.ParamKeyGroupInviteCodeLength, defaultValue: strconv.Itoa(domain.DefaultGroupInviteCodeLength), paramType: "int", category: "group", isRuntime: true},

	// Conflict — runtime: conflict detection is read per-request.
	{key: domain.ParamKeyConflictStaleDays, defaultValue: strconv.Itoa(domain.DefaultConflictStaleDays), paramType: "int", category: "conflict", isRuntime: true},
	{key: domain.ParamKeyConflictMaxScan, defaultValue: strconv.Itoa(domain.DefaultConflictMaxScan), paramType: "int", category: "conflict", isRuntime: true},

	// Pagination — runtime: read on every paginated request.
	{key: domain.ParamKeyPaginationDefaultLimit, defaultValue: strconv.Itoa(domain.DefaultPaginationDefaultLimit), paramType: "int", category: "pagination", isRuntime: true},
	{key: domain.ParamKeyPaginationMaxLimit, defaultValue: strconv.Itoa(domain.DefaultPaginationMaxLimit), paramType: "int", category: "pagination", isRuntime: true},

	// Tournament — runtime.
	{key: domain.ParamKeyTournamentWinPoints, defaultValue: strconv.Itoa(domain.StandingsWinPoints), paramType: "int", category: "tournament", isRuntime: true},

	// Admin — runtime: tunable during high-load periods without restart.
	{key: domain.ParamKeyAdminBulkMaxItems, defaultValue: strconv.Itoa(domain.DefaultAdminBulkMaxItems), paramType: "int", category: "admin", isRuntime: true},

	// Cache TTLs.
	// match_ttl_seconds is not runtime: no mutation hook wired; restart required.
	// leaderboard_ttl_seconds is runtime: CachedRankingService.UpdateTTL hook.
	// dashboard_ttl_seconds is runtime: CachedAdminReadService hook.
	{key: domain.ParamKeyCacheMatchTTL, defaultValue: strconv.Itoa(domain.DefaultCacheMatchTTLSeconds), paramType: "int", category: "cache", isRuntime: false},
	{key: domain.ParamKeyCacheLeaderboardTTL, defaultValue: strconv.Itoa(domain.DefaultCacheLeaderboardTTLSeconds), paramType: "int", category: "cache", isRuntime: true},
	{key: domain.ParamKeyCacheDashboardTTLSeconds, defaultValue: strconv.Itoa(domain.DefaultCacheDashboardTTLSeconds), paramType: "int", category: "cache", isRuntime: true},

	// Infrastructure timeouts — not runtime: read once at process startup; restart required.
	{key: domain.ParamKeyAuditWriteTimeout, defaultValue: strconv.Itoa(domain.DefaultAuditWriteTimeoutSeconds), paramType: "int", category: "system", isRuntime: false},
	{key: domain.ParamKeyAuthValidationTimeout, defaultValue: strconv.Itoa(domain.DefaultAuthValidationTimeoutSeconds), paramType: "int", category: "system", isRuntime: false},
	{key: domain.ParamKeyPurgeRetentionDays, defaultValue: strconv.Itoa(domain.DefaultPurgeRetentionDays), paramType: "int", category: "system", isRuntime: false},

	// DLQ — not runtime: restart required.
	{key: domain.ParamKeyDLQSampleSize, defaultValue: strconv.Itoa(domain.DefaultDLQSampleSize), paramType: "int", category: "dlq", isRuntime: false},
	{key: domain.ParamKeyDLQReplayDefaultLimit, defaultValue: strconv.Itoa(domain.DefaultDLQReplayDefaultLimit), paramType: "int", category: "dlq", isRuntime: false},

	// Messaging / Redis Streams — not runtime: restart required.
	{key: domain.ParamKeyMessagingMaxRetries, defaultValue: strconv.Itoa(domain.DefaultMessagingMaxRetries), paramType: "int", category: "messaging", isRuntime: false},
	{key: domain.ParamKeyMessagingStreamMaxLen, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamMaxLen), paramType: "int", category: "messaging", isRuntime: false},
	{key: domain.ParamKeyMessagingStreamWorkerCount, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamWorkerCount), paramType: "int", category: "messaging", isRuntime: false},
	{key: domain.ParamKeyMessagingStreamReadBlockSec, defaultValue: strconv.Itoa(domain.DefaultMessagingStreamReadBlockSec), paramType: "int", category: "messaging", isRuntime: false},

	// Audit retry policy — not runtime: restart required.
	{key: domain.ParamKeyAuditMaxRetries, defaultValue: strconv.Itoa(domain.DefaultAuditMaxRetries), paramType: "int", category: "system", isRuntime: false},
	{key: domain.ParamKeyAuditRetryDelayMs, defaultValue: strconv.Itoa(domain.DefaultAuditRetryDelayMs), paramType: "int", category: "system", isRuntime: false},

	// Worker: snapshot generation — not runtime: worker restart required.
	{key: domain.ParamKeyWorkerSnapshotConcurrency, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotConcurrency), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSnapshotRetryBaseMs, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotRetryBaseMs), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSnapshotMaxAttempts, defaultValue: strconv.Itoa(domain.DefaultWorkerSnapshotMaxAttempts), paramType: "int", category: "worker", isRuntime: false},

	// Worker: background maintenance — not runtime: worker restart required.
	{key: domain.ParamKeyWorkerDLQMonitorIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerDLQMonitorIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerPurgeIntervalHours, defaultValue: strconv.Itoa(domain.DefaultWorkerPurgeIntervalHours), paramType: "int", category: "worker", isRuntime: false},

	// API request limits — not runtime: restart required.
	{key: domain.ParamKeyAPIBodySizeLimitBytes, defaultValue: strconv.Itoa(domain.DefaultAPIBodySizeLimitBytes), paramType: "int", category: "api", isRuntime: false},

	// API rate limiting — not runtime: LimiterStore is constructed once at startup; restart required.
	{key: domain.ParamKeyAPIRateLimitRatePerSec, defaultValue: strconv.Itoa(domain.DefaultAPIRateLimitRatePerSec), paramType: "int", category: "api", isRuntime: false},
	{key: domain.ParamKeyAPIRateLimitBurst, defaultValue: strconv.Itoa(domain.DefaultAPIRateLimitBurst), paramType: "int", category: "api", isRuntime: false},
	// Idempotency middleware — not runtime: TTL and key limit are fixed at server startup.
	{key: domain.ParamKeyAPIIdempotencyTTLHours, defaultValue: strconv.Itoa(domain.DefaultAPIIdempotencyTTLHours), paramType: "int", category: "api", isRuntime: false},
	{key: domain.ParamKeyAPIIdempotencyKeyMaxLen, defaultValue: strconv.Itoa(domain.DefaultAPIIdempotencyKeyMaxLen), paramType: "int", category: "api", isRuntime: false},

	// IP-based rate limiting (migration 000142) — not runtime: LimiterStores constructed at startup; restart required.
	// L1 global bucket: applied to all /api/v1 routes, one bucket per source IP.
	{key: domain.ParamKeyIPRateLimitGlobalRPS, defaultValue: strconv.Itoa(domain.DefaultIPRateLimitGlobalRPS), paramType: "int", category: "api", isRuntime: false},
	{key: domain.ParamKeyIPRateLimitGlobalBurst, defaultValue: strconv.Itoa(domain.DefaultIPRateLimitGlobalBurst), paramType: "int", category: "api", isRuntime: false},
	// L2 webhook bucket: applied to /webhooks/recurrente and /webhooks/paypal, tighter limit.
	{key: domain.ParamKeyIPRateLimitWebhookRPS, defaultValue: strconv.Itoa(domain.DefaultIPRateLimitWebhookRPS), paramType: "int", category: "api", isRuntime: false},
	{key: domain.ParamKeyIPRateLimitWebhookBurst, defaultValue: strconv.Itoa(domain.DefaultIPRateLimitWebhookBurst), paramType: "int", category: "api", isRuntime: false},

	// Snapshot retention — not runtime: worker restart required.
	{key: domain.ParamKeySnapshotKeepLatestCount, defaultValue: strconv.Itoa(domain.DefaultSnapshotKeepLatestCount), paramType: "int", category: "worker", isRuntime: false},

	// Circuit breaker: PayPal cert fetcher — not runtime: restart required.
	{key: domain.ParamKeyBreakerPaypalCertMaxFails, defaultValue: strconv.Itoa(domain.DefaultBreakerPaypalCertMaxFails), paramType: "int", category: "breaker", isRuntime: false},
	{key: domain.ParamKeyBreakerPaypalCertCooldownSec, defaultValue: strconv.Itoa(domain.DefaultBreakerPaypalCertCooldownSec), paramType: "int", category: "breaker", isRuntime: false},

	// Circuit breaker: file store — not runtime: restart required.
	{key: domain.ParamKeyBreakerFileStoreMaxFails, defaultValue: strconv.Itoa(domain.DefaultBreakerFileStoreMaxFails), paramType: "int", category: "breaker", isRuntime: false},
	{key: domain.ParamKeyBreakerFileStoreCooldownSec, defaultValue: strconv.Itoa(domain.DefaultBreakerFileStoreCooldownSec), paramType: "int", category: "breaker", isRuntime: false},

	// Circuit breaker: Redis cache — not runtime: restart required.
	{key: domain.ParamKeyBreakerCacheMaxFails, defaultValue: strconv.Itoa(domain.DefaultBreakerCacheMaxFails), paramType: "int", category: "breaker", isRuntime: false},
	{key: domain.ParamKeyBreakerCacheCooldownSec, defaultValue: strconv.Itoa(domain.DefaultBreakerCacheCooldownSec), paramType: "int", category: "breaker", isRuntime: false},

	// DB transaction retry policy — not runtime: restart required.
	{key: domain.ParamKeyTxRetryMaxAttempts, defaultValue: strconv.Itoa(domain.DefaultTxRetryMaxAttempts), paramType: "int", category: "repository", isRuntime: false},
	{key: domain.ParamKeyTxRetryBaseDelayMs, defaultValue: strconv.Itoa(domain.DefaultTxRetryBaseDelayMs), paramType: "int", category: "repository", isRuntime: false},
	{key: domain.ParamKeyTxRetryMaxDelayMs, defaultValue: strconv.Itoa(domain.DefaultTxRetryMaxDelayMs), paramType: "int", category: "repository", isRuntime: false},

	// Payment / balance — runtime: changes take effect within the 30 s cache window.
	{key: domain.ParamKeyPaymentMaxUploadBytes, defaultValue: strconv.Itoa(domain.DefaultPaymentMaxUploadBytes), paramType: "int", category: "payment", isRuntime: true},
	{key: domain.ParamKeyWithdrawalMinCents, defaultValue: strconv.Itoa(domain.DefaultWithdrawalMinCents), paramType: "int", category: "payment", isRuntime: true},
	{key: domain.ParamKeyWithdrawalMaxCents, defaultValue: strconv.Itoa(domain.DefaultWithdrawalMaxCents), paramType: "int", category: "payment", isRuntime: true},
	{key: domain.ParamKeyBankTransferMinAmountCents, defaultValue: strconv.Itoa(domain.DefaultBankTransferMinAmountCents), paramType: "int", category: "payment", isRuntime: true},
	{key: domain.ParamKeyBankTransferMaxAmountCents, defaultValue: strconv.Itoa(domain.DefaultBankTransferMaxAmountCents), paramType: "int", category: "payment", isRuntime: true},
	{key: domain.ParamKeyPaymentIntentTTLMinutes, defaultValue: strconv.Itoa(domain.DefaultPaymentIntentTTLMinutes), paramType: "int", category: "payment", isRuntime: true},

	// Notifications — runtime: thresholds and recipient list are tunable without restart.
	{key: domain.ParamKeyNotifyBankTransferStaleSec, defaultValue: strconv.Itoa(domain.DefaultNotifyBankTransferStaleSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyWithdrawalStaleSec, defaultValue: strconv.Itoa(domain.DefaultNotifyWithdrawalStaleSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyHighValueWithdrawalCents, defaultValue: strconv.Itoa(domain.DefaultNotifyHighValueWithdrawalCents), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPendingReminderIntervalSec, defaultValue: strconv.Itoa(domain.DefaultNotifyPendingReminderIntervalSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPredictionDeadlineLeadMin1, defaultValue: strconv.Itoa(domain.DefaultNotifyPredictionDeadlineLeadMin1), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPredictionDeadlineLeadMin2, defaultValue: strconv.Itoa(domain.DefaultNotifyPredictionDeadlineLeadMin2), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPredictionMissingLeadMin, defaultValue: strconv.Itoa(domain.DefaultNotifyPredictionMissingLeadMin), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyBankTransferQueueDepthThreshold, defaultValue: strconv.Itoa(domain.DefaultNotifyBankTransferQueueDepthThreshold), paramType: "int", category: "notify", isRuntime: true},
	// SSE and Web Push delivery tuning (Phase 2).
	{key: domain.ParamKeyNotifySSEHeartbeatIntervalSec, defaultValue: strconv.Itoa(domain.DefaultNotifySSEHeartbeatIntervalSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyWebPushTTLSec, defaultValue: strconv.Itoa(domain.DefaultNotifyWebPushTTLSec), paramType: "int", category: "notify", isRuntime: true},
	// String params — empty default is intentional: must be set by the operator before enabling notifications.
	{key: domain.ParamKeyNotifyAdminEmails, defaultValue: "", paramType: "string", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyWebPushVAPIDPublicKey, defaultValue: "", paramType: "string", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyWebPushVAPIDSubject, defaultValue: "", paramType: "string", category: "notify", isRuntime: true},
	// Web Push notification asset URLs (Phase 3); string params with non-empty defaults.
	{key: domain.ParamKeyNotifyPushIconURL, defaultValue: domain.DefaultNotifyPushIconURL, paramType: "string", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPushBadgeURL, defaultValue: domain.DefaultNotifyPushBadgeURL, paramType: "string", category: "notify", isRuntime: true},
	// Scheduler timezone (Phase 4 · Sprint 7); not runtime — worker restart required.
	{key: domain.ParamKeyNotifySchedulerTimezone, defaultValue: domain.DefaultNotifySchedulerTimezone, paramType: "string", category: "notify", isRuntime: false},
	// Default locale for all user-facing notification text; runtime — propagates within cache window.
	{key: domain.ParamKeyNotifyDefaultLocale, defaultValue: "es", paramType: "string", category: "notify", isRuntime: true},
	// Template cache and push payload limits (Phase 5 · migration 000099); runtime — propagate within cache window.
	{key: domain.ParamKeyNotifyTemplateCacheTTLSec, defaultValue: strconv.Itoa(domain.DefaultNotifyTemplateCacheTTLSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPushTitleMaxChars, defaultValue: strconv.Itoa(domain.DefaultNotifyPushTitleMaxChars), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyPushBodyMaxChars, defaultValue: strconv.Itoa(domain.DefaultNotifyPushBodyMaxChars), paramType: "int", category: "notify", isRuntime: true},
	// Push subscription pruning retention (migration 000102); runtime — takes effect on next daily prune run.
	{key: domain.ParamKeyNotifyPushSubRetentionDays, defaultValue: strconv.Itoa(domain.DefaultNotifyPushSubRetentionDays), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyFromAddress, defaultValue: "", paramType: "string", category: "notify", isRuntime: true},
	// Push digest gate (migration 000105); not runtime — worker restart required.
	{key: domain.ParamKeyNotifyPushDigestWindowSec, defaultValue: strconv.Itoa(domain.DefaultNotifyPushDigestWindowSec), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyPushDigestThreshold, defaultValue: strconv.Itoa(domain.DefaultNotifyPushDigestThreshold), paramType: "int", category: "notify", isRuntime: false},
	// Param history retention (migration 000106); not runtime — worker restart required.
	{key: domain.ParamKeySystemParamHistoryRetentionDays, defaultValue: strconv.Itoa(domain.DefaultSystemParamHistoryRetentionDays), paramType: "int", category: "system", isRuntime: false},

	// Notification scheduler polling intervals (migration 000107); not runtime — worker restart required.
	// Each param controls how often the notification scheduler fires the corresponding job.
	{key: domain.ParamKeyWorkerSchedPredDeadlineIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerSchedPredDeadlineIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSchedMatchResultIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerSchedMatchResultIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSchedPendingReminderIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerSchedPendingReminderIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSchedStaleEscalationIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerSchedStaleEscalationIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	{key: domain.ParamKeyWorkerSchedPushPruneIntervalSec, defaultValue: strconv.Itoa(domain.DefaultWorkerSchedPushPruneIntervalSec), paramType: "int", category: "worker", isRuntime: false},
	// Email render timeout (migration 000108); runtime — takes effect within 30 s cache window.
	{key: domain.ParamKeyNotifyRenderTimeoutMs, defaultValue: strconv.Itoa(domain.DefaultNotifyRenderTimeoutMs), paramType: "int", category: "notify", isRuntime: true},

	// Notification DLQ replay worker (migration 000110); not runtime — worker restart required.
	{key: domain.ParamKeyNotifyDLQReplayBatchSize, defaultValue: strconv.Itoa(domain.DefaultNotifyDLQReplayBatchSize), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyDLQReplayPollIntervalSec, defaultValue: strconv.Itoa(domain.DefaultNotifyDLQReplayPollIntervalSec), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyDLQReplayMaxAttempts, defaultValue: strconv.Itoa(domain.DefaultNotifyDLQReplayMaxAttempts), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyDLQReplayAlertThreshold, defaultValue: strconv.Itoa(domain.DefaultNotifyDLQReplayAlertThreshold), paramType: "int", category: "notify", isRuntime: false},

	// Notification outbox dispatch worker (migration 000111); not runtime — worker restart required.
	{key: domain.ParamKeyNotifyOutboxBatchSize, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxBatchSize), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyOutboxPollIntervalSec, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxPollIntervalSec), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyOutboxLockDurationSec, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxLockDurationSec), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyOutboxMaxAttempts, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxMaxAttempts), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyOutboxLagAlertThresholdSec, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxLagAlertThresholdSec), paramType: "int", category: "notify", isRuntime: false},

	// Observability alerting thresholds (migration 000112); runtime — adjustable without restart
	// to tune Prometheus alert sensitivity without rolling a new worker build.
	{key: domain.ParamKeyNotifyOutboxLagCriticalSec, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxLagCriticalSec), paramType: "int", category: "notify", isRuntime: true},
	{key: domain.ParamKeyNotifyDLQWarningThreshold, defaultValue: strconv.Itoa(domain.DefaultNotifyDLQWarningThreshold), paramType: "int", category: "notify", isRuntime: true},

	// Phase 7 infrastructure params (migration 000113); not runtime — restart required.
	{key: domain.ParamKeyNotifySSEChanBufSize, defaultValue: strconv.Itoa(domain.DefaultNotifySSEChanBufSize), paramType: "int", category: "notify", isRuntime: false},
	// Per-user SSE connection cap (migration 000136); not runtime — hub rebuilt at startup.
	// 0 = unlimited; default 5 allows multi-tab/device without unbounded heap growth.
	{key: domain.ParamKeyNotifySSEMaxConnsPerUser, defaultValue: strconv.Itoa(domain.DefaultNotifySSEMaxConnsPerUser), paramType: "int", category: "notify", isRuntime: false},
	{key: domain.ParamKeyNotifyOutboxStaleLockThresholdSec, defaultValue: strconv.Itoa(domain.DefaultNotifyOutboxStaleLockThresholdSec), paramType: "int", category: "notify", isRuntime: false},

	// KYC/AML gate params (migrations 000121 + 000125); runtime — all limits are enforced
	// per-request by KYCGate and propagate within the 30 s cache window.
	{key: domain.ParamKeyKYCTier1DepositLimitCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier1DepositLimitCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier2DepositLimitCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier2DepositLimitCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier2PayoutLimitCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier2PayoutLimitCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCAMLThresholdCents, defaultValue: strconv.Itoa(domain.DefaultKYCAMLThresholdCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCReviewIntervalDays, defaultValue: strconv.Itoa(domain.DefaultKYCReviewIntervalDays), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCMaxDocUploadBytes, defaultValue: strconv.Itoa(domain.DefaultKYCMaxDocUploadBytes), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier1DepositVelocityCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier1DepositVelocityCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier2DepositVelocityCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier2DepositVelocityCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier1WithdrawalVelocityCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier1WithdrawalVelocityCents), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCTier2WithdrawalVelocityCents, defaultValue: strconv.Itoa(domain.DefaultKYCTier2WithdrawalVelocityCents), paramType: "int", category: "kyc", isRuntime: true},
	// kyc.risk_dashboard_cache_ttl_sec has a mutation hook wired in migration 000125.
	{key: domain.ParamKeyKYCRiskDashboardCacheTTLSec, defaultValue: strconv.Itoa(domain.DefaultKYCRiskDashboardCacheTTLSecs), paramType: "int", category: "kyc", isRuntime: true},
	// KYC IP velocity (migration 000129); runtime — enforced per-request by CheckIPSubmissionVelocity.
	{key: domain.ParamKeyKYCIPVelocityWindowMinutes, defaultValue: strconv.Itoa(domain.DefaultKYCIPVelocityWindowMinutes), paramType: "int", category: "kyc", isRuntime: true},
	{key: domain.ParamKeyKYCIPVelocityMaxSubmissions, defaultValue: strconv.Itoa(domain.DefaultKYCIPVelocityMaxSubmissions), paramType: "int", category: "kyc", isRuntime: true},
	// KYC document retention (migration 000144); not runtime — worker restart required to pick up changes.
	{key: domain.ParamKeyKYCDocRetentionYears, defaultValue: strconv.Itoa(domain.DefaultKYCDocRetentionYears), paramType: "int", category: "kyc", isRuntime: false},
}

type dbParam struct {
	key          string
	value        string
	defaultValue string
	paramType    string
	category     string
	isRuntime    bool
	description  string
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	ctx := context.Background()

	pool, err := connectDatabase()
	if err != nil {
		return err
	}
	defer pool.Close()

	dbParams, err := fetchAllParams(ctx, pool)
	if err != nil {
		return fmt.Errorf("failed to fetch system_params: %w", err)
	}

	return validateFromParams(dbParams)
}

// validateFromParams runs the full in-memory validation pipeline against a
// pre-fetched slice of database params. Extracted from run so it can be unit
// tested without a live database connection.
func validateFromParams(dbParams []dbParam) error {
	dbMap := buildParamMap(dbParams)
	errors := validateAllParams(dbMap)
	checkUnexpectedParams(dbParams)
	return reportResults(errors)
}

func connectDatabase() (*pgxpool.Pool, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}

	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	return pool, nil
}

func buildParamMap(dbParams []dbParam) map[string]dbParam {
	dbMap := make(map[string]dbParam, len(dbParams))
	for _, p := range dbParams {
		dbMap[p.key] = p
	}
	return dbMap
}

func validateAllParams(dbMap map[string]dbParam) []string {
	var errors []string
	for _, spec := range allParams {
		errs := validateSingleParam(spec, dbMap)
		errors = append(errors, errs...)
	}
	return errors
}

func validateSingleParam(spec paramSpec, dbMap map[string]dbParam) []string {
	db, exists := dbMap[spec.key]
	if !exists {
		return []string{fmt.Sprintf("❌ MISSING: %s (expected default: %s)", spec.key, spec.defaultValue)}
	}

	var errors []string
	errors = append(errors, validateType(spec, db)...)
	errors = append(errors, validateCategory(spec, db)...)
	errors = append(errors, validateIsRuntime(spec, db)...)
	errors = append(errors, validateDescription(spec, db)...)
	errors = append(errors, validateDefaultValue(spec, db)...)

	checkValueOverride(spec, db)
	printValidParam(spec, db)

	return errors
}

func validateDefaultValue(spec paramSpec, db dbParam) []string {
	if db.defaultValue != spec.defaultValue {
		return []string{fmt.Sprintf("❌ DEFAULT VALUE MISMATCH: %s (code: %s, db.default_value: %s) — migration seed may be out of sync with domain constants", spec.key, spec.defaultValue, db.defaultValue)}
	}
	return nil
}

func validateType(spec paramSpec, db dbParam) []string {
	if db.paramType != spec.paramType {
		return []string{fmt.Sprintf("❌ TYPE MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.paramType, db.paramType)}
	}
	return nil
}

func validateCategory(spec paramSpec, db dbParam) []string {
	if db.category != spec.category {
		return []string{fmt.Sprintf("❌ CATEGORY MISMATCH: %s (expected: %s, got: %s)", spec.key, spec.category, db.category)}
	}
	return nil
}

func validateIsRuntime(spec paramSpec, db dbParam) []string {
	if db.isRuntime != spec.isRuntime {
		return []string{fmt.Sprintf(
			"❌ IS_RUNTIME MISMATCH: %s (expected: %v, got: %v) — "+
				"migration 000078 should have corrected this; re-run migrations",
			spec.key, spec.isRuntime, db.isRuntime,
		)}
	}
	return nil
}

func validateDescription(spec paramSpec, db dbParam) []string {
	if db.description == "" {
		return []string{fmt.Sprintf("❌ MISSING DESCRIPTION: %s", spec.key)}
	}
	return nil
}

func checkValueOverride(spec paramSpec, db dbParam) {
	if db.value != spec.defaultValue {
		fmt.Printf("⚠️  VALUE OVERRIDE: %s (code default: %s, DB value: %s) — operator override detected\n",
			spec.key, spec.defaultValue, db.value)
	}
}

func printValidParam(spec paramSpec, db dbParam) {
	fmt.Printf("✅ %s = %s (%s, %s)\n", spec.key, db.value, db.paramType, db.category)
}

func checkUnexpectedParams(dbParams []dbParam) {
	expectedKeys := buildExpectedKeysSet()
	for _, db := range dbParams {
		if !expectedKeys[db.key] {
			fmt.Printf("⚠️  UNEXPECTED PARAM IN DB: %s (not defined in constants.go) — consider removing or documenting\n", db.key)
		}
	}
}

func buildExpectedKeysSet() map[string]bool {
	expected := make(map[string]bool, len(allParams))
	for _, spec := range allParams {
		expected[spec.key] = true
	}
	return expected
}

func reportResults(errors []string) error {
	if len(errors) > 0 {
		fmt.Println("\n❌ VALIDATION FAILED:")
		for _, err := range errors {
			fmt.Println(err)
		}
		return fmt.Errorf("system_params validation failed with %d error(s)", len(errors))
	}

	fmt.Printf("\n✅ VALIDATION PASSED: All %d system parameters are correctly configured\n", len(allParams))
	return nil
}

func fetchAllParams(ctx context.Context, pool *pgxpool.Pool) ([]dbParam, error) {
	rows, err := pool.Query(ctx, `
		SELECT key, value, default_value, type, category, is_runtime, COALESCE(description, '') as description
		FROM system_params
		ORDER BY category, key
	`)
	if err != nil {
		return nil, fmt.Errorf("query system_params: %w", err)
	}
	defer rows.Close()

	var params []dbParam
	for rows.Next() {
		var p dbParam
		if err := rows.Scan(&p.key, &p.value, &p.defaultValue, &p.paramType, &p.category, &p.isRuntime, &p.description); err != nil {
			return nil, fmt.Errorf("scan system_param row: %w", err)
		}
		params = append(params, p)
	}

	if err := rows.Err(); err != nil {
		return params, fmt.Errorf("fetch system_params: %w", err)
	}
	return params, nil
}
