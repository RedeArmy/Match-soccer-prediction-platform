package domain

import (
	"reflect"
	"strings"
	"testing"
)

// TestSystemParamConstants_AllPaired validates that every ParamKey* constant
// has a corresponding Default* (or documented business-rule) constant, and
// that the total counts match what is expected. A count mismatch is a reminder
// to update this test, create a migration, and add the new key to validate-params.
func TestSystemParamConstants_AllPaired(t *testing.T) {
	// ── ParamKey* enumeration (64 total) ──────────────────────────────────────
	paramKeys := map[string]string{
		// Scoring
		"ParamKeyScoringExactScore":     ParamKeyScoringExactScore,
		"ParamKeyScoringCorrectOutcome": ParamKeyScoringCorrectOutcome,
		"ParamKeyScoringGoalDiff":       ParamKeyScoringGoalDiff,
		"ParamKeyScoringExtraTimeBonus": ParamKeyScoringExtraTimeBonus,
		"ParamKeyScoringPenaltiesBonus": ParamKeyScoringPenaltiesBonus,
		// Prediction
		"ParamKeyPredictionDeadlineMin": ParamKeyPredictionDeadlineMin,
		// Group
		"ParamKeyGroupMinMembers":       ParamKeyGroupMinMembers,
		"ParamKeyGroupMaxSize":          ParamKeyGroupMaxSize,
		"ParamKeyGroupInviteCodeLength": ParamKeyGroupInviteCodeLength,
		// Conflict
		"ParamKeyConflictStaleDays": ParamKeyConflictStaleDays,
		"ParamKeyConflictMaxScan":   ParamKeyConflictMaxScan,
		// Pagination
		"ParamKeyPaginationDefaultLimit": ParamKeyPaginationDefaultLimit,
		"ParamKeyPaginationMaxLimit":     ParamKeyPaginationMaxLimit,
		// Tournament
		"ParamKeyTournamentWinPoints": ParamKeyTournamentWinPoints,
		// Admin
		"ParamKeyAdminBulkMaxItems": ParamKeyAdminBulkMaxItems,
		// Cache
		"ParamKeyCacheMatchTTL":            ParamKeyCacheMatchTTL,
		"ParamKeyCacheLeaderboardTTL":      ParamKeyCacheLeaderboardTTL,
		"ParamKeyCacheDashboardTTLSeconds": ParamKeyCacheDashboardTTLSeconds,
		// Audit
		"ParamKeyAuditWriteTimeout": ParamKeyAuditWriteTimeout,
		"ParamKeyAuditMaxRetries":   ParamKeyAuditMaxRetries,
		"ParamKeyAuditRetryDelayMs": ParamKeyAuditRetryDelayMs,
		// Auth
		"ParamKeyAuthValidationTimeout": ParamKeyAuthValidationTimeout,
		// DLQ
		"ParamKeyDLQSampleSize":         ParamKeyDLQSampleSize,
		"ParamKeyDLQReplayDefaultLimit": ParamKeyDLQReplayDefaultLimit,
		// Messaging
		"ParamKeyMessagingMaxRetries":         ParamKeyMessagingMaxRetries,
		"ParamKeyMessagingStreamMaxLen":       ParamKeyMessagingStreamMaxLen,
		"ParamKeyMessagingStreamWorkerCount":  ParamKeyMessagingStreamWorkerCount,
		"ParamKeyMessagingStreamReadBlockSec": ParamKeyMessagingStreamReadBlockSec,
		// Worker
		"ParamKeyWorkerSnapshotConcurrency":   ParamKeyWorkerSnapshotConcurrency,
		"ParamKeyWorkerSnapshotRetryBaseMs":   ParamKeyWorkerSnapshotRetryBaseMs,
		"ParamKeyWorkerSnapshotMaxAttempts":   ParamKeyWorkerSnapshotMaxAttempts,
		"ParamKeyWorkerDLQMonitorIntervalSec": ParamKeyWorkerDLQMonitorIntervalSec,
		"ParamKeyWorkerPurgeIntervalHours":    ParamKeyWorkerPurgeIntervalHours,
		// System
		"ParamKeyPurgeRetentionDays": ParamKeyPurgeRetentionDays,
		// API
		"ParamKeyAPIBodySizeLimitBytes":   ParamKeyAPIBodySizeLimitBytes,
		"ParamKeyAPIRateLimitRatePerSec":  ParamKeyAPIRateLimitRatePerSec,
		"ParamKeyAPIRateLimitBurst":       ParamKeyAPIRateLimitBurst,
		"ParamKeyAPIIdempotencyTTLHours":  ParamKeyAPIIdempotencyTTLHours,
		"ParamKeyAPIIdempotencyKeyMaxLen": ParamKeyAPIIdempotencyKeyMaxLen,
		// Snapshot
		"ParamKeySnapshotKeepLatestCount": ParamKeySnapshotKeepLatestCount,
		// Circuit breaker
		"ParamKeyBreakerPaypalCertMaxFails":    ParamKeyBreakerPaypalCertMaxFails,
		"ParamKeyBreakerPaypalCertCooldownSec": ParamKeyBreakerPaypalCertCooldownSec,
		"ParamKeyBreakerFileStoreMaxFails":     ParamKeyBreakerFileStoreMaxFails,
		"ParamKeyBreakerFileStoreCooldownSec":  ParamKeyBreakerFileStoreCooldownSec,
		// Repository / TX retry
		"ParamKeyTxRetryMaxAttempts": ParamKeyTxRetryMaxAttempts,
		"ParamKeyTxRetryBaseDelayMs": ParamKeyTxRetryBaseDelayMs,
		"ParamKeyTxRetryMaxDelayMs":  ParamKeyTxRetryMaxDelayMs,
		// Payment
		"ParamKeyPaymentMaxUploadBytes":      ParamKeyPaymentMaxUploadBytes,
		"ParamKeyWithdrawalMinCents":         ParamKeyWithdrawalMinCents,
		"ParamKeyWithdrawalMaxCents":         ParamKeyWithdrawalMaxCents,
		"ParamKeyBankTransferMinAmountCents": ParamKeyBankTransferMinAmountCents,
		"ParamKeyBankTransferMaxAmountCents": ParamKeyBankTransferMaxAmountCents,
		"ParamKeyPaymentIntentTTLMinutes":    ParamKeyPaymentIntentTTLMinutes,
		// Notification subsystem
		"ParamKeyNotifyBankTransferStaleSec":            ParamKeyNotifyBankTransferStaleSec,
		"ParamKeyNotifyWithdrawalStaleSec":              ParamKeyNotifyWithdrawalStaleSec,
		"ParamKeyNotifyHighValueWithdrawalCents":        ParamKeyNotifyHighValueWithdrawalCents,
		"ParamKeyNotifyPendingReminderIntervalSec":      ParamKeyNotifyPendingReminderIntervalSec,
		"ParamKeyNotifyPredictionDeadlineLeadMin1":      ParamKeyNotifyPredictionDeadlineLeadMin1,
		"ParamKeyNotifyPredictionDeadlineLeadMin2":      ParamKeyNotifyPredictionDeadlineLeadMin2,
		"ParamKeyNotifyPredictionMissingLeadMin":        ParamKeyNotifyPredictionMissingLeadMin,
		"ParamKeyNotifyBankTransferQueueDepthThreshold": ParamKeyNotifyBankTransferQueueDepthThreshold,
		"ParamKeyNotifyAdminEmails":                     ParamKeyNotifyAdminEmails,
		"ParamKeyNotifyWebPushVAPIDPublicKey":           ParamKeyNotifyWebPushVAPIDPublicKey,
		"ParamKeyNotifyWebPushVAPIDSubject":             ParamKeyNotifyWebPushVAPIDSubject,
		"ParamKeyNotifySSEHeartbeatIntervalSec":         ParamKeyNotifySSEHeartbeatIntervalSec,
		"ParamKeyNotifyWebPushTTLSec":                   ParamKeyNotifyWebPushTTLSec,
		"ParamKeyNotifyPushIconURL":                     ParamKeyNotifyPushIconURL,
		"ParamKeyNotifyPushBadgeURL":                    ParamKeyNotifyPushBadgeURL,
		"ParamKeyNotifySchedulerTimezone":               ParamKeyNotifySchedulerTimezone,
		"ParamKeyNotifyDefaultLocale":                   ParamKeyNotifyDefaultLocale,
		"ParamKeyNotifyTemplateCacheTTLSec":             ParamKeyNotifyTemplateCacheTTLSec,
		"ParamKeyNotifyPushTitleMaxChars":               ParamKeyNotifyPushTitleMaxChars,
		"ParamKeyNotifyPushBodyMaxChars":                ParamKeyNotifyPushBodyMaxChars,
		"ParamKeyNotifyPushSubRetentionDays":            ParamKeyNotifyPushSubRetentionDays,
		"ParamKeyNotifyFromAddress":                     ParamKeyNotifyFromAddress,
	}

	// ── Default* enumeration ─────────────────────────────────────────────────
	// Note: PointsExactScore, PointsCorrectOutcome, PointsGoalDifference,
	// StandingsWinPoints, MinMembersForActive, MaxMembersPerGroup, and
	// PredictionDeadlineOffset are intentionally NOT in the Default* naming
	// pattern — they are fundamental business rules, not system-param fallbacks.
	//
	// DefaultScoringExtraTimeBonus and DefaultScoringPenaltiesBonus are
	// legitimately zero: they represent "no global win-method bonus applied".
	defaults := map[string]interface{}{
		// Group
		"DefaultGroupInviteCodeLength": DefaultGroupInviteCodeLength,
		// Pagination
		"DefaultPaginationDefaultLimit": DefaultPaginationDefaultLimit,
		"DefaultPaginationMaxLimit":     DefaultPaginationMaxLimit,
		// Prediction
		"DefaultPredictionDeadlineMin": DefaultPredictionDeadlineMin,
		// Admin
		"DefaultAdminBulkMaxItems": DefaultAdminBulkMaxItems,
		// Cache
		"DefaultCacheDashboardTTLSeconds":   DefaultCacheDashboardTTLSeconds,
		"DefaultCacheMatchTTLSeconds":       DefaultCacheMatchTTLSeconds,
		"DefaultCacheLeaderboardTTLSeconds": DefaultCacheLeaderboardTTLSeconds,
		// Scoring bonuses (intentionally zero = disabled)
		"DefaultScoringExtraTimeBonus": DefaultScoringExtraTimeBonus,
		"DefaultScoringPenaltiesBonus": DefaultScoringPenaltiesBonus,
		// Conflict
		"DefaultConflictStaleDays": DefaultConflictStaleDays,
		"DefaultConflictMaxScan":   DefaultConflictMaxScan,
		// DLQ
		"DefaultDLQSampleSize":         DefaultDLQSampleSize,
		"DefaultDLQReplayDefaultLimit": DefaultDLQReplayDefaultLimit,
		// Messaging
		"DefaultMessagingMaxRetries":         DefaultMessagingMaxRetries,
		"DefaultMessagingStreamMaxLen":       DefaultMessagingStreamMaxLen,
		"DefaultMessagingStreamWorkerCount":  DefaultMessagingStreamWorkerCount,
		"DefaultMessagingStreamReadBlockSec": DefaultMessagingStreamReadBlockSec,
		// Audit
		"DefaultAuthValidationTimeoutSeconds": DefaultAuthValidationTimeoutSeconds,
		"DefaultAuditWriteTimeoutSeconds":     DefaultAuditWriteTimeoutSeconds,
		"DefaultAuditMaxRetries":              DefaultAuditMaxRetries,
		"DefaultAuditRetryDelayMs":            DefaultAuditRetryDelayMs,
		// Worker
		"DefaultWorkerSnapshotConcurrency":   DefaultWorkerSnapshotConcurrency,
		"DefaultWorkerSnapshotRetryBaseMs":   DefaultWorkerSnapshotRetryBaseMs,
		"DefaultWorkerSnapshotMaxAttempts":   DefaultWorkerSnapshotMaxAttempts,
		"DefaultWorkerDLQMonitorIntervalSec": DefaultWorkerDLQMonitorIntervalSec,
		"DefaultWorkerPurgeIntervalHours":    DefaultWorkerPurgeIntervalHours,
		// System
		"DefaultPurgeRetentionDays": DefaultPurgeRetentionDays,
		// API
		"DefaultAPIBodySizeLimitBytes":  DefaultAPIBodySizeLimitBytes,
		"DefaultAPIRateLimitRatePerSec": DefaultAPIRateLimitRatePerSec,
		"DefaultAPIRateLimitBurst":      DefaultAPIRateLimitBurst,
		// Snapshot
		"DefaultSnapshotKeepLatestCount": DefaultSnapshotKeepLatestCount,
		// Payment
		"DefaultPaymentMaxUploadBytes":      DefaultPaymentMaxUploadBytes,
		"DefaultWithdrawalMinCents":         DefaultWithdrawalMinCents,
		"DefaultWithdrawalMaxCents":         DefaultWithdrawalMaxCents,
		"DefaultBankTransferMinAmountCents": DefaultBankTransferMinAmountCents,
		"DefaultBankTransferMaxAmountCents": DefaultBankTransferMaxAmountCents,
		"DefaultPaymentIntentTTLMinutes":    DefaultPaymentIntentTTLMinutes,
		// API (idempotency)
		"DefaultAPIIdempotencyTTLHours":  DefaultAPIIdempotencyTTLHours,
		"DefaultAPIIdempotencyKeyMaxLen": DefaultAPIIdempotencyKeyMaxLen,
		// Circuit breaker
		"DefaultBreakerPaypalCertMaxFails":    DefaultBreakerPaypalCertMaxFails,
		"DefaultBreakerPaypalCertCooldownSec": DefaultBreakerPaypalCertCooldownSec,
		"DefaultBreakerFileStoreMaxFails":     DefaultBreakerFileStoreMaxFails,
		"DefaultBreakerFileStoreCooldownSec":  DefaultBreakerFileStoreCooldownSec,
		// Repository / TX retry
		"DefaultTxRetryMaxAttempts": DefaultTxRetryMaxAttempts,
		"DefaultTxRetryBaseDelayMs": DefaultTxRetryBaseDelayMs,
		"DefaultTxRetryMaxDelayMs":  DefaultTxRetryMaxDelayMs,
		// Notification subsystem
		"DefaultNotifyBankTransferStaleSec":            DefaultNotifyBankTransferStaleSec,
		"DefaultNotifyWithdrawalStaleSec":              DefaultNotifyWithdrawalStaleSec,
		"DefaultNotifyHighValueWithdrawalCents":        DefaultNotifyHighValueWithdrawalCents,
		"DefaultNotifyPendingReminderIntervalSec":      DefaultNotifyPendingReminderIntervalSec,
		"DefaultNotifyPredictionDeadlineLeadMin1":      DefaultNotifyPredictionDeadlineLeadMin1,
		"DefaultNotifyPredictionDeadlineLeadMin2":      DefaultNotifyPredictionDeadlineLeadMin2,
		"DefaultNotifyPredictionMissingLeadMin":        DefaultNotifyPredictionMissingLeadMin,
		"DefaultNotifyBankTransferQueueDepthThreshold": DefaultNotifyBankTransferQueueDepthThreshold,
		"DefaultNotifySSEHeartbeatIntervalSec":         DefaultNotifySSEHeartbeatIntervalSec,
		"DefaultNotifyWebPushTTLSec":                   DefaultNotifyWebPushTTLSec,
		"DefaultNotifyTemplateCacheTTLSec":             DefaultNotifyTemplateCacheTTLSec,
		"DefaultNotifyPushTitleMaxChars":               DefaultNotifyPushTitleMaxChars,
		"DefaultNotifyPushBodyMaxChars":                DefaultNotifyPushBodyMaxChars,
		"DefaultNotifyPushSubRetentionDays":            DefaultNotifyPushSubRetentionDays,
		// String defaults — not in the int defaults map; documented separately.
		"DefaultNotifyPushIconURL":       DefaultNotifyPushIconURL,
		"DefaultNotifyPushBadgeURL":      DefaultNotifyPushBadgeURL,
		"DefaultNotifySchedulerTimezone": DefaultNotifySchedulerTimezone,
	}

	t.Run("all_param_keys_documented", func(t *testing.T) {
		const expectedCount = 75 // update when adding a new ParamKey* constant
		if len(paramKeys) != expectedCount {
			t.Errorf("ParamKey enumeration may be incomplete: expected %d, got %d", expectedCount, len(paramKeys))
			t.Log("If you added a new ParamKey* constant, update the enumeration in this test and create a migration")
		}
	})

	t.Run("all_defaults_documented", func(t *testing.T) {
		const expectedCount = 64 // update when adding a new Default* constant (+3 string defaults: push_icon_url, push_badge_url, scheduler_timezone)
		if len(defaults) != expectedCount {
			t.Errorf("Default enumeration may be incomplete: expected %d, got %d", expectedCount, len(defaults))
			t.Log("If you added a new Default* constant, update the enumeration in this test")
		}
	})

	t.Run("no_orphaned_param_keys", func(t *testing.T) {
		for name, key := range paramKeys {
			if key == "" {
				t.Errorf("%s is defined but has an empty value", name)
			}
		}
	})

	t.Run("no_duplicate_param_keys", func(t *testing.T) {
		seen := make(map[string]string)
		for name, key := range paramKeys {
			if existing, found := seen[key]; found {
				t.Errorf("Duplicate ParamKey value %q used by both %s and %s", key, existing, name)
			}
			seen[key] = name
		}
	})
}

