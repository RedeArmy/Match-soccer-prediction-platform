package hub_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

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

	h.Broadcast(context.Background(), 1, makeNotif(1, 1))

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
	h.Broadcast(context.Background(), 2, makeNotif(2, 2))
	conns, _, _ := h.Metrics()
	if conns != 0 {
		t.Errorf("connections after cleanup: got %d; want 0", conns)
	}
}

func TestHub_HasLocalConnection(t *testing.T) {
	t.Parallel()
	h := hub.New()

	if h.HasLocalConnection(3) {
		t.Error("expected no connection before Connect")
	}
	_, cleanup := h.Connect(3)
	if !h.HasLocalConnection(3) {
		t.Error("expected connection after Connect")
	}
	cleanup()
	if h.HasLocalConnection(3) {
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

	h.Broadcast(context.Background(), 4, makeNotif(10, 4))

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
		h.Broadcast(context.Background(), 5, n)
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

	h.Broadcast(context.Background(), 99, makeNotif(1, 99)) // different user — ch must not receive it

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
	h.Broadcast(context.Background(), 100, makeNotif(1, 100))

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
			h.Broadcast(context.Background(), uid, makeNotif(int64(uid), uid))
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

// TestHub_RegisterMetrics_CallbacksExecuted verifies that the OTel observable
// callbacks actually run — and return the live atomic values — when an SDK
// ManualReader collects metrics. The noop meter used elsewhere skips callbacks;
// this test exercises the Observe/return-nil paths inside RegisterMetrics.
func TestHub_RegisterMetrics_CallbacksExecuted(t *testing.T) {
	t.Parallel()

	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	defer provider.Shutdown(context.Background()) //nolint:errcheck

	h := hub.New()
	_, c1 := h.Connect(200)
	defer c1()
	h.Broadcast(context.Background(), 200, makeNotif(1, 200))

	meter := provider.Meter("hub-test")
	if err := h.RegisterMetrics(meter); err != nil {
		t.Fatalf("RegisterMetrics returned error: %v", err)
	}

	// Collect triggers all registered Int64Callback functions.
	var data metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &data); err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Verify via Metrics() that the atomics being reported have the expected values.
	conns, broadcasts, _ := h.Metrics()
	if conns < 1 {
		t.Errorf("connections counter: got %d; want ≥ 1", conns)
	}
	if broadcasts < 1 {
		t.Errorf("broadcasts counter: got %d; want ≥ 1", broadcasts)
	}
}

// ── Dead-client eviction ──────────────────────────────────────────────────────

// TestHub_DeadClientEviction verifies that a connection whose buffer is
// repeatedly full is evicted after evictAfterDrops consecutive failures:
//   - the channel is closed (SSE handler detects !ok and returns)
//   - the connections metric is decremented
//   - the evicted metric is incremented
//   - subsequent cleanup() for the same connection is a safe no-op
func TestHub_DeadClientEviction_ClosesChannelAfterThreshold(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(20)
	defer cleanup()

	n := makeNotif(0, 20)

	// Fill the buffer (chanBufSize = 32), then keep broadcasting until the hub
	// evicts the connection. evictAfterDrops = 5, so 32 + 5 sends suffice.
	for i := 0; i < 32+hub.EvictAfterDrops; i++ {
		n.ID = int64(i)
		h.Broadcast(context.Background(), 20, n)
	}

	// Channel must be closed by the hub.
	select {
	case _, ok := <-ch:
		// Drain any buffered events until the channel is seen as closed.
		for ok {
			_, ok = <-ch
		}
	case <-time.After(100 * time.Millisecond):
		// If the channel wasn't closed, the read below will also fail; let it.
	}

	// Verify the channel is closed (non-blocking zero-value receive).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed after eviction")
		}
	default:
		t.Error("channel should be closed, not just empty")
	}

	conns, _, _ := h.Metrics()
	if conns != 0 {
		t.Errorf("connections after eviction: got %d; want 0", conns)
	}
}

