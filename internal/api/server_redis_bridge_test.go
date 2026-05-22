package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// newRedisTestClient starts an in-process miniredis and returns a connected client.
func newRedisTestClient(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return mr, rc
}

// newBridgeTestServer builds a minimal Server with Routes() called so the SSE hub
// is initialised. Uses a fake (lazy-connect) pool — only SSE/notification functionality is exercised.
func newBridgeTestServer(t *testing.T) *api.Server {
	t.Helper()
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	srv.Routes()
	return srv
}

// ── SetInfraContext ───────────────────────────────────────────────────────────

func TestServer_SetInfraContext_DoesNotPanic(t *testing.T) {
	t.Parallel()
	srv := api.New(fakePool(t), &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	srv.SetInfraContext(context.Background()) // must not panic
}

// ── StartRedisBridge lifecycle ────────────────────────────────────────────────

func TestServer_StartRedisBridge_StopsCleanlyOnCancel(t *testing.T) {
	t.Parallel()
	_, rc := newRedisTestClient(t)
	srv := newBridgeTestServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	srv.StartRedisBridge(ctx, rc)
	cancel()
	// StopPgNotifyBridge waits for the goroutine to exit; deadlock = failure.
	srv.StopPgNotifyBridge()
}

// ── runRedisBridge via RunRedisBridgeForTest ──────────────────────────────────

func TestServer_RunRedisBridge_NilHub_ReturnsImmediately(t *testing.T) {
	t.Parallel()
	_, rc := newRedisTestClient(t)

	// Server without Routes() → notifHub is nil.
	srv := api.New(nil, &config.Config{}, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.RunRedisBridgeForTest(ctx, rc)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("runRedisBridge with nil hub did not return immediately")
	}
}

func TestServer_RunRedisBridge_BroadcastsPayload(t *testing.T) {
	t.Parallel()
	mr, rc := newRedisTestClient(t)
	srv := newBridgeTestServer(t)
	h := srv.NotifHubForTest()

	const userID = 77_001
	ch, unsub := h.Connect(userID)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ready := make(chan struct{})
	go func() {
		close(ready)
		srv.RunRedisBridgeForTest(ctx, rc)
	}()
	<-ready
	time.Sleep(30 * time.Millisecond) // let Subscribe complete

	mr.Publish("user_notifications",
		`{"user_id":77001,"id":9,"event_type":"test.redis","title":"T","body":"B","action_url":"/","created_at":"2026-01-01T00:00:00Z"}`)

	select {
	case n := <-ch:
		if n.EventType != "test.redis" {
			t.Errorf("event_type = %q; want test.redis", n.EventType)
		}
		if n.UserID != userID {
			t.Errorf("user_id = %d; want %d", n.UserID, userID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hub did not receive notification within 2 s")
	}
}

func TestServer_RunRedisBridge_InvalidJSON_ContinuesAfterBadFrame(t *testing.T) {
	t.Parallel()
	mr, rc := newRedisTestClient(t)
	srv := newBridgeTestServer(t)
	h := srv.NotifHubForTest()

	const userID = 77_002
	ch, unsub := h.Connect(userID)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go srv.RunRedisBridgeForTest(ctx, rc)
	time.Sleep(30 * time.Millisecond)

	// Publish malformed JSON first — bridge must survive.
	mr.Publish("user_notifications", `{not valid json`)

	// Publish a valid payload immediately after.
	mr.Publish("user_notifications",
		`{"user_id":77002,"id":10,"event_type":"test.recovery","title":"R","body":"B","action_url":"/","created_at":"2026-01-01T00:00:00Z"}`)

	select {
	case n := <-ch:
		if n.EventType != "test.recovery" {
			t.Errorf("event_type = %q; want test.recovery", n.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("hub did not receive recovery notification after bad frame")
	}
}
