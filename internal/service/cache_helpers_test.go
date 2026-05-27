package service

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
)

func TestCacheDelete_SuccessRecordsKeys(t *testing.T) {
	store := newStubCache()
	cacheDelete(context.Background(), store, zap.NewNop(), domain.CacheKeyKYCRiskDashboard)
	if len(store.deleted) != 1 || store.deleted[0] != domain.CacheKeyKYCRiskDashboard {
		t.Errorf("expected [%q] deleted, got %v", domain.CacheKeyKYCRiskDashboard, store.deleted)
	}
}

func TestCacheDelete_ErrorIsLoggedNotPropagated(t *testing.T) {
	store := newStubCache()
	store.delErr = errors.New("redis unavailable")
	// must not panic; the error is swallowed and logged as a warning
	cacheDelete(context.Background(), store, zap.NewNop(), domain.CacheKeyKYCRiskDashboard)
}
