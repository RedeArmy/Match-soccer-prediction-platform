//go:build integration

package api_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"github.com/rede/world-cup-quiniela/internal/api"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
	"github.com/rede/world-cup-quiniela/pkg/config"
)

// newBridgeServer builds a minimal Server wired to e2eDB (set by TestMain) and
// calls Routes() so the SSE hub is initialised. Tests that need to inspect the
// hub use srv.NotifHubForTest().
func newBridgeServer(t *testing.T) *api.Server {
	t.Helper()
	cfg := &config.Config{}
	srv := api.New(e2eDB, cfg, zaptest.NewLogger(t), messaging.NewInMemoryBus(nil), nil, nil)
	srv.Routes() // initialises notifHub
	return srv
}

// sendNotify executes pg_notify on the given channel with a JSON payload.
func sendNotify(t *testing.T, channel, payload string) {
	t.Helper()
	_, err := e2eDB.Exec(context.Background(),
		"SELECT pg_notify($1, $2)", channel, payload)
	if err != nil {
		t.Fatalf("pg_notify: %v", err)
	}
}

// TestListenAndBridge_ReceivesNotification verifies the end-to-end happy path:
// listenAndBridge LISTENs on user_notifications, receives a pg_notify payload,
// parses it, and broadcasts the notification to the in-process SSE hub.
func TestListenAndBridge_ReceivesNotification(t *testing.T) {
	skipIfNoE2EDB(t)

	srv := newBridgeServer(t)
	h := srv.NotifHubForTest()

	// Subscribe before the bridge starts so no event is missed.
	const testUserID = 999_001
	ch, unsub := h.Connect(testUserID)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridgeDone := make(chan error, 1)
	go func() { bridgeDone <- srv.RunBridgeOnce(ctx) }()

	// Allow the bridge goroutine time to LISTEN before we notify.
	time.Sleep(100 * time.Millisecond)

	payload, _ := json.Marshal(map[string]any{
		"user_id":    testUserID,
		"id":         int64(1),
		"event_type": "test.bridge",
		"title":      "Bridge test",
		"body":       "integration test",
		"action_url": "/test",
		"created_at": "2026-01-01T00:00:00Z",
	})
	sendNotify(t, "user_notifications", string(payload))

	select {
	case n := <-ch:
		if n.EventType != "test.bridge" {
			t.Errorf("event_type = %q, want %q", n.EventType, "test.bridge")
		}
		if n.UserID != testUserID {
			t.Errorf("user_id = %d, want %d", n.UserID, testUserID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hub did not receive notification within 3 s")
	}

	// Cancel the context — listenAndBridge should return nil (clean shutdown).
	cancel()
	select {
	case err := <-bridgeDone:
		if err != nil {
			t.Errorf("RunBridgeOnce returned non-nil on clean shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("listenAndBridge did not return after context cancellation")
	}
}

// TestListenAndBridge_InvalidPayloadContinues verifies that a malformed JSON
// payload does not crash the bridge loop — the frame is logged and discarded,
// and subsequent valid notifications are still delivered.
func TestListenAndBridge_InvalidPayloadContinues(t *testing.T) {
	skipIfNoE2EDB(t)

	srv := newBridgeServer(t)
	h := srv.NotifHubForTest()

	const testUserID = 999_002
	ch, unsub := h.Connect(testUserID)
	defer unsub()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bridgeDone := make(chan error, 1)
	go func() { bridgeDone <- srv.RunBridgeOnce(ctx) }()

	time.Sleep(100 * time.Millisecond)

	// Send invalid JSON first — bridge must survive and continue.
	sendNotify(t, "user_notifications", `{not valid json`)

	// Send a valid payload immediately after.
	payload, _ := json.Marshal(map[string]any{
		"user_id":    testUserID,
		"id":         int64(2),
		"event_type": "test.bridge.recovery",
		"title":      "Recovery",
		"body":       "after bad frame",
		"action_url": "/recovery",
		"created_at": "2026-01-01T00:00:00Z",
	})
	sendNotify(t, "user_notifications", string(payload))

	select {
	case n := <-ch:
		if n.EventType != "test.bridge.recovery" {
			t.Errorf("event_type = %q, want %q", n.EventType, "test.bridge.recovery")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("hub did not receive recovery notification within 3 s")
	}

	cancel()
	<-bridgeDone
}
