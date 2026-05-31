package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

const (
	defaultCacheTTL        = 5 * time.Minute
	defaultRuntimeCacheTTL = 30 * time.Second // shorter TTL for is_runtime = TRUE params
)

// cacheEntry wraps a SystemParam with a TTL deadline.
type cacheEntry struct {
	param     *domain.SystemParam
	expiresAt time.Time
}

func (e *cacheEntry) valid() bool {
	return time.Now().Before(e.expiresAt)
}

// MutationHookRegisterer is an optional extension of SystemParamService that
// allows callers to register post-mutation callbacks. Each hook is called
// synchronously after a successful Set or BulkSet for the matching key, and
// after the in-process cache has been evicted. Wiring code uses a type
// assertion to check for support; the hook is silently skipped on
// implementations that do not implement this interface.
type MutationHookRegisterer interface {
	RegisterMutationHook(key string, fn func(ctx context.Context))
}

// SystemParamService provides typed, cached access to runtime-configurable
// key-value settings stored in the system_params table.
//
// All Get* helpers return a typed value and fall back to their defaultVal
// argument when the key is absent or the stored string cannot be parsed.
// This means callers never receive an error from a missing param - the domain
// constant is always the fallback, so the system degrades gracefully.
//
// Set invalidates the in-memory cache entry for the affected key immediately,
// guaranteeing that the next read within the same process sees the new value.
type SystemParamService interface {
	Get(ctx context.Context, key string) (*domain.SystemParam, error)
	GetAll(ctx context.Context) ([]*domain.SystemParam, error)
	GetByCategory(ctx context.Context, cat string) ([]*domain.SystemParam, error)
	Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error)
	// GetString returns the raw string value, falling back to defaultVal.
	GetString(ctx context.Context, key, defaultVal string) string
	// GetInt parses the value as a base-10 integer, falling back to defaultVal.
	GetInt(ctx context.Context, key string, defaultVal int) int
	// GetDuration parses the value as a time.Duration string (e.g. "5m"),
	// falling back to defaultVal.
	GetDuration(ctx context.Context, key string, defaultVal time.Duration) time.Duration
	// GetBool parses the value as a boolean, falling back to defaultVal.
	GetBool(ctx context.Context, key string, defaultVal bool) bool
	// BulkSet updates multiple parameters in a single repository call.
	// Each key-value pair is upserted atomically. actorID is recorded as
	// the editor for the audit trail.
	BulkSet(ctx context.Context, params map[string]string, actorID int) error
	// ResetToDefault restores the operational value of key to the immutable
	// default_value set by the seeding migration. The cache entry is evicted
	// and any registered mutation hooks are fired, identical to Set.
	// Returns ErrNotFound when key does not exist.
	ResetToDefault(ctx context.Context, key string, actorID int) (*domain.SystemParam, error)
	// GetHistory returns the mutation history for key in reverse-chronological
	// order with cursor-based pagination. Returns an empty slice when no history
	// repository is wired (WithParamHistory was not used at construction).
	GetHistory(ctx context.Context, key string, p repository.CursorPage) ([]*domain.SystemParamHistory, string, error)
}

// systemParamService is the concrete implementation of SystemParamService.
// All reads go through an in-memory cache. is_runtime = TRUE params use a
// shorter 30 s TTL so runtime changes propagate within one cache window per
// replica; infrastructure params (is_runtime = FALSE) use the full 5 min TTL.
// Set() immediately evicts the affected key so the next read fetches the fresh value.
type systemParamService struct {
	repo        repository.SystemParamRepository
	historyRepo repository.SystemParamHistoryRepository // nil = history disabled
	mu          sync.RWMutex
	cache       map[string]*cacheEntry
	hooks       map[string][]func(context.Context) // protected by mu
	ttl         time.Duration
	runtimeTTL  time.Duration
	audit       AuditLogger
	log         *zap.Logger
}

// SystemParamServiceOption is a functional option for NewSystemParamService.
type SystemParamServiceOption func(*systemParamService)

// WithParamHistory wires a history repository so that every successful Set or
// ResetToDefault call appends a row to system_params_history. When this option
// is not provided, history recording is silently skipped.
func WithParamHistory(repo repository.SystemParamHistoryRepository) SystemParamServiceOption {
	return func(s *systemParamService) { s.historyRepo = repo }
}