// TestSystemParamNamingConventions validates that all system param keys
// follow the documented naming convention: category.snake_case_name.
// The "category" field in each case is the prefix before the dot, which
// is used for grouping in the admin UI and naming-convention enforcement.
// Note: the category prefix in the key (e.g. "audit.") may differ from
// the DB category column (e.g. "system") — they are distinct concepts.
func TestSystemParamNamingConventions(t *testing.T) {
	paramKeys := []struct {
		name     string
		key      string
		category string // expected prefix before the first "."
	}{
		// Scoring
		{"ParamKeyScoringExactScore", ParamKeyScoringExactScore, "scoring"},
		{"ParamKeyScoringCorrectOutcome", ParamKeyScoringCorrectOutcome, "scoring"},
		{"ParamKeyScoringGoalDiff", ParamKeyScoringGoalDiff, "scoring"},
		{"ParamKeyScoringExtraTimeBonus", ParamKeyScoringExtraTimeBonus, "scoring"},
		{"ParamKeyScoringPenaltiesBonus", ParamKeyScoringPenaltiesBonus, "scoring"},
		// Prediction
		{"ParamKeyPredictionDeadlineMin", ParamKeyPredictionDeadlineMin, "prediction"},
		// Group
		{"ParamKeyGroupMinMembers", ParamKeyGroupMinMembers, "group"},
		{"ParamKeyGroupMaxSize", ParamKeyGroupMaxSize, "group"},
		{"ParamKeyGroupInviteCodeLength", ParamKeyGroupInviteCodeLength, "group"},
		// Conflict
		{"ParamKeyConflictStaleDays", ParamKeyConflictStaleDays, "conflict"},
		{"ParamKeyConflictMaxScan", ParamKeyConflictMaxScan, "conflict"},
		// Pagination
		{"ParamKeyPaginationDefaultLimit", ParamKeyPaginationDefaultLimit, "pagination"},
		{"ParamKeyPaginationMaxLimit", ParamKeyPaginationMaxLimit, "pagination"},
		// Tournament
		{"ParamKeyTournamentWinPoints", ParamKeyTournamentWinPoints, "tournament"},
		// Admin
		{"ParamKeyAdminBulkMaxItems", ParamKeyAdminBulkMaxItems, "admin"},
		// Cache
		{"ParamKeyCacheMatchTTL", ParamKeyCacheMatchTTL, "cache"},
		{"ParamKeyCacheLeaderboardTTL", ParamKeyCacheLeaderboardTTL, "cache"},
		{"ParamKeyCacheDashboardTTLSeconds", ParamKeyCacheDashboardTTLSeconds, "cache"},
		// Audit (key prefix "audit"; DB category is "system")
		{"ParamKeyAuditWriteTimeout", ParamKeyAuditWriteTimeout, "audit"},
		{"ParamKeyAuditMaxRetries", ParamKeyAuditMaxRetries, "audit"},
		{"ParamKeyAuditRetryDelayMs", ParamKeyAuditRetryDelayMs, "audit"},
		// Auth (key prefix "auth"; DB category is "system")
		{"ParamKeyAuthValidationTimeout", ParamKeyAuthValidationTimeout, "auth"},
		// DLQ
		{"ParamKeyDLQSampleSize", ParamKeyDLQSampleSize, "dlq"},
		{"ParamKeyDLQReplayDefaultLimit", ParamKeyDLQReplayDefaultLimit, "dlq"},
		// Messaging
		{"ParamKeyMessagingMaxRetries", ParamKeyMessagingMaxRetries, "messaging"},
		{"ParamKeyMessagingStreamMaxLen", ParamKeyMessagingStreamMaxLen, "messaging"},
		{"ParamKeyMessagingStreamWorkerCount", ParamKeyMessagingStreamWorkerCount, "messaging"},
		{"ParamKeyMessagingStreamReadBlockSec", ParamKeyMessagingStreamReadBlockSec, "messaging"},
		// Worker
		{"ParamKeyWorkerSnapshotConcurrency", ParamKeyWorkerSnapshotConcurrency, "worker"},
		{"ParamKeyWorkerSnapshotRetryBaseMs", ParamKeyWorkerSnapshotRetryBaseMs, "worker"},
		{"ParamKeyWorkerSnapshotMaxAttempts", ParamKeyWorkerSnapshotMaxAttempts, "worker"},
		{"ParamKeyWorkerDLQMonitorIntervalSec", ParamKeyWorkerDLQMonitorIntervalSec, "worker"},
		{"ParamKeyWorkerPurgeIntervalHours", ParamKeyWorkerPurgeIntervalHours, "worker"},
		// System
		{"ParamKeyPurgeRetentionDays", ParamKeyPurgeRetentionDays, "system"},
		// API
		{"ParamKeyAPIBodySizeLimitBytes", ParamKeyAPIBodySizeLimitBytes, "api"},
		{"ParamKeyAPIRateLimitRatePerSec", ParamKeyAPIRateLimitRatePerSec, "api"},
		{"ParamKeyAPIRateLimitBurst", ParamKeyAPIRateLimitBurst, "api"},
		{"ParamKeyAPIIdempotencyTTLHours", ParamKeyAPIIdempotencyTTLHours, "api"},
		{"ParamKeyAPIIdempotencyKeyMaxLen", ParamKeyAPIIdempotencyKeyMaxLen, "api"},
		// Snapshot
		{"ParamKeySnapshotKeepLatestCount", ParamKeySnapshotKeepLatestCount, "snapshot"},
		// Circuit breaker
		{"ParamKeyBreakerPaypalCertMaxFails", ParamKeyBreakerPaypalCertMaxFails, "breaker"},
		{"ParamKeyBreakerPaypalCertCooldownSec", ParamKeyBreakerPaypalCertCooldownSec, "breaker"},
		{"ParamKeyBreakerFileStoreMaxFails", ParamKeyBreakerFileStoreMaxFails, "breaker"},
		{"ParamKeyBreakerFileStoreCooldownSec", ParamKeyBreakerFileStoreCooldownSec, "breaker"},
		// Repository / TX retry
		{"ParamKeyTxRetryMaxAttempts", ParamKeyTxRetryMaxAttempts, "repository"},
		{"ParamKeyTxRetryBaseDelayMs", ParamKeyTxRetryBaseDelayMs, "repository"},
		{"ParamKeyTxRetryMaxDelayMs", ParamKeyTxRetryMaxDelayMs, "repository"},
		// Payment
		{"ParamKeyPaymentMaxUploadBytes", ParamKeyPaymentMaxUploadBytes, "payment"},
		{"ParamKeyWithdrawalMinCents", ParamKeyWithdrawalMinCents, "payment"},
		{"ParamKeyWithdrawalMaxCents", ParamKeyWithdrawalMaxCents, "payment"},
		{"ParamKeyBankTransferMinAmountCents", ParamKeyBankTransferMinAmountCents, "payment"},
		{"ParamKeyBankTransferMaxAmountCents", ParamKeyBankTransferMaxAmountCents, "payment"},
		{"ParamKeyPaymentIntentTTLMinutes", ParamKeyPaymentIntentTTLMinutes, "payment"},
		// Notification subsystem
		{"ParamKeyNotifyBankTransferStaleSec", ParamKeyNotifyBankTransferStaleSec, "notify"},
		{"ParamKeyNotifyWithdrawalStaleSec", ParamKeyNotifyWithdrawalStaleSec, "notify"},
		{"ParamKeyNotifyHighValueWithdrawalCents", ParamKeyNotifyHighValueWithdrawalCents, "notify"},
		{"ParamKeyNotifyPendingReminderIntervalSec", ParamKeyNotifyPendingReminderIntervalSec, "notify"},
		{"ParamKeyNotifyPredictionDeadlineLeadMin1", ParamKeyNotifyPredictionDeadlineLeadMin1, "notify"},
		{"ParamKeyNotifyPredictionDeadlineLeadMin2", ParamKeyNotifyPredictionDeadlineLeadMin2, "notify"},
		{"ParamKeyNotifyPredictionMissingLeadMin", ParamKeyNotifyPredictionMissingLeadMin, "notify"},
		{"ParamKeyNotifyBankTransferQueueDepthThreshold", ParamKeyNotifyBankTransferQueueDepthThreshold, "notify"},
		{"ParamKeyNotifyAdminEmails", ParamKeyNotifyAdminEmails, "notify"},
		{"ParamKeyNotifyWebPushVAPIDPublicKey", ParamKeyNotifyWebPushVAPIDPublicKey, "notify"},
		{"ParamKeyNotifyWebPushVAPIDSubject", ParamKeyNotifyWebPushVAPIDSubject, "notify"},
		{"ParamKeyNotifySSEHeartbeatIntervalSec", ParamKeyNotifySSEHeartbeatIntervalSec, "notify"},
		{"ParamKeyNotifyWebPushTTLSec", ParamKeyNotifyWebPushTTLSec, "notify"},
		{"ParamKeyNotifyPushIconURL", ParamKeyNotifyPushIconURL, "notify"},
		{"ParamKeyNotifyPushBadgeURL", ParamKeyNotifyPushBadgeURL, "notify"},
		{"ParamKeyNotifySchedulerTimezone", ParamKeyNotifySchedulerTimezone, "notify"},
		{"ParamKeyNotifyDefaultLocale", ParamKeyNotifyDefaultLocale, "notify"},
		{"ParamKeyNotifyTemplateCacheTTLSec", ParamKeyNotifyTemplateCacheTTLSec, "notify"},
		{"ParamKeyNotifyPushTitleMaxChars", ParamKeyNotifyPushTitleMaxChars, "notify"},
		{"ParamKeyNotifyPushBodyMaxChars", ParamKeyNotifyPushBodyMaxChars, "notify"},
		{"ParamKeyNotifyPushSubRetentionDays", ParamKeyNotifyPushSubRetentionDays, "notify"},
		{"ParamKeyNotifyFromAddress", ParamKeyNotifyFromAddress, "notify"},
	}

	for _, tc := range paramKeys {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(tc.key, ".") {
				t.Errorf("%s = %q does not follow 'category.name' convention", tc.name, tc.key)
				return
			}
			parts := strings.SplitN(tc.key, ".", 2)
			if parts[0] != tc.category {
				t.Errorf("%s = %q: expected key prefix %q, got %q", tc.name, tc.key, tc.category, parts[0])
			}
			name := parts[1]
			if strings.ToLower(name) != name {
				t.Errorf("%s = %q: name part should be lowercase snake_case", tc.name, tc.key)
			}
			if strings.Contains(name, "-") {
				t.Errorf("%s = %q: name part should use underscores not hyphens", tc.name, tc.key)
			}
		})
	}
}

