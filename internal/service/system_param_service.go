package service

import (
	"context"
	"fmt"
	"strconv"
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

// systemParamService is the concrete implementation of SystemParamService.
// All reads go through an in-memory cache. is_runtime = TRUE params use a
// shorter 30 s TTL so runtime changes propagate within one cache window per
// replica; infrastructure params (is_runtime = FALSE) use the full 5 min TTL.
// Set() immediately evicts the affected key so the next read fetches the fresh value.
type systemParamService struct {
	repo       repository.SystemParamRepository
	mu         sync.RWMutex
	cache      map[string]*cacheEntry
	hooks      map[string][]func(context.Context) // protected by mu
	ttl        time.Duration
	runtimeTTL time.Duration
	audit      AuditLogger
	log        *zap.Logger
}

// NewSystemParamService constructs a systemParamService.
// audit records param mutations in the audit trail.
func NewSystemParamService(repo repository.SystemParamRepository, audit AuditLogger, log *zap.Logger) SystemParamService {
	return &systemParamService{
		repo:       repo,
		cache:      make(map[string]*cacheEntry),
		hooks:      make(map[string][]func(context.Context)),
		ttl:        defaultCacheTTL,
		runtimeTTL: defaultRuntimeCacheTTL,
		audit:      audit,
		log:        log,
	}
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
	if err := s.validateValueForKey(ctx, key, value); err != nil {
		return nil, err
	}
	p, err := s.repo.Set(ctx, key, value, actorID)
	if err != nil {
		return nil, err
	}
	s.evict(key)
	s.callHooks(ctx, key)
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
	p, err := s.repo.ResetToDefault(ctx, key)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, apperrors.NotFound("system param not found: " + key)
	}
	s.evict(key)
	s.callHooks(ctx, key)
	if s.audit != nil {
		resType := "system_param"
		role := domain.RoleAdmin
		s.audit.Log(ctx, &actorID, &role, domain.AuditActionParamUpdated, &resType, nil, map[string]any{"key": key, "value": p.Value, "reset_to_default": true})
	}
	return p, nil
}

// validateValueForKey fetches the declared type of key and checks that value
// can be parsed to that type and satisfies per-key business-rule constraints.
// It is a no-op when the key does not yet exist (new params have no constraint
// at write time).
func (s *systemParamService) validateValueForKey(ctx context.Context, key, value string) error {
	existing, err := s.Get(ctx, key)
	if err != nil || existing == nil {
		return err // missing key -> no type to validate against
	}
	if err := validateParamValue(value, existing.Type); err != nil {
		return err
	}
	return validateParamConstraints(key, value, existing.Type)
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
	domain.ParamKeyScoringExactScore:        {1, 100},
	domain.ParamKeyScoringCorrectOutcome:    {0, 100},
	domain.ParamKeyScoringGoalDiff:          {0, 100},
	domain.ParamKeyPredictionDeadlineMin:    {0, 1440}, // 0 = no deadline; max 24 h
	domain.ParamKeyGroupMinMembers:          {2, 1000},
	domain.ParamKeyGroupInviteCodeLength:    {6, 64},
	domain.ParamKeyConflictStaleDays:        {1, 365},
	domain.ParamKeyPaginationDefaultLimit:   {1, 1_000},
	domain.ParamKeyPaginationMaxLimit:       {1, 10_000},
	domain.ParamKeyTournamentWinPoints:      {1, 10},
	domain.ParamKeyAdminBulkMaxItems:        {1, 10_000},
	domain.ParamKeyCacheMatchTTL:            {0, 86_400}, // 0 = disable cache
	domain.ParamKeyCacheLeaderboardTTL:      {0, 86_400},
	domain.ParamKeyCacheDashboardTTLSeconds: {0, 86_400},
	domain.ParamKeyAuditWriteTimeout:        {1, 60},
	domain.ParamKeyDLQSampleSize:            {1, 100},
	domain.ParamKeyDLQReplayDefaultLimit:    {1, 1_000},
	domain.ParamKeyMessagingMaxRetries:      {1, 20},
	domain.ParamKeyMessagingStreamMaxLen:    {10_000, 10_000_000},
	domain.ParamKeyAuthValidationTimeout:    {1, 60},
}

// validateParamConstraints enforces per-key business-rule bounds for int params.
// validateParamValue must be called first to ensure value is already parseable.
func validateParamConstraints(key, value string, typ domain.SystemParamType) error {
	if typ != domain.SystemParamTypeInt {
		return nil
	}
	n, _ := strconv.Atoi(value) // safe: already validated
	c, ok := paramIntConstraints[key]
	if !ok {
		return nil
	}
	if n < c.min || n > c.max {
		return apperrors.Validation(fmt.Sprintf("value %d is out of allowed range [%d, %d]", n, c.min, c.max))
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
func (s *systemParamService) BulkSet(ctx context.Context, params map[string]string, actorID int) error {
	for key, value := range params {
		if err := s.validateValueForKey(ctx, key, value); err != nil {
			return fmt.Errorf("param %q: %w", key, err)
		}
	}
	if err := s.repo.BulkSet(ctx, params, actorID); err != nil {
		return err
	}
	for key := range params {
		s.evict(key)
		s.callHooks(ctx, key)
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

var _ SystemParamService = (*systemParamService)(nil)
var _ MutationHookRegisterer = (*systemParamService)(nil)