// NewSystemParamService constructs a systemParamService.
// audit records param mutations in the audit trail.
func NewSystemParamService(repo repository.SystemParamRepository, audit AuditLogger, log *zap.Logger, opts ...SystemParamServiceOption) SystemParamService {
	s := &systemParamService{
		repo:       repo,
		cache:      make(map[string]*cacheEntry),
		hooks:      make(map[string][]func(context.Context)),
		ttl:        defaultCacheTTL,
		runtimeTTL: defaultRuntimeCacheTTL,
		audit:      audit,
		log:        log,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RegisterMutationHook registers fn to be called synchronously after a
// successful Set or BulkSet that mutates key. The hook fires after the
// in-process cache entry has been evicted, so fn can safely call Get to
// read the fresh value.
func (s *systemParamService) RegisterMutationHook(key string, fn func(ctx context.Context)) {
	s.mu.Lock()
	s.hooks[key] = append(s.hooks[key], fn)
	s.mu.Unlock()
}

// callHooks invokes all hooks registered for key. Called after eviction so
// hooks always observe the new value on the next Get.
func (s *systemParamService) callHooks(ctx context.Context, key string) {
	s.mu.RLock()
	fns := s.hooks[key]
	s.mu.RUnlock()
	for _, fn := range fns {
		fn(ctx)
	}
}

func (s *systemParamService) Get(ctx context.Context, key string) (*domain.SystemParam, error) {
	if p := s.fromCache(key); p != nil {
		return p, nil
	}
	p, err := s.repo.Get(ctx, key)
	if err != nil {
		return nil, err
	}
	if p != nil {
		s.setCache(key, p)
	}
	return p, nil
}

func (s *systemParamService) GetAll(ctx context.Context) ([]*domain.SystemParam, error) {
	return s.repo.GetAll(ctx)
}

func (s *systemParamService) GetByCategory(ctx context.Context, cat string) ([]*domain.SystemParam, error) {
	return s.repo.GetByCategory(ctx, cat)
}

// Set upserts a param and evicts the cache entry so the next read fetches
// the fresh value. When the key already exists its declared type is fetched
// first and the new value is validated against it - an unparseable value
// is rejected with ErrValidation before any write reaches the repository.
func (s *systemParamService) Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error) {
	old, err := s.validateAndGetExisting(ctx, key, value)
	if err != nil {
		return nil, err
	}
	p, err := s.repo.Set(ctx, key, value, actorID)
	if err != nil {
		return nil, err
	}
	s.evict(key)
	s.callHooks(ctx, key)
	if old != nil {
		s.recordHistory(ctx, key, old.Value, value, actorID, "set")
	}
	if s.audit != nil {
		resType := "system_param"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &actorID, &role, domain.AuditActionParamUpdated, &resType, nil, map[string]any{"key": key, "value": value})
	}
	return p, nil
}

// ResetToDefault restores the operational value of key to the immutable
// default seeded by the initial migration. The cache entry is evicted and
// mutation hooks are fired so downstream components (e.g. CachedRankingService
// TTL) pick up the restored value immediately.
func (s *systemParamService) ResetToDefault(ctx context.Context, key string, actorID int) (*domain.SystemParam, error) {
	// Capture old value before mutation; reads from cache when warm (no extra DB round-trip).
	old, _ := s.Get(ctx, key)

	p, err := s.repo.ResetToDefault(ctx, key)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperrors.NotFound("system param not found: " + key)
	}
	s.evict(key)
	s.callHooks(ctx, key)
	if old != nil {
		s.recordHistory(ctx, key, old.Value, p.Value, actorID, "reset")
	}
	if s.audit != nil {
		resType := "system_param"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &actorID, &role, domain.AuditActionParamUpdated, &resType, nil, map[string]any{"key": key, "value": p.Value, "reset_to_default": true})
	}
	return p, nil
}

// validateAndGetExisting validates value against the declared type and
// constraints of key, returning the existing param so callers can capture
// old_value for history. Returns (nil, nil) when the key does not yet exist.
func (s *systemParamService) validateAndGetExisting(ctx context.Context, key, value string) (*domain.SystemParam, error) {
	existing, err := s.Get(ctx, key)
	if err != nil || existing == nil {
		return nil, err // missing key → no type constraint to enforce
	}
	if err := validateParamValue(value, existing.Type); err != nil {
		return nil, err
	}
	return existing, validateParamConstraints(key, value, existing.Type)
}

