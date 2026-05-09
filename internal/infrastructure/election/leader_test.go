package election_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/election"
)

const testKey = "test:leader"
const testTTL = 5 * time.Second

func newClient(t *testing.T, mr *miniredis.Miniredis) *redis.Client {
	t.Helper()
	rc := redis.NewClient(&redis.Options{
		Addr:         mr.Addr(),
		DialTimeout:  50 * time.Millisecond,
		ReadTimeout:  50 * time.Millisecond,
		WriteTimeout: 50 * time.Millisecond,
		MaxRetries:   0,
	})
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

func TestTryAcquire_FirstCaller_ReturnsTrue(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := newClient(t, mr)

	e := election.NewRedisLeaderElection(rc, testKey, testTTL, zap.NewNop())
	if !e.TryAcquire(context.Background()) {
		t.Fatal("expected first TryAcquire to return true (lock was free)")
	}
}

func TestTryAcquire_SecondCaller_ReturnsFalse(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := newClient(t, mr)

	e := election.NewRedisLeaderElection(rc, testKey, testTTL, zap.NewNop())

	if !e.TryAcquire(context.Background()) {
		t.Fatal("first acquire should succeed")
	}
	if e.TryAcquire(context.Background()) {
		t.Fatal("expected second TryAcquire to return false (lock already held)")
	}
}

func TestTryAcquire_LockExpires_SubsequentAcquireSucceeds(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := newClient(t, mr)

	ttl := 100 * time.Millisecond
	e := election.NewRedisLeaderElection(rc, testKey, ttl, zap.NewNop())

	if !e.TryAcquire(context.Background()) {
		t.Fatal("first acquire should succeed")
	}
	// Advance miniredis clock past the TTL so the key expires.
	mr.FastForward(ttl + 10*time.Millisecond)

	if !e.TryAcquire(context.Background()) {
		t.Fatal("expected TryAcquire to succeed after TTL expiry")
	}
}

func TestTryAcquire_RedisUnavailable_ReturnsFalse(t *testing.T) {
	mr := miniredis.RunT(t)
	rc := newClient(t, mr)

	e := election.NewRedisLeaderElection(rc, testKey, testTTL, zap.NewNop())

	// Stop the server to simulate a Redis outage.
	mr.Close()

	if e.TryAcquire(context.Background()) {
		t.Fatal("expected TryAcquire to return false when Redis is unreachable")
	}
}

func TestTryAcquire_TwoInstances_OnlyOneWins(t *testing.T) {
	mr := miniredis.RunT(t)
	rc1 := newClient(t, mr)
	rc2 := newClient(t, mr)

	e1 := election.NewRedisLeaderElection(rc1, testKey, testTTL, zap.NewNop())
	e2 := election.NewRedisLeaderElection(rc2, testKey, testTTL, zap.NewNop())

	won1 := e1.TryAcquire(context.Background())
	won2 := e2.TryAcquire(context.Background())

	if won1 == won2 {
		t.Fatalf("expected exactly one instance to win the lock, got won1=%v won2=%v", won1, won2)
	}
}