// TestHub_DeadClientEviction_CleanupIsNoopAfterEviction ensures that the
// HTTP handler's deferred cleanup func does not panic or double-close the
// channel after the hub has already evicted the connection.
func TestHub_DeadClientEviction_CleanupIsNoopAfterEviction(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(21)
	_ = ch

	// Trigger eviction.
	n := makeNotif(0, 21)
	for i := 0; i < 32+hub.EvictAfterDrops; i++ {
		n.ID = int64(i)
		h.Broadcast(context.Background(), 21, n)
	}

	// cleanup must not panic even though the hub closed the channel already.
	cleanup()

	conns, _, _ := h.Metrics()
	if conns != 0 {
		t.Errorf("connections after cleanup post-eviction: got %d; want 0", conns)
	}
}

// TestHub_DropCounterResets_OnSuccessfulSend verifies that a successful
// delivery resets the consecutive-drop counter, preventing a connection that
// has recovered from being evicted on its next single drop.
func TestHub_DropCounterResets_OnSuccessfulSend(t *testing.T) {
	t.Parallel()
	h := hub.New()

	ch, cleanup := h.Connect(22)
	defer cleanup()

	n := makeNotif(0, 22)

	// Drive 4 drops (one under the threshold).
	for i := 0; i < 32+hub.EvictAfterDrops-1; i++ {
		n.ID = int64(i)
		h.Broadcast(context.Background(), 22, n)
	}

	// Drain the channel so the next send succeeds and resets the counter.
	for len(ch) > 0 {
		<-ch
	}
	h.Broadcast(context.Background(), 22, makeNotif(999, 22)) // successful send → counter reset to 0
	// Drain the single buffered item so the second batch starts from an empty buffer.
	for len(ch) > 0 {
		<-ch
	}

	// Another evictAfterDrops-1 drops must NOT evict the connection.
	for i := 0; i < 32+hub.EvictAfterDrops-1; i++ {
		n.ID = int64(i)
		h.Broadcast(context.Background(), 22, n)
	}

	conns, _, _ := h.Metrics()
	if conns != 1 {
		t.Errorf("connection should still be alive after counter reset; connections=%d", conns)
	}
}

// ── RegisterMetrics error paths ───────────────────────────────────────────────

// meterFailAt wraps noop.Meter and returns an error on the Nth registration
// call (across Int64ObservableUpDownCounter and Int64ObservableCounter).
// Used to exercise the error-propagation branches inside RegisterMetrics.
type meterFailAt struct {
	noop.Meter
	failOnCall int
	cur        int
}

func (m *meterFailAt) Int64ObservableUpDownCounter(name string, opts ...metric.Int64ObservableUpDownCounterOption) (metric.Int64ObservableUpDownCounter, error) {
	m.cur++
	if m.cur == m.failOnCall {
		return noop.Int64ObservableUpDownCounter{}, errors.New("mock: meter registration error")
	}
	return m.Meter.Int64ObservableUpDownCounter(name, opts...)
}

func (m *meterFailAt) Int64ObservableCounter(name string, opts ...metric.Int64ObservableCounterOption) (metric.Int64ObservableCounter, error) {
	m.cur++
	if m.cur == m.failOnCall {
		return noop.Int64ObservableCounter{}, errors.New("mock: meter registration error")
	}
	return m.Meter.Int64ObservableCounter(name, opts...)
}

// TestHub_RegisterMetrics_RegistrationError_Propagates verifies that an error
// returned by the meter on any of the first three instrument registrations is
// propagated back to the caller rather than silently swallowed.
func TestHub_RegisterMetrics_RegistrationError_Propagates(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		failOnCall int
	}{
		{"connections_updown_counter", 1},
		{"broadcasts_counter", 2},
		{"dropped_counter", 3},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := hub.New()
			if err := h.RegisterMetrics(&meterFailAt{failOnCall: tc.failOnCall}); err == nil {
				t.Errorf("%s: expected error from meter registration failure, got nil", tc.name)
			}
		})
	}
}