// recordHistory appends one row to system_params_history. Failures are logged
// and suppressed: history is an observation layer — it must not block or
// reverse a mutation that has already succeeded.
func (s *systemParamService) recordHistory(ctx context.Context, key, oldValue, newValue string, actorID int, action string) {
	if s.historyRepo == nil {
		return
	}
	entry := &domain.SystemParamHistory{
		Key:      key,
		OldValue: oldValue,
		NewValue: newValue,
		ActorID:  actorID,
		Action:   action,
	}
	if err := s.historyRepo.Record(ctx, entry); err != nil {
		s.log.Warn("system_params: failed to record history entry",
			zap.String("key", key),
			zap.String("action", action),
			zap.Error(err),
		)
	}
}

// validateParamValue returns ErrValidation when value cannot be parsed to typ.
func validateParamValue(value string, typ domain.SystemParamType) error {
	switch typ {
	case domain.SystemParamTypeInt:
		if _, err := strconv.Atoi(value); err != nil {
			return apperrors.Validation(fmt.Sprintf("value %q cannot be parsed as int", value))
		}
	case domain.SystemParamTypeBool:
		if _, err := strconv.ParseBool(value); err != nil {
			return apperrors.Validation(fmt.Sprintf("value %q cannot be parsed as bool (accepted: true, false, 1, 0)", value))
		}
	case domain.SystemParamTypeDuration:
		if _, err := time.ParseDuration(value); err != nil {
			return apperrors.Validation(fmt.Sprintf("value %q cannot be parsed as duration (e.g. 5m, 30s, 1h)", value))
		}
	case domain.SystemParamTypeString:
		// any string is valid
	}
	return nil
}

// paramIntRange is an inclusive [min, max] bound for an integer system param.
type paramIntRange struct{ min, max int }