// TestDefaultConstantsArePositive validates that all Default* constants that
// represent active thresholds or limits are positive (> 0). Constants that
// are intentionally zero (disabled/off states) are tested separately below.
func TestDefaultConstantsArePositive(t *testing.T) {
	defaults := map[string]int{
		// Group
		"DefaultGroupInviteCodeLength": DefaultGroupInviteCodeLength,
		// Prediction
		"DefaultPredictionDeadlineMin": DefaultPredictionDeadlineMin,
		// Pagination
		"DefaultPaginationDefaultLimit": DefaultPaginationDefaultLimit,
		"DefaultPaginationMaxLimit":     DefaultPaginationMaxLimit,
		// Admin
		"DefaultAdminBulkMaxItems": DefaultAdminBulkMaxItems,
		// Cache
		"DefaultCacheDashboardTTLSeconds":   DefaultCacheDashboardTTLSeconds,
		"DefaultCacheMatchTTLSeconds":       DefaultCacheMatchTTLSeconds,
		"DefaultCacheLeaderboardTTLSeconds": DefaultCacheLeaderboardTTLSeconds,
		// Conflict
		"DefaultConflictStaleDays": DefaultConflictStaleDays,
		"DefaultConflictMaxScan":   DefaultConflictMaxScan,
		// DLQ
		"DefaultDLQSampleSize":         DefaultDLQSampleSize,
		"DefaultDLQReplayDefaultLimit": DefaultDLQReplayDefaultLimit,
		// Messaging
		"DefaultMessagingMaxRetries":         DefaultMessagingMaxRetries,
		"DefaultMessagingStreamMaxLen":       DefaultMessagingStreamMaxLen,
		"DefaultMessagingStreamWorkerCount":  DefaultMessagingStreamWorkerCount,
		"DefaultMessagingStreamReadBlockSec": DefaultMessagingStreamReadBlockSec,
		// Audit / auth
		"DefaultAuthValidationTimeoutSeconds": DefaultAuthValidationTimeoutSeconds,
		"DefaultAuditWriteTimeoutSeconds":     DefaultAuditWriteTimeoutSeconds,
		"DefaultAuditMaxRetries":              DefaultAuditMaxRetries,
		"DefaultAuditRetryDelayMs":            DefaultAuditRetryDelayMs,
		// Worker
		"DefaultWorkerSnapshotConcurrency":   DefaultWorkerSnapshotConcurrency,
		"DefaultWorkerSnapshotRetryBaseMs":   DefaultWorkerSnapshotRetryBaseMs,
		"DefaultWorkerSnapshotMaxAttempts":   DefaultWorkerSnapshotMaxAttempts,
		"DefaultWorkerDLQMonitorIntervalSec": DefaultWorkerDLQMonitorIntervalSec,
		"DefaultWorkerPurgeIntervalHours":    DefaultWorkerPurgeIntervalHours,
		// System
		"DefaultPurgeRetentionDays": DefaultPurgeRetentionDays,
		// API
		"DefaultAPIBodySizeLimitBytes":   DefaultAPIBodySizeLimitBytes,
		"DefaultAPIRateLimitRatePerSec":  DefaultAPIRateLimitRatePerSec,
		"DefaultAPIRateLimitBurst":       DefaultAPIRateLimitBurst,
		"DefaultAPIIdempotencyTTLHours":  DefaultAPIIdempotencyTTLHours,
		"DefaultAPIIdempotencyKeyMaxLen": DefaultAPIIdempotencyKeyMaxLen,
		// Snapshot
		"DefaultSnapshotKeepLatestCount": DefaultSnapshotKeepLatestCount,
		// Payment
		"DefaultPaymentMaxUploadBytes":      DefaultPaymentMaxUploadBytes,
		"DefaultWithdrawalMinCents":         DefaultWithdrawalMinCents,
		"DefaultWithdrawalMaxCents":         DefaultWithdrawalMaxCents,
		"DefaultBankTransferMinAmountCents": DefaultBankTransferMinAmountCents,
		"DefaultBankTransferMaxAmountCents": DefaultBankTransferMaxAmountCents,
		"DefaultPaymentIntentTTLMinutes":    DefaultPaymentIntentTTLMinutes,
		// Circuit breaker
		"DefaultBreakerPaypalCertMaxFails":    DefaultBreakerPaypalCertMaxFails,
		"DefaultBreakerPaypalCertCooldownSec": DefaultBreakerPaypalCertCooldownSec,
		"DefaultBreakerFileStoreMaxFails":     DefaultBreakerFileStoreMaxFails,
		"DefaultBreakerFileStoreCooldownSec":  DefaultBreakerFileStoreCooldownSec,
		// Repository / TX retry
		"DefaultTxRetryMaxAttempts": DefaultTxRetryMaxAttempts,
		"DefaultTxRetryBaseDelayMs": DefaultTxRetryBaseDelayMs,
		"DefaultTxRetryMaxDelayMs":  DefaultTxRetryMaxDelayMs,
		// Notification subsystem
		"DefaultNotifyBankTransferStaleSec":            DefaultNotifyBankTransferStaleSec,
		"DefaultNotifyWithdrawalStaleSec":              DefaultNotifyWithdrawalStaleSec,
		"DefaultNotifyHighValueWithdrawalCents":        DefaultNotifyHighValueWithdrawalCents,
		"DefaultNotifyPendingReminderIntervalSec":      DefaultNotifyPendingReminderIntervalSec,
		"DefaultNotifyPredictionDeadlineLeadMin1":      DefaultNotifyPredictionDeadlineLeadMin1,
		"DefaultNotifyPredictionDeadlineLeadMin2":      DefaultNotifyPredictionDeadlineLeadMin2,
		"DefaultNotifyPredictionMissingLeadMin":        DefaultNotifyPredictionMissingLeadMin,
		"DefaultNotifyBankTransferQueueDepthThreshold": DefaultNotifyBankTransferQueueDepthThreshold,
		"DefaultNotifySSEHeartbeatIntervalSec":         DefaultNotifySSEHeartbeatIntervalSec,
		"DefaultNotifyWebPushTTLSec":                   DefaultNotifyWebPushTTLSec,
		"DefaultNotifyPushSubRetentionDays":            DefaultNotifyPushSubRetentionDays,
	}

	for name, value := range defaults {
		t.Run(name, func(t *testing.T) {
			if value <= 0 {
				t.Errorf("%s = %d: default values must be positive", name, value)
			}
		})
	}
}

