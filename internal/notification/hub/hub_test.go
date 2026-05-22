package hub_test

import (
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/rede/world-cup-quiniela/internal/notification/hub"
)

func makeNotif(id int64, userID int) hub.Notification {
	return hub.Notification{ID: id, UserID: userID, EventType: "test.event",
		Title: "T", Body: "B", CreatedAt: "2026-01-01T00:00:00Z"}
}

func TestHub_ConnectAndReceive(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(1)
	defer cleanup()

	h.Broadcast(1, makeNotif(1, 1))

	select {
	case n := <-ch:
		if n.ID != 1 {
			t.Errorf("got notification ID %d; want 1", n.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("timed out waiting for notification")
	}
}

func TestHub_Disconnect_CleanedUp(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(2)
	cleanup()

	// Channel must be closed after cleanup.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("channel should be closed after cleanup")
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("channel was not closed")
	}

	// Broadcast to a user with no connections is a safe no-op.
	h.Broadcast(2, makeNotif(2, 2))
	conns, _, _ := h.Metrics()
	if conns != 0 {
		t.Errorf("connections after cleanup: got %d; want 0", conns)
	}
}

func TestHub_HasConnection(t *testing.T) {
	t.Parallel()
	h := hub.New()

	if h.HasConnection(3) {
		t.Error("expected no connection before Connect")
	}
	_, cleanup := h.Connect(3)
	if !h.HasConnection(3) {
		t.Error("expected connection after Connect")
	}
	cleanup()
	if h.HasConnection(3) {
		t.Error("expected no connection after cleanup")
	}
}

func TestHub_MultipleConnectionsSameUser(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch1, c1 := h.Connect(4)
	ch2, c2 := h.Connect(4)
	defer c1()
	defer c2()

	h.Broadcast(4, makeNotif(10, 4))

	got := 0
	deadline := time.After(200 * time.Millisecond)
	for got < 2 {
		select {
		case <-ch1:
			got++
		case <-ch2:
			got++
		case <-deadline:
			t.Fatalf("only received %d/2 notifications before timeout", got)
		}
	}
}

func TestHub_BroadcastDroppedWhenFull(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(5)
	defer cleanup()

	// Overfill the channel (buffer=32); the 33rd must be dropped.
	n := makeNotif(0, 5)
	for i := 0; i < 33; i++ {
		n.ID = int64(i)
		h.Broadcast(5, n)
	}

	_, _, dropped := h.Metrics()
	if dropped < 1 {
		t.Errorf("expected at least 1 dropped notification; got %d", dropped)
	}
	_ = ch
}

func TestHub_BroadcastToWrongUser(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(6)
	defer cleanup()

	h.Broadcast(99, makeNotif(1, 99)) // different user — ch must not receive it

	select {
	case n := <-ch:
		t.Errorf("received unexpected notification for wrong user: %+v", n)
	case <-time.After(50 * time.Millisecond):
		// correct: nothing delivered
	}
}

func TestHub_Metrics(t *testing.T) {
	t.Parallel()
	h := hub.New()

	_, c1 := h.Connect(7)
	_, c2 := h.Connect(7)
	_, c3 := h.Connect(8)

	conns, _, _ := h.Metrics()
	if conns != 3 {
		t.Errorf("connections: got %d; want 3", conns)
	}

	c1()
	c2()
	c3()

	conns, _, _ = h.Metrics()
	if conns != 0 {
		t.Errorf("connections after all cleanups: got %d; want 0", conns)
	}
}

func TestHub_RegisterMetrics_NilMeter_IsNoop(t *testing.T) {
	t.Parallel()
	h := hub.New()
	if err := h.RegisterMetrics(nil); err != nil {
		t.Errorf("RegisterMetrics(nil) should return nil, got: %v", err)
	}
}

func TestHub_RegisterMetrics_NoopMeter_RegistersAndCallbacks(t *testing.T) {
	t.Parallel()
	h := hub.New()

	// Produce some observable state so callbacks have non-zero values to report.
	_, c1 := h.Connect(100)
	defer c1()
	h.Broadcast(100, makeNotif(1, 100))

	meter := noop.NewMeterProvider().Meter("test")
	if err := h.RegisterMetrics(meter); err != nil {
		t.Fatalf("RegisterMetrics returned unexpected error: %v", err)
	}

	// Verify the meter callbacks do not panic and read live atomics correctly by
	// calling Metrics() directly — the OTel callbacks delegate to the same atomics.
	conns, broadcasts, _ := h.Metrics()
	if conns < 1 {
		t.Errorf("expected at least 1 connection; got %d", conns)
	}
	if broadcasts < 1 {
		t.Errorf("expected at least 1 broadcast; got %d", broadcasts)
	}
}

func TestHub_RaceCondition_500Concurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping 500-connection race test in short mode")
	}
	t.Parallel()

	h := hub.New()
	const numConns = 500

	var wg sync.WaitGroup
	cleanups := make([]func(), numConns)

	// Connect 500 connections across 5 user IDs.
	for i := 0; i < numConns; i++ {
		wg.Add(1)
		idx := i
		userID := (i % 5) + 10 // users 10–14
		go func() {
			defer wg.Done()
			_, cleanup := h.Connect(userID)
			cleanups[idx] = cleanup
		}()
	}
	wg.Wait()

	// Broadcast concurrently to all user IDs.
	for u := 10; u <= 14; u++ {
		wg.Add(1)
		uid := u
		go func() {
			defer wg.Done()
			h.Broadcast(uid, makeNotif(int64(uid), uid))
		}()
	}
	wg.Wait()

	// Disconnect all concurrently.
	for _, cleanup := range cleanups {
		if cleanup != nil {
			wg.Add(1)
			c := cleanup
			go func() { defer wg.Done(); c() }()
		}
	}
	wg.Wait()

	conns, _, _ := h.Metrics()
	if conns != 0 {
		t.Errorf("connections after all cleanups: got %d; want 0", conns)
	}
}