// paramIntConstraints maps every known integer param key to its valid range.
// The bounds encode business rules (e.g. scoring must be non-negative, invite
// codes need at least 6 chars to resist brute-force). Keys absent from the map
// accept any parseable integer.
var paramIntConstraints = map[string]paramIntRange{
	// Scoring — runtime, re-read on every ScoreMatch call
	domain.ParamKeyScoringExactScore:      {1, 100},
	domain.ParamKeyScoringCorrectOutcome:  {0, 100},
	domain.ParamKeyScoringGoalDiff:        {0, 100},
	domain.ParamKeyScoringExtraTimeBonus:  {0, 100},     // 0 = no global bonus
	domain.ParamKeyScoringPenaltiesBonus:  {0, 100},     // 0 = no global bonus
	domain.ParamKeyScoringUpdateChunkSize: {50, 10_000}, // min 50 rows; max 10K to prevent runaway lock hold

	// Prediction
	domain.ParamKeyPredictionDeadlineMin: {0, 1440}, // 0 = no deadline; max 24 h

	// Group lifecycle
	domain.ParamKeyGroupMinMembers:       {2, 1_000},
	domain.ParamKeyGroupMaxSize:          {2, 1_000},
	domain.ParamKeyGroupInviteCodeLength: {6, 64},

	// Conflict detection
	domain.ParamKeyConflictStaleDays: {1, 365},
	domain.ParamKeyConflictMaxScan:   {100, 100_000},

	// Pagination
	domain.ParamKeyPaginationDefaultLimit: {1, 1_000},
	domain.ParamKeyPaginationMaxLimit:     {1, 10_000},

	// Tournament
	domain.ParamKeyTournamentWinPoints: {1, 10},

	// Admin bulk operations
	domain.ParamKeyAdminBulkMaxItems: {1, 10_000},

	// Cache TTLs — 0 = disable cache
	domain.ParamKeyCacheMatchTTL:            {0, 86_400},
	domain.ParamKeyCacheLeaderboardTTL:      {0, 86_400},
	domain.ParamKeyCacheDashboardTTLSeconds: {0, 86_400},

	// Infrastructure timeouts (restart required)
	domain.ParamKeyAuditWriteTimeout:     {1, 60},
	domain.ParamKeyAuthValidationTimeout: {1, 60},

	// Audit retry policy
	domain.ParamKeyAuditMaxRetries:   {1, 10},
	domain.ParamKeyAuditRetryDelayMs: {10, 10_000},

	// DLQ
	domain.ParamKeyDLQSampleSize:         {1, 100},
	domain.ParamKeyDLQReplayDefaultLimit: {1, 1_000},

	// Messaging / Redis Streams
	domain.ParamKeyMessagingMaxRetries:         {1, 20},
	domain.ParamKeyMessagingStreamMaxLen:       {10_000, 10_000_000},
	domain.ParamKeyMessagingStreamWorkerCount:  {1, 64},
	domain.ParamKeyMessagingStreamReadBlockSec: {1, 60},

	// Worker: snapshot generation
	domain.ParamKeyWorkerSnapshotConcurrency: {1, 256},
	domain.ParamKeyWorkerSnapshotRetryBaseMs: {10, 10_000},
	domain.ParamKeyWorkerSnapshotMaxAttempts: {1, 10},

	// Worker: background maintenance
	domain.ParamKeyWorkerDLQMonitorIntervalSec: {10, 86_400},
	domain.ParamKeyWorkerPurgeIntervalHours:    {1, 720}, // 1 h – 30 days

	// Worker: notification scheduler polling intervals
	domain.ParamKeyWorkerSchedPredDeadlineIntervalSec:    {60, 3_600},      // 1 min – 1 h
	domain.ParamKeyWorkerSchedMatchResultIntervalSec:     {60, 3_600},      // 1 min – 1 h
	domain.ParamKeyWorkerSchedPendingReminderIntervalSec: {60, 86_400},     // 1 min – 24 h
	domain.ParamKeyWorkerSchedStaleEscalationIntervalSec: {60, 86_400},     // 1 min – 24 h
	domain.ParamKeyWorkerSchedPushPruneIntervalSec:       {3_600, 604_800}, // 1 h – 7 days

	// System
	domain.ParamKeyPurgeRetentionDays: {1, 365},

	// API
	domain.ParamKeyAPIBodySizeLimitBytes: {1_024, 10_485_760}, // 1 KB – 10 MB
	// Rate limiter params are is_runtime=FALSE (LimiterStore built at startup); a
	// restart is required to apply changes. Bounds prevent nonsensical values from
	// being accepted via the admin API.
	domain.ParamKeyAPIRateLimitRatePerSec: {1, 1_000}, // 1 token/s – 1 000 token/s
	domain.ParamKeyAPIRateLimitBurst:      {1, 1_000}, // min 1; 1 000 burst is already very generous
	// IP rate limiting (L1 global, L2 webhook) — is_runtime=FALSE; restart required.
	domain.ParamKeyIPRateLimitGlobalRPS:    {1, 10_000}, // 1–10K tokens/sec
	domain.ParamKeyIPRateLimitGlobalBurst:  {1, 10_000}, // min 1; 10K burst is already very generous
	domain.ParamKeyIPRateLimitWebhookRPS:   {1, 1_000},  // webhook: 1–1K tokens/sec
	domain.ParamKeyIPRateLimitWebhookBurst: {1, 1_000},  // webhook burst

	// Snapshot retention
	domain.ParamKeySnapshotKeepLatestCount: {1, 1_000},

	// System param history retention (is_runtime=FALSE; worker restart required)
	domain.ParamKeySystemParamHistoryRetentionDays: {1, 365}, // 1 day – 1 year

	// Payment / balance (is_runtime = TRUE; changes take effect within cache window)
	domain.ParamKeyPaymentMaxUploadBytes:      {102_400, 52_428_800}, // 100 KB – 50 MB
	domain.ParamKeyWithdrawalMinCents:         {100, 1_000_000},      // 1 GTQ – 10 000 GTQ
	domain.ParamKeyWithdrawalMaxCents:         {1_000, 100_000_000},  // 10 GTQ – 1 000 000 GTQ
	domain.ParamKeyBankTransferMinAmountCents: {100, 1_000_000},      // 1 GTQ – 10 000 GTQ
	domain.ParamKeyBankTransferMaxAmountCents: {1_000, 100_000_000},  // 10 GTQ – 1 000 000 GTQ
	domain.ParamKeyPaymentIntentTTLMinutes:    {5, 10_080},           // 5 min – 1 week

	// Idempotency middleware (is_runtime=FALSE; restart required)
	domain.ParamKeyAPIIdempotencyTTLHours:  {1, 720},   // 1 h – 30 days
	domain.ParamKeyAPIIdempotencyKeyMaxLen: {16, 4096}, // 16 bytes min; 4 KB max

	// Circuit breaker: PayPal certificate fetcher (is_runtime=FALSE; restart required)
	domain.ParamKeyBreakerPaypalCertMaxFails:    {1, 100},   // at least 1 failure to open
	domain.ParamKeyBreakerPaypalCertCooldownSec: {1, 3_600}, // 1 s – 1 hour

	// Circuit breaker: file store (S3/GDrive/OneDrive) (is_runtime=FALSE; restart required)
	domain.ParamKeyBreakerFileStoreMaxFails:    {1, 100},   // at least 1 failure to open
	domain.ParamKeyBreakerFileStoreCooldownSec: {1, 3_600}, // 1 s – 1 hour

	// Circuit breaker: Redis cache (is_runtime=FALSE; restart required)
	domain.ParamKeyBreakerCacheMaxFails:    {1, 100},   // at least 1 failure to open
	domain.ParamKeyBreakerCacheCooldownSec: {1, 3_600}, // 1 s – 1 hour

	// DB transaction retry policy (is_runtime=FALSE; restart required)
	domain.ParamKeyTxRetryMaxAttempts: {1, 20},     // at least 1 attempt; 20 is very generous
	domain.ParamKeyTxRetryBaseDelayMs: {1, 10_000}, // 1 ms – 10 s base backoff
	domain.ParamKeyTxRetryMaxDelayMs:  {1, 60_000}, // 1 ms – 60 s max cap

	// Notification subsystem (is_runtime=TRUE; changes propagate within 30 s cache window)
	domain.ParamKeyNotifyBankTransferStaleSec:            {3_600, 172_800},       // 1 h – 48 h
	domain.ParamKeyNotifyWithdrawalStaleSec:              {3_600, 259_200},       // 1 h – 72 h
	domain.ParamKeyNotifyHighValueWithdrawalCents:        {100_000, 100_000_000}, // Q1 000 – Q1 000 000
	domain.ParamKeyNotifyPendingReminderIntervalSec:      {1_800, 86_400},        // 30 min – 24 h
	domain.ParamKeyNotifyPredictionDeadlineLeadMin1:      {5, 120},               // 5 min – 2 h
	domain.ParamKeyNotifyPredictionDeadlineLeadMin2:      {5, 60},                // 5 min – 1 h
	domain.ParamKeyNotifyPredictionMissingLeadMin:        {15, 240},              // 15 min – 4 h
	domain.ParamKeyNotifySSEHeartbeatIntervalSec:         {5, 300},               // 5 s – 5 min
	domain.ParamKeyNotifyWebPushTTLSec:                   {3_600, 2_592_000},     // 1 h – 30 days
	domain.ParamKeyNotifyBankTransferQueueDepthThreshold: {1, 500},               // 1 – 500 pending transfers
	// Template cache and push payload limits (Phase 5 · migration 000099).
	domain.ParamKeyNotifyTemplateCacheTTLSec: {30, 3_600}, // 30 s – 1 h
	domain.ParamKeyNotifyPushTitleMaxChars:   {10, 500},   // 10 – 500 chars
	domain.ParamKeyNotifyPushBodyMaxChars:    {50, 2_000}, // 50 – 2 000 chars
	// Push subscription pruning retention (migration 000102).
	domain.ParamKeyNotifyPushSubRetentionDays: {1, 365}, // 1 day – 1 year
	// Push digest gate (migration 000105).
	domain.ParamKeyNotifyPushDigestWindowSec: {30, 3_600}, // 30 s – 1 hour
	domain.ParamKeyNotifyPushDigestThreshold: {1, 100},    // at least 1 push before digest
	// Email render budget (migration 000108).
	domain.ParamKeyNotifyRenderTimeoutMs: {100, 30_000}, // 100 ms – 30 s

	// Notification DLQ replay worker (migration 000110, is_runtime=FALSE).
	domain.ParamKeyNotifyDLQReplayBatchSize:       {1, 500},    // 1 – 500 entries per poll
	domain.ParamKeyNotifyDLQReplayPollIntervalSec: {5, 3_600},  // 5 s – 1 hour
	domain.ParamKeyNotifyDLQReplayMaxAttempts:     {1, 20},     // 1 – 20 replay attempts
	domain.ParamKeyNotifyDLQReplayAlertThreshold:  {1, 10_000}, // 1 – 10 000 unresolved entries

	// Notification outbox dispatch worker (migration 000111, is_runtime=FALSE).
	domain.ParamKeyNotifyOutboxBatchSize:            {1, 1_000},  // 1 – 1 000 rows per poll
	domain.ParamKeyNotifyOutboxPollIntervalSec:      {1, 3_600},  // 1 s – 1 hour
	domain.ParamKeyNotifyOutboxLockDurationSec:      {30, 3_600}, // 30 s – 1 hour
	domain.ParamKeyNotifyOutboxMaxAttempts:          {1, 20},     // 1 – 20 dispatch attempts
	domain.ParamKeyNotifyOutboxLagAlertThresholdSec: {1, 3_600},  // 1 s – 1 hour

	// Observability alerting thresholds (migration 000112, is_runtime=TRUE).
	domain.ParamKeyNotifyOutboxLagCriticalSec: {1, 86_400}, // 1 s – 24 hours
	domain.ParamKeyNotifyDLQWarningThreshold:  {1, 10_000}, // 1 – 10 000 unresolved entries

	// Phase 7 infrastructure params (migration 000113, is_runtime=FALSE).
	domain.ParamKeyNotifySSEChanBufSize:              {8, 1_024},   // 8 – 1 024 slots per connection
	domain.ParamKeyNotifySSEMaxConnsPerUser:          {0, 100},     // 0 = unlimited; 100 is very generous
	domain.ParamKeyNotifyOutboxStaleLockThresholdSec: {60, 86_400}, // 60 s – 24 hours

	// KYC / AML per-transaction caps (migration 000121, is_runtime=TRUE).
	domain.ParamKeyKYCTier1DepositLimitCents: {100_000, 500_000_000},   // Q1 000 – Q5 000 000
	domain.ParamKeyKYCTier2DepositLimitCents: {100_000, 1_000_000_000}, // Q1 000 – Q10 000 000
	domain.ParamKeyKYCTier2PayoutLimitCents:  {100_000, 1_000_000_000}, // Q1 000 – Q10 000 000
	domain.ParamKeyKYCAMLThresholdCents:      {100_000, 2_000_000_000}, // Q1 000 – Q20 000 000 (UAF threshold)
	domain.ParamKeyKYCReviewIntervalDays:     {30, 1_825},              // 30 days – 5 years
	domain.ParamKeyKYCMaxDocUploadBytes:      {102_400, 52_428_800},    // 100 KB – 50 MB

	// KYC / AML velocity limits — 24-hour rolling totals (migration 000124, is_runtime=TRUE).
	domain.ParamKeyKYCTier1DepositVelocityCents:    {0, 50_000_000},        // Q0 (blocked) – Q500 000 /24 h
	domain.ParamKeyKYCTier2DepositVelocityCents:    {100_000, 500_000_000}, // Q1 000 – Q5 000 000 /24 h
	domain.ParamKeyKYCTier1WithdrawalVelocityCents: {0, 50_000_000},        // Q0 (blocked) – Q500 000 /24 h
	domain.ParamKeyKYCTier2WithdrawalVelocityCents: {0, 500_000_000},       // Q0 (blocked) – Q5 000 000 /24 h
	domain.ParamKeyKYCRiskDashboardCacheTTLSec:     {10, 3_600},            // 10 s – 1 h
	domain.ParamKeyKYCIPVelocityWindowMinutes:      {5, 1_440},             // 5 min – 24 h
	domain.ParamKeyKYCIPVelocityMaxSubmissions:     {0, 100},               // 0 (disabled) – 100 per window
	domain.ParamKeyKYCDocRetentionYears:            {1, 20},                // 1 year minimum – 20 years maximum
}