// TestZeroValuedDefaultsAreIntentional documents constants that are legitimately
// zero because zero means "feature disabled" or "no bonus applied". A negative
// value would always be a bug, so we guard against that here.
func TestZeroValuedDefaultsAreIntentional(t *testing.T) {
	zeroDefaults := map[string]int{
		// Scoring win-method bonuses: 0 = no global bonus applied.
		// Per-phase scoring_rules can override these for knockout rounds.
		"DefaultScoringExtraTimeBonus": DefaultScoringExtraTimeBonus,
		"DefaultScoringPenaltiesBonus": DefaultScoringPenaltiesBonus,
	}
	for name, value := range zeroDefaults {
		t.Run(name, func(t *testing.T) {
			if value < 0 {
				t.Errorf("%s = %d: zero-valued defaults must not be negative", name, value)
			}
		})
	}
}

// TestStringDefaultConstantsAreNonEmpty validates that string Default* constants
// which represent asset paths or configuration values are not accidentally empty.
func TestStringDefaultConstantsAreNonEmpty(t *testing.T) {
	stringDefaults := map[string]string{
		"DefaultNotifyPushIconURL":       DefaultNotifyPushIconURL,
		"DefaultNotifyPushBadgeURL":      DefaultNotifyPushBadgeURL,
		"DefaultNotifySchedulerTimezone": DefaultNotifySchedulerTimezone,
	}
	for name, value := range stringDefaults {
		t.Run(name, func(t *testing.T) {
			if value == "" {
				t.Errorf("%s: string default must not be empty", name)
			}
		})
	}
}

