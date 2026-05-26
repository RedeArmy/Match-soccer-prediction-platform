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
func (r *stubAuditLogRepo) ListByEntity(_ context.Context, _ string, _ int, _ repository.CursorPage) ([]*domain.AuditLog, string, error) {
	return nil, "", nil
}
func (r *stubAuditLogRepo) ListByActor(_ context.Context, _ int, _ repository.CursorPage) ([]*domain.AuditLog, string, error) {
	return nil, "", nil
}
func (r *stubAuditLogRepo) ListByAction(_ context.Context, _ string, _ repository.CursorPage) ([]*domain.AuditLog, string, error) {
	return nil, "", nil
}
func (r *stubAuditLogRepo) List(_ context.Context, _ repository.AuditLogFilters, _ repository.CursorPage) ([]*domain.AuditLog, string, error) {
	return nil, "", nil
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

// ── Drain tests ───────────────────────────────────────────────────────────────

// TestAuditService_Drain_WaitsForInFlightGoroutines validates that Drain
// blocks until all Log goroutines complete, preventing data loss during
// graceful shutdown.
func TestAuditService_Drain_WaitsForInFlightGoroutines(t *testing.T) {
	repo := &stubAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	// Launch 10 audit log writes concurrently
	const numLogs = 10
	for i := 0; i < numLogs; i++ {
		id := i + 1
		action := "test.action"
		svc.Log(context.Background(), &id, nil, action, nil, &id, nil)
	}

	// Drain should block until all goroutines complete
	start := time.Now()
	svc.Drain()
	elapsed := time.Since(start)

	// Verify all entries were persisted
	repo.mu.Lock()
	defer repo.mu.Unlock()
	if len(repo.created) != numLogs {
		t.Errorf("expected %d audit entries after Drain, got %d", numLogs, len(repo.created))
	}

	// Drain should complete quickly (< 1s) since writes are fast
	if elapsed > time.Second {
		t.Errorf("Drain took %v, expected < 1s", elapsed)
	}
}

// TestAuditService_Drain_NoInFlightGoroutines validates that Drain is a no-op
// when there are no in-flight goroutines (safe to call multiple times).
func TestAuditService_Drain_NoInFlightGoroutines(t *testing.T) {
	repo := &stubAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	// Call Drain with no in-flight goroutines - should return immediately
	start := time.Now()
	svc.Drain()
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("Drain with no goroutines took %v, expected instant", elapsed)
	}
}

// TestAuditService_Drain_MultipleCalls validates that Drain can be called
// multiple times safely (e.g., in defer blocks or error paths).
func TestAuditService_Drain_MultipleCalls(t *testing.T) {
	repo := &stubAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	id := 1
	svc.Log(context.Background(), &id, nil, "test.action", nil, &id, nil)

	// First drain waits for goroutine
	svc.Drain()

	// Second drain should be instant (no goroutines)
	start := time.Now()
	svc.Drain()
	elapsed := time.Since(start)

	if elapsed > 10*time.Millisecond {
		t.Errorf("Second Drain took %v, expected instant", elapsed)
	}
}

// TestAuditService_Drain_WithSlowWrites validates that Drain respects the
// write timeout and completes even if some writes are slow.
func TestAuditService_Drain_WithSlowWrites(t *testing.T) {
	// Use a stub that delays writes to simulate slow DB
	slowRepo := &slowAuditLogRepo{
		stubAuditLogRepo: &stubAuditLogRepo{},
		delay:            50 * time.Millisecond,
	}
	svc := NewAuditService(slowRepo, 5*time.Second, zap.NewNop())

	// Launch 5 concurrent writes
	const numLogs = 5
	for i := 0; i < numLogs; i++ {
		id := i + 1
		svc.Log(context.Background(), &id, nil, "test.action", nil, &id, nil)
	}

	// Drain should wait for all writes to complete (they run concurrently)
	start := time.Now()
	svc.Drain()
	elapsed := time.Since(start)

	// With 50ms delay and concurrent execution, should complete in ~50-100ms
	if elapsed < 50*time.Millisecond {
		t.Errorf("Drain completed too quickly (%v), writes may not have finished", elapsed)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("Drain took too long (%v), expected ~50-100ms for concurrent writes", elapsed)
	}

	// Verify all entries were persisted
	slowRepo.mu.Lock()
	defer slowRepo.mu.Unlock()
	if len(slowRepo.created) != numLogs {
		t.Errorf("expected %d audit entries, got %d", numLogs, len(slowRepo.created))
	}
}

// slowAuditLogRepo simulates a slow database by adding a delay to Create.
type slowAuditLogRepo struct {
	*stubAuditLogRepo
	delay time.Duration
}

func (r *slowAuditLogRepo) Create(ctx context.Context, entry *domain.AuditLog) error {
	time.Sleep(r.delay)
	return r.stubAuditLogRepo.Create(ctx, entry)
}

// ── Panic recovery tests ──────────────────────────────────────────────────────

// panicAuditLogRepo panics on every Create call to exercise the recover() path.
type panicAuditLogRepo struct{ stubAuditLogRepo }

func (r *panicAuditLogRepo) Create(_ context.Context, _ *domain.AuditLog) error {
	panic("simulated repository panic")
}

