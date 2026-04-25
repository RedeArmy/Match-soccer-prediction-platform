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

const defaultCacheTTL = 5 * time.Minute

// cacheEntry wraps a SystemParam with a TTL deadline.
type cacheEntry struct {
	param     *domain.SystemParam
	expiresAt time.Time
}

func (e *cacheEntry) valid() bool {
	return time.Now().Before(e.expiresAt)
}

// systemParamService is the concrete implementation of SystemParamService.
// All reads go through an in-memory cache (TTL = 5 min). Set() immediately
// evicts the affected key so the next read fetches the fresh value.
type systemParamService struct {
	repo repository.SystemParamRepository
	mu   sync.RWMutex
	// cache maps param key → *cacheEntry. Stale or missing entries trigger
	// a read-through to the repository.
	cache map[string]*cacheEntry
	ttl   time.Duration
	log   *zap.Logger
}

// NewSystemParamService constructs a systemParamService with a 5-minute TTL.
func NewSystemParamService(repo repository.SystemParamRepository, log *zap.Logger) SystemParamService {
	return &systemParamService{
		repo:  repo,
		cache: make(map[string]*cacheEntry),
		ttl:   defaultCacheTTL,
		log:   log,
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
// first and the new value is validated against it — an unparseable value
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
	return p, nil
}

// validateValueForKey fetches the declared type of key and checks that value
// can be parsed to that type. It is a no-op when the key does not yet exist
// (new params have no type constraint at write time).
func (s *systemParamService) validateValueForKey(ctx context.Context, key, value string) error {
	existing, err := s.Get(ctx, key)
	if err != nil || existing == nil {
		return err // missing key → no type to validate against
	}
	return validateParamValue(value, existing.Type)
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
	s.mu.Lock()
	s.cache[key] = &cacheEntry{param: p, expiresAt: time.Now().Add(s.ttl)}
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
	}
	return nil
}

var _ SystemParamService = (*systemParamService)(nil)