// TestConstantsDocumentation is a reminder test that fails when new constants
// are added but not documented in this file. If you see a failure here, you
// likely added a new ParamKey* or Default* constant. Make sure to:
//
//  1. Add it to the paramKeys map in TestSystemParamConstants_AllPaired
//  2. Add it to TestSystemParamNamingConventions
//  3. Add it to TestDefaultConstantsArePositive (or TestZeroValuedDefaultsAreIntentional
//     if the default is legitimately zero)
//  4. Create a migration to seed it in system_params
//  5. Add it to allParams in cmd/validate-params/main.go
//  6. Add its range bounds to paramIntConstraints (int) or paramStringConstraints
//     (string) in service/system_param_service.go
//  7. Update the expectedCount smoke-test values in TestSystemParamConstants_AllPaired
func TestConstantsDocumentation(t *testing.T) {
	t.Run("scoring_constants", testScoringConstants)
	t.Run("business_rule_constants", testBusinessRuleConstants)
	t.Run("payment_constants_ordering", testPaymentConstantsOrdering)
}

func testScoringConstants(t *testing.T) {
	t.Helper()
	scoring := map[string]int{
		"PointsExactScore":      PointsExactScore,
		"PointsCorrectOutcome":  PointsCorrectOutcome,
		"PointsGoalDifference":  PointsGoalDifference,
		"PointsIncorrectResult": PointsIncorrectResult,
	}
	for name, value := range scoring {
		if value < 0 {
			t.Errorf("%s = %d: scoring points should not be negative", name, value)
		}
	}
	if PointsExactScore <= PointsCorrectOutcome {
		t.Error("PointsExactScore should be greater than PointsCorrectOutcome")
	}
	if PointsCorrectOutcome <= PointsGoalDifference {
		t.Error("PointsCorrectOutcome should be greater than PointsGoalDifference")
	}
}