// paramStringValidator validates a string system-param value for a specific key.
// An empty string is always treated as "not configured — use the compiled default"
// and must be accepted by every validator.
type paramStringValidator func(value string) error

// paramStringConstraints maps string param keys to their format validators.
// Keys absent from the map accept any string. The empty string is always valid
// (it means "fall back to the binary default").
var paramStringConstraints = map[string]paramStringValidator{
	// notify.default_locale must be one of the two supported BCP-47 tags.
	domain.ParamKeyNotifyDefaultLocale: func(value string) error {
		if value == "" || value == "en" || value == "es" {
			return nil
		}
		return apperrors.Validation(fmt.Sprintf("value %q is not a supported locale (accepted: en, es, or empty)", value))
	},
	// notify.scheduler_timezone must be a valid IANA timezone name.
	domain.ParamKeyNotifySchedulerTimezone: func(value string) error {
		if value == "" {
			return nil
		}
		if _, err := time.LoadLocation(value); err != nil {
			return apperrors.Validation(fmt.Sprintf("value %q is not a valid IANA timezone (e.g. America/Guatemala)", value))
		}
		return nil
	},
	// notify.from_address must be empty or contain '@' (bare address or RFC 5322 display-name form).
	domain.ParamKeyNotifyFromAddress: func(value string) error {
		if value == "" || strings.Contains(value, "@") {
			return nil
		}
		return apperrors.Validation(fmt.Sprintf("value %q is not a valid email address (must contain '@')", value))
	},
	// notify.push_icon_url must be a server-relative path (single leading '/') or empty.
	// Protocol-relative URLs ("//host/path") are rejected; they would point off-origin.
	domain.ParamKeyNotifyPushIconURL: func(value string) error {
		if value == "" || (strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")) {
			return nil
		}
		return apperrors.Validation(fmt.Sprintf("value %q must be a server-relative path starting with a single '/'", value))
	},
	// notify.push_badge_url must be a server-relative path (single leading '/') or empty.
	domain.ParamKeyNotifyPushBadgeURL: func(value string) error {
		if value == "" || (strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "//")) {
			return nil
		}
		return apperrors.Validation(fmt.Sprintf("value %q must be a server-relative path starting with a single '/'", value))
	},
	// notify.web_push_vapid_public_key must be a base64url-encoded uncompressed P-256 point
	// (65 bytes → 87–88 base64url chars) or empty (falls back to the binary default).
	domain.ParamKeyNotifyWebPushVAPIDPublicKey: func(value string) error {
		if value == "" {
			return nil
		}
		b, err := base64.RawURLEncoding.DecodeString(value)
		if err != nil {
			return apperrors.Validation("notify.web_push_vapid_public_key must be base64url-encoded (no padding)")
		}
		if len(b) != 65 {
			return apperrors.Validation(fmt.Sprintf("notify.web_push_vapid_public_key decoded to %d bytes; expected 65 (uncompressed P-256 point)", len(b)))
		}
		return nil
	},
	// notify.web_push_vapid_subject must be a contact URI per RFC 8292 §2.1 or empty.
	domain.ParamKeyNotifyWebPushVAPIDSubject: func(value string) error {
		if value == "" || strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "mailto:") {
			return nil
		}
		return apperrors.Validation(fmt.Sprintf("value %q must start with 'https://' or 'mailto:'", value))
	},
	// notify.admin_emails must be empty or a comma-separated list of RFC 5322 addresses.
	domain.ParamKeyNotifyAdminEmails: func(value string) error {
		if value == "" {
			return nil
		}
		for _, addr := range strings.Split(value, ",") {
			addr = strings.TrimSpace(addr)
			if !strings.Contains(addr, "@") {
				return apperrors.Validation(fmt.Sprintf("address %q in notify.admin_emails must contain '@'", addr))
			}
		}
		return nil
	},
}

