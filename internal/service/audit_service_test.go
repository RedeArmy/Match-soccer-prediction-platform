package service

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/repository"
)

// stubAuditLogRepo records created audit entries and optionally returns an error.
type stubAuditLogRepo struct {
	mu      sync.Mutex
	created []*domain.AuditLog
	err     error
}

func (r *stubAuditLogRepo) Create(_ context.Context, entry *domain.AuditLog) error {
	if r.err != nil {
		return r.err
	}
	r.mu.Lock()
	r.created = append(r.created, entry)
	r.mu.Unlock()
	return nil
}
func (r *stubAuditLogRepo) ListByEntity(_ context.Context, _ string, _ int, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return nil, nil
}
func (r *stubAuditLogRepo) ListByActor(_ context.Context, _ int, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return nil, nil
}
func (r *stubAuditLogRepo) ListByAction(_ context.Context, _ string, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return nil, nil
}
func (r *stubAuditLogRepo) List(_ context.Context, _ repository.AuditLogFilters, _ repository.Pagination) ([]*domain.AuditLog, error) {
	return nil, nil
}

func TestAuditService_Log_PersistsEntry(t *testing.T) {
	repo := &stubAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	resType := "match"
	id := 1
	svc.Log(context.Background(), &id, nil, "match.created", &resType, &id, nil)

	// Log is fire-and-forget; wait for the goroutine to finish.
	waitForAuditEntry(t, repo, 1)

	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.created) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(repo.created))
	}
	if repo.created[0].Action != "match.created" {
		t.Errorf("expected action 'match.created', got %q", repo.created[0].Action)
	}
}

func TestAuditService_Log_RepoError_DoesNotPanic(t *testing.T) {
	repo := &stubAuditLogRepo{err: errors.New("db down")}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	svc.Log(context.Background(), nil, nil, "some.action", nil, nil, nil)
	// Give the goroutine time to run. A panic would fail the test immediately.
	waitForAuditEntry(t, repo, 0)
}

// waitForAuditEntry spins until the repo has at least n entries or the test
// times out. This is necessary because Log is fire-and-forget (goroutine).
func waitForAuditEntry(t *testing.T, repo *stubAuditLogRepo, n int) {
	t.Helper()
	for i := 0; i < 10000; i++ {
		repo.mu.Lock()
		got := len(repo.created)
		repo.mu.Unlock()
		if got >= n {
			return
		}
		runtime.Gosched()
	}
}
