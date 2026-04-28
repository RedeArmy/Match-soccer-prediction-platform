package service

import (
	"context"
	"errors"
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

// waitForAuditEntry polls until repo has at least n entries or 2 s elapses.
// A real sleep between checks (rather than Gosched) guarantees the goroutine
// scheduler has an opportunity to run the fire-and-forget audit write.
func waitForAuditEntry(t *testing.T, repo *stubAuditLogRepo, n int) {
	t.Helper()
	if n == 0 {
		// Nothing to wait for; yield once to let any goroutine run.
		time.Sleep(time.Millisecond)
		return
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		repo.mu.Lock()
		got := len(repo.created)
		repo.mu.Unlock()
		if got >= n {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out after 2 s waiting for %d audit entries to be persisted", n)
}