// validateParamConstraints enforces per-key business-rule bounds for int params
// and format constraints for string params.
// validateParamValue must be called first to ensure value is already parseable.
func validateParamConstraints(key, value string, typ domain.SystemParamType) error {
	switch typ {
	case domain.SystemParamTypeInt:
		n, _ := strconv.Atoi(value) // safe: already validated
		c, ok := paramIntConstraints[key]
		if !ok {
			return nil
		}
		if n < c.min || n > c.max {
			return apperrors.Validation(fmt.Sprintf("value %d is out of allowed range [%d, %d]", n, c.min, c.max))
		}
	case domain.SystemParamTypeString:
		if fn, ok := paramStringConstraints[key]; ok {
			return fn(value)
		}
	default: // SystemParamTypeBool, SystemParamTypeDuration: no constraints defined yet
		return nil
	}
	return nil
}

// GetString returns the param's string value, falling back to defaultVal when
// the key is absent or the repository returns an error.
func (s *systemParamService) GetString(ctx context.Context, key, defaultVal string) string {
	p, err := s.Get(ctx, key)
	if err != nil {
		s.log.Warn("system_params: read error, using default",
			zap.String("key", key), zap.Error(err))
		return defaultVal
	}
	if p == nil {
		return defaultVal
	}
	return p.Value
}

