package election

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/testutil"
)

// mockLockDB records queryBool and ping calls and returns pre-configured results.
// Index i for queryBool consults queryErrors[i] first; if non-nil, returns the
// error. Otherwise returns queryResults[i] (false if out of bounds).
type mockLockDB struct {
	queryResults []bool
	queryErrors  []error
	queryIdx     int
	pingErr      error
	pingCalled   int
	closeErr     error
	closeCalled  bool
}

func (m *mockLockDB) queryBool(_ context.Context, _ string, _ ...any) (bool, error) {
	i := m.queryIdx
	m.queryIdx++
	if i < len(m.queryErrors) && m.queryErrors[i] != nil {
		return false, m.queryErrors[i]
	}
	if i < len(m.queryResults) {
		return m.queryResults[i], nil
	}
	return false, nil
}

func (m *mockLockDB) ping(_ context.Context) error {
	m.pingCalled++
	return m.pingErr
}

func (m *mockLockDB) close(_ context.Context) error {
	m.closeCalled = true
	return m.closeErr
}

func newMockElection(db *mockLockDB) *PgLeaderElection {
	return &PgLeaderElection{
		db:     db,
		dial:   func(_ context.Context) (lockDB, error) { return db, nil },
		lockID: 1,
		log:    zap.NewNop(),
	}
}

func TestPgLeaderElection_TryAcquire_WhenLockFree_ReturnsTrue(t *testing.T) {
	db := &mockLockDB{queryResults: []bool{true}}
	e := newMockElection(db)
	if !e.TryAcquire(context.Background()) {
		t.Fatal("expected TryAcquire to return true when lock is free")
	}
}

func TestPgLeaderElection_TryAcquire_WhenLockHeld_ReturnsFalse(t *testing.T) {
	db := &mockLockDB{queryResults: []bool{false}}
	e := newMockElection(db)
	if e.TryAcquire(context.Background()) {
		t.Fatal("expected TryAcquire to return false when lock is already held")
	}
}

func TestPgLeaderElection_TryAcquire_Idempotent_PingsInsteadOfRequery(t *testing.T) {
	db := &mockLockDB{queryResults: []bool{true}}
	e := newMockElection(db)
	e.TryAcquire(context.Background())
	e.TryAcquire(context.Background())
	if db.queryIdx != 1 {
		t.Errorf("expected exactly 1 DB query after lock is held, got %d", db.queryIdx)
	}
	if db.pingCalled != 1 {
		t.Errorf("expected exactly 1 ping on second TryAcquire, got %d", db.pingCalled)
	}
}

func TestPgLeaderElection_TryAcquire_DBError_ReturnsFalse(t *testing.T) {
	db := &mockLockDB{queryErrors: []error{errors.New("db unavailable")}}
	e := newMockElection(db)
	if e.TryAcquire(context.Background()) {
		t.Fatal("expected TryAcquire to return false on DB error")
	}
}

func TestPgLeaderElection_TryAcquire_DBError_DoesNotSetHeld(t *testing.T) {
	db := &mockLockDB{queryErrors: []error{errors.New("db unavailable")}}
	e := newMockElection(db)
	e.TryAcquire(context.Background())
	if e.held {
		t.Error("expected held to remain false after DB error")
	}
}

func TestPgLeaderElection_Close_ReleasesLockAndClosesConn(t *testing.T) {
	// queryResults[0]=true for acquire; queryResults[1]=true for unlock.
	db := &mockLockDB{queryResults: []bool{true, true}}
	e := newMockElection(db)
	e.TryAcquire(context.Background())
	e.Close(context.Background())

	if db.queryIdx != 2 {
		t.Errorf("expected 2 DB queries (acquire + unlock), got %d", db.queryIdx)
	}
	if !db.closeCalled {
		t.Error("expected connection to be closed after Close")
	}
	if e.held {
		t.Error("expected held to be false after Close")
	}
}

func TestPgLeaderElection_Close_WithoutAcquire_OnlyClosesConn(t *testing.T) {
	db := &mockLockDB{}
	e := newMockElection(db)
	e.Close(context.Background())

	if db.queryIdx != 0 {
		t.Errorf("expected 0 DB queries when lock was never acquired, got %d", db.queryIdx)
	}
	if !db.closeCalled {
		t.Error("expected connection to be closed")
	}
}

func TestPgLeaderElection_Close_UnlockError_StillClosesConn(t *testing.T) {
	// queryErrors[0]=nil (acquire succeeds), queryErrors[1]=error (unlock fails).
	db := &mockLockDB{
		queryResults: []bool{true},
		queryErrors:  []error{nil, errors.New("unlock failed")},
	}
	e := newMockElection(db)
	e.TryAcquire(context.Background())
	e.Close(context.Background()) // must not panic even if unlock fails

	if !db.closeCalled {
		t.Error("expected connection closed even when pg_advisory_unlock returned an error")
	}
}

// ── Connection-drop / split-brain tests ──────────────────────────────────────

