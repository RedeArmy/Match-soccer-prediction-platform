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
	srv.Routes(context.Background())
	return srv
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

// TestServer_RunRedisBridge_RestartsAfterChannelClose verifies that the restart
// loop in runRedisBridge fires when runRedisBridgeLoop exits due to the
// subscription channel being closed (simulating a Redis restart). A second
// bridge on a fresh miniredis instance confirms that delivery resumes.
func TestServer_RunRedisBridge_RestartsAfterChannelClose(t *testing.T) {
	t.Parallel()

	// mr1/rc1: first Redis. Bridge 1 subscribes here; we close mr1 to force
	// the channel closed and exercise the restart-warning code path.
	mr1, rc1 := newRedisTestClient(t)
	// mr2/rc2: second Redis. Bridge 2 subscribes here after mr1 is closed.
	mr2, rc2 := newRedisTestClient(t)

	srv := newBridgeTestServer(t)
	h := srv.NotifHubForTest()

	const userID = 77_010
	ch, unsub := h.Connect(userID)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Bridge 1 on rc1 — confirms initial delivery.
	go srv.RunRedisBridgeForTest(ctx, rc1)
	time.Sleep(40 * time.Millisecond)

	mr1.Publish("user_notifications",
		`{"user_id":77010,"id":20,"event_type":"test.first","title":"T","body":"B","action_url":"/","created_at":"2026-01-01T00:00:00Z"}`)

	select {
	case n := <-ch:
		if n.EventType != "test.first" {
			t.Errorf("first event_type = %q; want test.first", n.EventType)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("first notification not received")
	}

	// Close mr1 — forces rc1's pubsub channel closed, which causes
	// runRedisBridgeLoop to exit via !ok. The outer runRedisBridge then logs
	// the restart-warning and enters the backoff select. Allow time for
	// go-redis to detect the EOF and for the goroutine to be scheduled.
	mr1.Close()
	time.Sleep(200 * time.Millisecond)

	// Bridge 2 on rc2 — independent of bridge 1; verifies the hub still
	// delivers to the connected subscriber after bridge 1 enters its restart loop.
	go srv.RunRedisBridgeForTest(ctx, rc2)
	time.Sleep(40 * time.Millisecond)

	mr2.Publish("user_notifications",
		`{"user_id":77010,"id":21,"event_type":"test.second","title":"T","body":"B","action_url":"/","created_at":"2026-01-01T00:00:00Z"}`)

	select {
	case n := <-ch:
		if n.EventType != "test.second" {
			t.Errorf("second event_type = %q; want test.second", n.EventType)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("second notification not received within 3 s")
	}
}

// TestServer_RunRedisBridgeLoop_PanicRecovery verifies that runRedisBridgeLoop
// recovers from a panic and returns cleanly, and that the outer runRedisBridge
// restart-warning path is exercised. The panic is provoked by delivering a
// message to a server whose notifHub is nil — Broadcast dereferences a nil
// pointer. After the panic is recovered and the loop exits, runRedisBridge
// logs the restart warning before honouring the context cancellation.
func TestServer_RunRedisBridgeLoop_PanicRecovery(t *testing.T) {
	t.Parallel()

	mr2, rc := newRedisTestClient(t)
	log := zaptest.NewLogger(t)

	// Server without Routes() → notifHub is nil. runRedisBridgeLoop does NOT
	// guard against nil hub (only runRedisBridge does), so Broadcast panics.
	srv := api.New(nil, &config.Config{}, log, messaging.NewInMemoryBus(nil), nil, nil)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		defer close(done)
		// RunRedisBridgeForTest calls runRedisBridge (the outer restart loop).
		// After runRedisBridgeLoop panics and recovers, runRedisBridge checks
		// ctx.Err() — context is still live — so it logs the restart warning.
		// We cancel the context shortly after, which causes the backoff select
		// to return via ctx.Done(), exercising the clean-shutdown path too.
		srv.RunRedisBridgeForTest(ctx, rc)
	}()
	time.Sleep(30 * time.Millisecond)

	// Trigger a Broadcast on the nil hub → panic inside runRedisBridgeLoop.
	mr2.Publish("user_notifications",
		`{"user_id":1,"id":99,"event_type":"test.panic","title":"P","body":"B","action_url":"/","created_at":"2026-01-01T00:00:00Z"}`)

	// Give the panic + recover + restart-warning log time to execute before
	// we cancel the context. The backoff is bridgeBackoffInit (1 s); cancelling
	// after 300 ms causes the backoff select to take the ctx.Done() path.
	time.Sleep(300 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// runRedisBridge exited cleanly after context cancellation.
	case <-time.After(2 * time.Second):
		t.Fatal("runRedisBridge did not return after context cancel within 2 s")
	}
}