// GetInt parses the param value as a base-10 integer, falling back to
// defaultVal when the key is absent, the value is empty, or parsing fails.
func (s *systemParamService) GetInt(ctx context.Context, key string, defaultVal int) int {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		s.log.Warn("system_params: cannot parse int, using default",
			zap.String("key", key), zap.String("value", raw))
		return defaultVal
	}
	return n
}

// GetDuration parses the param value as a time.Duration string (e.g. "5m"),
// falling back to defaultVal when the key is absent or parsing fails.
func (s *systemParamService) GetDuration(ctx context.Context, key string, defaultVal time.Duration) time.Duration {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return defaultVal
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		s.log.Warn("system_params: cannot parse duration, using default",
			zap.String("key", key), zap.String("value", raw))
		return defaultVal
	}
	return d
}

// GetBool parses the param value as a boolean ("true"/"1"/"false"/"0"),
// falling back to defaultVal when the key is absent or parsing fails.
func (s *systemParamService) GetBool(ctx context.Context, key string, defaultVal bool) bool {
	raw := s.GetString(ctx, key, "")
	if raw == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(raw)
	if err != nil {
		s.log.Warn("system_params: cannot parse bool, using default",
			zap.String("key", key), zap.String("value", raw))
		return defaultVal
	}
	return b
}