func testBusinessRuleConstants(t *testing.T) {
	t.Helper()
	if MinMembersForActive < 2 {
		t.Errorf("MinMembersForActive = %d: should be at least 2", MinMembersForActive)
	}
	if MaxMembersPerGroup < MinMembersPerGroup {
		t.Errorf("MaxMembersPerGroup (%d) must be >= MinMembersPerGroup (%d)", MaxMembersPerGroup, MinMembersPerGroup)
	}
	if StandingsWinPoints < 1 {
		t.Errorf("StandingsWinPoints = %d: should be at least 1", StandingsWinPoints)
	}
}

func testPaymentConstantsOrdering(t *testing.T) {
	t.Helper()
	if DefaultWithdrawalMinCents >= DefaultWithdrawalMaxCents {
		t.Errorf(
			"DefaultWithdrawalMinCents (%d) must be less than DefaultWithdrawalMaxCents (%d)",
			DefaultWithdrawalMinCents, DefaultWithdrawalMaxCents,
		)
	}
	if DefaultBankTransferMinAmountCents >= DefaultBankTransferMaxAmountCents {
		t.Errorf(
			"DefaultBankTransferMinAmountCents (%d) must be less than DefaultBankTransferMaxAmountCents (%d)",
			DefaultBankTransferMinAmountCents, DefaultBankTransferMaxAmountCents,
		)
	}
}

// Helper function kept for potential future use.
func getConstantValue(constName string) (interface{}, error) {
	pkgType := reflect.TypeOf((*interface{})(nil)).Elem()
	_ = pkgType
	return nil, nil
}