// TestAuditService_Log_PanicInRepo_DoesNotCrash verifies that a panicking
// repository does not crash the process and that Drain still returns cleanly.
// The deferred recover() inside the goroutine must fire before wg.Done.
func TestAuditService_Log_PanicInRepo_DoesNotCrash(t *testing.T) {
	repo := &panicAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	// If recover() is missing or placed incorrectly, this call panics the test.
	svc.Log(context.Background(), nil, nil, "test.panic", nil, nil, nil)

	// Drain must return even though the goroutine panicked; wg.Done is called
	// by the deferred recover path before re-panicking would occur.
	done := make(chan struct{})
	go func() {
		svc.Drain()
		close(done)
	}()
	select {
	case <-done:
		// pass
	case <-time.After(2 * time.Second):
		t.Fatal("Drain blocked after goroutine panic — wg.Done was not called")
	}
}

// TestAuditService_Log_PanicInRepo_DropCounterIncrements verifies that when
// both the goroutine write and the synchronous fallback fail (panicAuditLogRepo
// panics on every call), Dropped() is incremented and Drain returns cleanly.
func TestAuditService_Log_PanicInRepo_DropCounterIncrements(t *testing.T) {
	repo := &panicAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	svc.Log(context.Background(), nil, nil, "test.panic.drop", nil, nil, nil)
	svc.Drain()

	if got := svc.Dropped(); got != 1 {
		t.Errorf("expected Dropped() == 1 after panic with failing fallback, got %d", got)
	}
}

// oncePanicRepo panics on the first Create, then succeeds on subsequent calls.
// This simulates a transient connection panic that the synchronous fallback can recover.
type oncePanicRepo struct {
	stubAuditLogRepo
	calls int
	mu    sync.Mutex
}

func (r *oncePanicRepo) Create(ctx context.Context, entry *domain.AuditLog) error {
	r.mu.Lock()
	r.calls++
	first := r.calls == 1
	r.mu.Unlock()
	if first {
		panic("simulated transient panic")
	}
	return r.stubAuditLogRepo.Create(ctx, entry)
}

// TestAuditService_Log_PanicInRepo_FallbackSucceeds verifies that when the
// goroutine write panics but the synchronous fallback succeeds, Dropped() stays
// at zero and the audit entry is persisted.
func TestAuditService_Log_PanicInRepo_FallbackSucceeds(t *testing.T) {
	repo := &oncePanicRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	svc.Log(context.Background(), nil, nil, "test.panic.fallback", nil, nil, nil)
	svc.Drain()

	if got := svc.Dropped(); got != 0 {
		t.Errorf("expected Dropped() == 0 when fallback succeeded, got %d", got)
	}
	// Fallback write must have persisted the entry.
	repo.mu.Lock()
	n := len(repo.created)
	repo.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 audit entry persisted via fallback, got %d", n)
	}
}

// TestAuditService_Dropped_ZeroAtStart verifies initial Dropped() value.
func TestAuditService_Dropped_ZeroAtStart(t *testing.T) {
	svc := NewAuditService(&stubAuditLogRepo{}, 5*time.Second, zap.NewNop())
	if got := svc.Dropped(); got != 0 {
		t.Errorf("expected Dropped() == 0 at start, got %d", got)
	}
}

// ── InFlight tracking tests ───────────────────────────────────────────────────

// blockingAuditLogRepo blocks Create until the release channel is closed,
// allowing the test to observe InFlight() while a goroutine is in flight.
type blockingAuditLogRepo struct {
	stubAuditLogRepo
	release chan struct{}
}

func (r *blockingAuditLogRepo) Create(_ context.Context, _ *domain.AuditLog) error {
	<-r.release
	return nil
}

// TestAuditService_InFlight_TracksGoroutineCount verifies that InFlight()
// increments while audit goroutines are executing and returns to zero after
// Drain completes.
func TestAuditService_InFlight_TracksGoroutineCount(t *testing.T) {
	release := make(chan struct{})
	repo := &blockingAuditLogRepo{release: release}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	if got := svc.InFlight(); got != 0 {
		t.Fatalf("expected InFlight() == 0 before any Log calls, got %d", got)
	}

	const numLogs = 3
	for i := 0; i < numLogs; i++ {
		svc.Log(context.Background(), nil, nil, "test.inflight", nil, nil, nil)
	}

	// Poll until all goroutines are blocked inside Create.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.InFlight() == numLogs {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := svc.InFlight(); got != numLogs {
		t.Errorf("expected InFlight() == %d while goroutines are blocked, got %d", numLogs, got)
	}

	// Unblock all goroutines and wait for them to finish.
	close(release)
	svc.Drain()

	if got := svc.InFlight(); got != 0 {
		t.Errorf("expected InFlight() == 0 after Drain, got %d", got)
	}
}

// TestAuditService_Log_CancelledContext_WriteStillCompletes verifies that
// cancelling the caller's context before Log returns does not abort the audit
// write. This is the key invariant of the WithoutCancel detach: an operation
// that already succeeded must produce an audit entry even if the HTTP client
// disconnected immediately after.
func TestAuditService_Log_CancelledContext_WriteStillCompletes(t *testing.T) {
	repo := &stubAuditLogRepo{}
	svc := NewAuditService(repo, 5*time.Second, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling Log — simulates client disconnect

	id := 1
	svc.Log(ctx, &id, nil, "match.result_set", nil, &id, nil)
	svc.Drain()

	repo.mu.Lock()
	n := len(repo.created)
	repo.mu.Unlock()
	if n != 1 {
		t.Errorf("expected 1 audit entry despite cancelled context, got %d", n)
	}
}