func (s *systemParamService) fromCache(key string) *domain.SystemParam {
	s.mu.RLock()
	e, ok := s.cache[key]
	s.mu.RUnlock()
	if ok && e.valid() {
		return e.param
	}
	return nil
}

func (s *systemParamService) setCache(key string, p *domain.SystemParam) {
	ttl := s.ttl
	if p.IsRuntime {
		ttl = s.runtimeTTL
	}
	s.mu.Lock()
	s.cache[key] = &cacheEntry{param: p, expiresAt: time.Now().Add(ttl)}
	s.mu.Unlock()
}

func (s *systemParamService) evict(key string) {
	s.mu.Lock()
	delete(s.cache, key)
	s.mu.Unlock()
}

// BulkSet validates all values against their declared types, then updates
// all parameters atomically and evicts their cache entries. Validation runs
// before any write so a single invalid value aborts the entire batch.
// Old values are captured during the validation pass so that history rows can
// be written for each key after the batch succeeds.
func (s *systemParamService) BulkSet(ctx context.Context, params map[string]string, actorID int) error {
	oldValues := make(map[string]string, len(params))
	for key, value := range params {
		existing, err := s.validateAndGetExisting(ctx, key, value)
		if err != nil {
			return fmt.Errorf("param %q: %w", key, err)
		}
		if existing != nil {
			oldValues[key] = existing.Value
		}
	}
	if err := s.repo.BulkSet(ctx, params, actorID); err != nil {
		return err
	}
	for key := range params {
		s.evict(key)
		s.callHooks(ctx, key)
	}
	for key, newValue := range params {
		if oldValue, ok := oldValues[key]; ok {
			s.recordHistory(ctx, key, oldValue, newValue, actorID, "set")
		}
	}
	if s.audit != nil {
		keys := make([]string, 0, len(params))
		for k := range params {
			keys = append(keys, k)
		}
		resType := "system_param"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &actorID, &role, domain.AuditActionParamUpdated, &resType, nil, map[string]any{"keys": keys})
	}
	return nil
}

// GetHistory returns mutation history for key, newest-first. Returns an empty
// slice when no history repository is configured.
func (s *systemParamService) GetHistory(ctx context.Context, key string, p repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	if s.historyRepo == nil {
		return nil, "", nil
	}
	return s.historyRepo.ListByKey(ctx, key, p)
}

var _ SystemParamService = (*systemParamService)(nil)
var _ MutationHookRegisterer = (*systemParamService)(nil)
