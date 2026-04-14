package health

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// newTestRedis starts a miniredis server and returns a redis.Client connected
// to it. The server is automatically stopped when the test ends.
func newTestRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return mr, rc
}

// newDeadRedis returns a redis.Client pointing at an unreachable address with
// no retries so that commands fail immediately without sleeping.
func newDeadRedis(t *testing.T) *redis.Client {
	t.Helper()
	rc := redis.NewClient(&redis.Options{
		Addr:       "localhost:1", // OS rejects the connection immediately
		MaxRetries: 0,
	})
	t.Cleanup(func() { _ = rc.Close() })
	return rc
}

// ── isNoGroupError ────────────────────────────────────────────────────────────

func TestIsNoGroupError_Nil_ReturnsFalse(t *testing.T) {
	if isNoGroupError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsNoGroupError_NoGroupPrefix_ReturnsTrue(t *testing.T) {
	err := errors.New("NOGROUP No such key 'stream' or consumer group 'g'")
	if !isNoGroupError(err) {
		t.Errorf("expected true for NOGROUP error, got false (err=%v)", err)
	}
}

func TestIsNoGroupError_OtherError_ReturnsFalse(t *testing.T) {
	err := errors.New("WRONGTYPE Operation against a key holding the wrong kind of value")
	if isNoGroupError(err) {
		t.Errorf("expected false for non-NOGROUP error, got true (err=%v)", err)
	}
}

func TestIsNoGroupError_ShortString_ReturnsFalse(t *testing.T) {
	err := errors.New("ERR")
	if isNoGroupError(err) {
		t.Errorf("expected false for short error string, got true (err=%v)", err)
	}
}

// ── DLQChecker ────────────────────────────────────────────────────────────────

func TestDLQChecker_Name_ContainsEventType(t *testing.T) {
	_, rc := newTestRedis(t)
	c := NewDLQChecker(rc, "match.finished")
	if c.Name() != "dlq:match.finished" {
		t.Errorf("expected name 'dlq:match.finished', got %q", c.Name())
	}
}

func TestDLQChecker_EmptyDLQ_ReturnsOK(t *testing.T) {
	_, rc := newTestRedis(t)
	c := NewDLQChecker(rc, "match.finished")

	result := c.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("expected status 'ok' for empty DLQ, got %q (error: %q)", result.Status, result.Error)
	}
}

func TestDLQChecker_NonEmptyDLQ_ReturnsError(t *testing.T) {
	mr, rc := newTestRedis(t)
	mr.Lpush("dlq:match.finished", "event1")
	mr.Lpush("dlq:match.finished", "event2")

	c := NewDLQChecker(rc, "match.finished")
	result := c.Check(context.Background())
	if result.Status != "error" {
		t.Errorf("expected status 'error' for non-empty DLQ, got %q", result.Status)
	}
	if result.Error == "" {
		t.Error("expected non-empty Error field for non-empty DLQ")
	}
}

func TestDLQChecker_RedisError_ReturnsError(t *testing.T) {
	rc := newDeadRedis(t)
	c := NewDLQChecker(rc, "match.finished")
	result := c.Check(context.Background())
	if result.Status != "error" {
		t.Errorf("expected status 'error' on Redis failure, got %q", result.Status)
	}
}

// ── StreamPendingChecker ──────────────────────────────────────────────────────

const (
	testStream = "stream:match.finished"
	testGroup  = "quiniela-workers"
)

func TestStreamPendingChecker_Name_ContainsStreamKey(t *testing.T) {
	_, rc := newTestRedis(t)
	c := NewStreamPendingChecker(rc, testStream, testGroup)
	if c.Name() != "stream_pending:"+testStream {
		t.Errorf("expected name 'stream_pending:%s', got %q", testStream, c.Name())
	}
}

func TestStreamPendingChecker_NoGroupYet_ReturnsOK(t *testing.T) {
	// The stream/group does not exist; XPENDING returns NOGROUP, which is treated
	// as healthy — the worker simply has not started yet.
	_, rc := newTestRedis(t)
	c := NewStreamPendingChecker(rc, testStream, testGroup)

	result := c.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("expected status 'ok' for missing group (NOGROUP), got %q (error: %q)",
			result.Status, result.Error)
	}
}

func TestStreamPendingChecker_GroupExistsNoPending_ReturnsOK(t *testing.T) {
	mr, rc := newTestRedis(t)

	// Create stream and group; add a message but acknowledge it immediately.
	mr.XAdd(testStream, "*", []string{"key", "val"})
	if err := rc.XGroupCreate(context.Background(), testStream, testGroup, "0").Err(); err != nil {
		t.Fatalf("XGroupCreate: %v", err)
	}
	// Read and acknowledge so pending count is 0.
	msgs, err := rc.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    testGroup,
		Consumer: "consumer-1",
		Streams:  []string{testStream, ">"},
		Count:    10,
	}).Result()
	if err != nil {
		t.Fatalf("XReadGroup: %v", err)
	}
	for _, s := range msgs {
		for _, msg := range s.Messages {
			if err := rc.XAck(context.Background(), testStream, testGroup, msg.ID).Err(); err != nil {
				t.Fatalf("XAck: %v", err)
			}
		}
	}

	c := NewStreamPendingChecker(rc, testStream, testGroup)
	result := c.Check(context.Background())
	if result.Status != "ok" {
		t.Errorf("expected status 'ok' with 0 pending, got %q (error: %q)",
			result.Status, result.Error)
	}
}

func TestStreamPendingChecker_PendingMessages_ReturnsError(t *testing.T) {
	mr, rc := newTestRedis(t)

	// Add a message to the stream and create the consumer group.
	mr.XAdd(testStream, "*", []string{"key", "val"})
	if err := rc.XGroupCreate(context.Background(), testStream, testGroup, "0").Err(); err != nil {
		t.Fatalf("XGroupCreate: %v", err)
	}
	// Read but do NOT acknowledge → message stays in pending-entry list.
	if _, err := rc.XReadGroup(context.Background(), &redis.XReadGroupArgs{
		Group:    testGroup,
		Consumer: "consumer-1",
		Streams:  []string{testStream, ">"},
		Count:    10,
	}).Result(); err != nil {
		t.Fatalf("XReadGroup: %v", err)
	}

	c := NewStreamPendingChecker(rc, testStream, testGroup)
	result := c.Check(context.Background())
	if result.Status != "error" {
		t.Errorf("expected status 'error' for pending messages, got %q", result.Status)
	}
}

func TestStreamPendingChecker_RedisError_ReturnsError(t *testing.T) {
	rc := newDeadRedis(t)
	c := NewStreamPendingChecker(rc, testStream, testGroup)
	result := c.Check(context.Background())
	if result.Status != "error" {
		t.Errorf("expected status 'error' on Redis failure, got %q", result.Status)
	}
}