func TestPgLeaderElection_TryAcquire_ConnectionDropped_ReconnectSucceeds_ReacquiresLock(t *testing.T) {
	// Call 1: acquire (queryResults[0]=true).
	// Call 2: ping fails → reconnect via dial (same db) → re-acquire (queryResults[1]=true).
	db := &mockLockDB{queryResults: []bool{true, true}, pingErr: errors.New("connection reset")}
	e := newMockElection(db)

	if !e.TryAcquire(context.Background()) {
		t.Fatal("expected first TryAcquire to return true")
	}
	// Simulate ping failure on second call by keeping pingErr set.
	if !e.TryAcquire(context.Background()) {
		t.Fatal("expected second TryAcquire to return true after reconnect + re-acquire")
	}
	if db.pingCalled != 1 {
		t.Errorf("expected 1 ping, got %d", db.pingCalled)
	}
	if db.queryIdx != 2 {
		t.Errorf("expected 2 DB queries (acquire + re-acquire), got %d", db.queryIdx)
	}
}

func TestPgLeaderElection_TryAcquire_ConnectionDropped_LockLost_ReturnsFalse(t *testing.T) {
	// Call 1: acquire (queryResults[0]=true).
	// Call 2: ping fails → reconnect → re-acquire returns false (another leader won).
	db := &mockLockDB{queryResults: []bool{true, false}, pingErr: errors.New("connection reset")}
	e := newMockElection(db)

	e.TryAcquire(context.Background())
	if e.TryAcquire(context.Background()) {
		t.Fatal("expected false when lock cannot be re-acquired after reconnect")
	}
	if e.held {
		t.Error("expected held=false after failing to re-acquire")
	}
}

func TestPgLeaderElection_TryAcquire_ConnectionDropped_ReconnectFails_ReturnsFalse(t *testing.T) {
	// Call 1: acquire.
	// Call 2: ping fails → dial fails → returns false without querying.
	db := &mockLockDB{queryResults: []bool{true}, pingErr: errors.New("connection reset")}
	e := newMockElection(db)
	e.dial = func(_ context.Context) (lockDB, error) { return nil, errors.New("dial failed") }

	e.TryAcquire(context.Background())
	if e.TryAcquire(context.Background()) {
		t.Fatal("expected false when reconnect fails")
	}
	if e.held {
		t.Error("expected held=false after failed reconnect")
	}
	if db.queryIdx != 1 {
		t.Errorf("expected no re-acquire query when reconnect fails, got queryIdx=%d", db.queryIdx)
	}
}

func TestPgLeaderElection_SatisfiesLeaderElectionInterface(t *testing.T) {
	// Compile-time check: *PgLeaderElection must implement LeaderElection.
	var _ LeaderElection = (*PgLeaderElection)(nil)
}

func TestPgLeaderElection_Close_ConnectionCloseError_LogsAndContinues(t *testing.T) {
	db := &mockLockDB{closeErr: errors.New("connection already closed")}
	e := newMockElection(db)
	e.Close(context.Background()) // must not panic when close returns an error

	if !db.closeCalled {
		t.Error("expected close to be called")
	}
}

func TestRedisLeaderElection_SatisfiesLeaderElectionInterface(t *testing.T) {
	// Compile-time check: *RedisLeaderElection must implement LeaderElection.
	var _ LeaderElection = (*RedisLeaderElection)(nil)
}

// ── Tests exercising the real pgConnDB and NewPgLeaderElection ────────────────
// These tests require a real PostgreSQL instance. SetupPostgres spins one up
// via testcontainers, which is available in the standard test run (no build tag
// needed — the container is started and cleaned up per test via t.Cleanup).

func TestNewPgLeaderElection_ValidDSN_Connects(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	e, err := NewPgLeaderElection(context.Background(), dsn, 999, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	e.Close(context.Background())
}

func TestNewPgLeaderElection_InvalidDSN_ReturnsError(t *testing.T) {
	_, err := NewPgLeaderElection(context.Background(), "postgres://127.0.0.1:1/nodb", 999, zap.NewNop())
	if err == nil {
		t.Fatal("expected error for unreachable DSN, got nil")
	}
}

func TestPgLeaderElection_TryAcquire_RealDB_OnlyOneWins(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	e1, err := NewPgLeaderElection(context.Background(), dsn, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("e1: %v", err)
	}
	defer e1.Close(context.Background())

	e2, err := NewPgLeaderElection(context.Background(), dsn, 100, zap.NewNop())
	if err != nil {
		t.Fatalf("e2: %v", err)
	}
	defer e2.Close(context.Background())

	won1 := e1.TryAcquire(context.Background())
	won2 := e2.TryAcquire(context.Background())

	if won1 == won2 {
		t.Fatalf("expected exactly one instance to win lock 100, got won1=%v won2=%v", won1, won2)
	}
}

func TestPgLeaderElection_Close_RealDB_ReleasesLock(t *testing.T) {
	dsn := testutil.SetupPostgres(t)

	e1, err := NewPgLeaderElection(context.Background(), dsn, 101, zap.NewNop())
	if err != nil {
		t.Fatalf("e1: %v", err)
	}
	if !e1.TryAcquire(context.Background()) {
		t.Fatal("e1 should have acquired lock 101")
	}

	// Explicitly release via Close — a new instance should now acquire.
	e1.Close(context.Background())

	e2, err := NewPgLeaderElection(context.Background(), dsn, 101, zap.NewNop())
	if err != nil {
		t.Fatalf("e2: %v", err)
	}
	defer e2.Close(context.Background())

	if !e2.TryAcquire(context.Background()) {
		t.Error("e2 should have acquired lock 101 after e1 released it via Close")
	}
}
