package api

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/cache"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/internal/service"
	"github.com/rede/world-cup-quiniela/pkg/breaker"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// resilientCacheParamSvc is a test-only stub of service.SystemParamService
// that returns the default value for every parameter.
type resilientCacheParamSvc struct{}

func (resilientCacheParamSvc) Get(_ context.Context, _ string) (*domain.SystemParam, error) {
	return nil, nil
}
func (resilientCacheParamSvc) GetAll(_ context.Context) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (resilientCacheParamSvc) GetByCategory(_ context.Context, _ string) ([]*domain.SystemParam, error) {
	return nil, nil
}
func (resilientCacheParamSvc) Set(_ context.Context, _, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (resilientCacheParamSvc) GetString(_ context.Context, _, def string) string { return def }
func (resilientCacheParamSvc) GetInt(_ context.Context, _ string, def int) int   { return def }
func (resilientCacheParamSvc) GetDuration(_ context.Context, _ string, def time.Duration) time.Duration {
	return def
}
func (resilientCacheParamSvc) GetBool(_ context.Context, _ string, def bool) bool { return def }
func (resilientCacheParamSvc) BulkSet(_ context.Context, _ map[string]string, _ int) error {
	return nil
}
func (resilientCacheParamSvc) ResetToDefault(_ context.Context, _ string, _ int) (*domain.SystemParam, error) {
	return nil, nil
}
func (resilientCacheParamSvc) GetHistory(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.SystemParamHistory, string, error) {
	return nil, "", nil
}

var _ service.SystemParamService = resilientCacheParamSvc{}

func newServerWithRedisCache(t *testing.T) *Server {
	t.Helper()
	rc := redis.NewClient(&redis.Options{Addr: "localhost:1"})
	t.Cleanup(func() { _ = rc.Close() })
	rs := cache.NewRedisStore(rc)
	log := zaptest.NewLogger(t)
	return New(nil, &config.Config{}, log, messaging.NewInMemoryBus(nil), rs, nil)
}

func TestBuildResilientCache_WithMemoryStore_ReturnsOriginal(t *testing.T) {
	ms := cache.NewMemoryStore()
	log := zaptest.NewLogger(t)
	s := New(nil, &config.Config{}, log, messaging.NewInMemoryBus(nil), ms, nil)

	result := s.buildResilientCache(context.Background(), resilientCacheParamSvc{})
	if result != cache.Store(ms) {
		t.Errorf("expected MemoryStore returned unchanged, got %T", result)
	}
}

func TestBuildResilientCache_WithRedisStore_ReturnsResilientStore(t *testing.T) {
	s := newServerWithRedisCache(t)

	result := s.buildResilientCache(context.Background(), resilientCacheParamSvc{})
	if _, ok := result.(*cache.ResilientStore); !ok {
		t.Errorf("expected *cache.ResilientStore, got %T", result)
	}
}

func TestBuildResilientCache_WithRedisStore_AutoCreatesRegistry(t *testing.T) {
	s := newServerWithRedisCache(t)

	_ = s.buildResilientCache(context.Background(), resilientCacheParamSvc{})
	if s.breakerRegistry == nil {
		t.Error("expected breakerRegistry to be initialised by buildResilientCache")
	}
}

func TestBuildResilientCache_WithExistingRegistry_PreservesRegistry(t *testing.T) {
	s := newServerWithRedisCache(t)
	existing := breaker.NewRegistry()
	s.breakerRegistry = existing

	_ = s.buildResilientCache(context.Background(), resilientCacheParamSvc{})
	if s.breakerRegistry != existing {
		t.Error("expected existing breakerRegistry to be preserved")
	}
}
