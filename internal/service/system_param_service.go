package service

import (
	"context"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
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
// the fresh value. actorID is forwarded to the repository for audit purposes.
func (s *systemParamService) Set(ctx context.Context, key, value string, actorID int) (*domain.SystemParam, error) {
	p, err := s.repo.Set(ctx, key, value, actorID)
	if err != nil {
		return nil, err
	}
	s.evict(key)
	return p, nil
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

// BulkSet updates multiple parameters atomically and evicts their cache entries.
func (s *systemParamService) BulkSet(ctx context.Context, params map[string]string, actorID int) error {
	if err := s.repo.BulkSet(ctx, params, actorID); err != nil {
		return err
	}
	for key := range params {
		s.evict(key)
	}
	return nil
}

var _ SystemParamService = (*systemParamService)(nil)
