package messaging_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
	"github.com/rede/world-cup-quiniela/internal/infrastructure/messaging"
)

// ── helpers ───────────────────────────────────────────────────────────────────

const (
	fmtHandlerNotCalled = "handler was not called within timeout"
	fmtWrongPayload     = "expected payload %v, got %v"
	fmtUnexpectedErr    = "unexpected error: %v"
	fmtWrongCallCount   = "expected handler call count %d, got %d"
)

func newMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })
	return mr, client
}

func matchFinishedEnvelope() events.Envelope {
	return events.Envelope{
		Type:       events.EventMatchFinished,
		OccurredAt: time.Now().UTC(),
		Payload:    events.MatchFinished{MatchID: 42, HomeTeam: "Brazil", AwayTeam: "Argentina", HomeScore: 2, AwayScore: 1},
	}
}

// ── InMemoryBus ───────────────────────────────────────────────────────────────

func TestInMemoryBus_Publish_DeliversToSubscriber(t *testing.T) {
	bus := messaging.NewInMemoryBus(nil)
	want := matchFinishedEnvelope()

	received := make(chan events.Envelope, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, env events.Envelope) error {
		received <- env
		return nil
	})

	if err := bus.Publish(context.Background(), want); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	select {
	case got := <-received:
		if got.Type != want.Type {
			t.Errorf(fmtWrongPayload, want.Type, got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal(fmtHandlerNotCalled)
	}
}

func TestInMemoryBus_Publish_DeliversToMultipleHandlers(t *testing.T) {
	bus := messaging.NewInMemoryBus(nil)
	var mu sync.Mutex
	count := 0

	for range 3 {
		bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
			mu.Lock()
			count++
			mu.Unlock()
			return nil
		})
	}

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	mu.Lock()
	got := count
	mu.Unlock()
	if got != 3 {
		t.Errorf(fmtWrongCallCount, 3, got)
	}
}

func TestInMemoryBus_Publish_NoSubscribers_IsNoop(t *testing.T) {
	bus := messaging.NewInMemoryBus(nil)
	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestInMemoryBus_Publish_DoesNotCrossDeliver(t *testing.T) {
	bus := messaging.NewInMemoryBus(nil)
	called := false

	bus.Subscribe(events.EventMatchStarted, func(_ context.Context, _ events.Envelope) error {
		called = true
		return nil
	})

	// Publish a different event type — the MatchStarted handler must not fire.
	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
	if called {
		t.Error("handler for EventMatchStarted must not be called for EventMatchFinished")
	}
}

func TestInMemoryBus_Publish_ConcurrentSafe(t *testing.T) {
	bus := messaging.NewInMemoryBus(nil)
	var wg sync.WaitGroup
	received := make(chan struct{}, 20)

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		received <- struct{}{}
		return nil
	})

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = bus.Publish(context.Background(), matchFinishedEnvelope())
		}()
	}
	wg.Wait()

	if len(received) != 20 {
		t.Errorf(fmtWrongCallCount, 20, len(received))
	}
}

func TestInMemoryBus_Publish_RetriesAndDLQ_OnHandlerError(t *testing.T) {
	// Override backoff so retries complete in milliseconds.
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	bus := messaging.NewInMemoryBus(nil)
	var calls int

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		calls++
		return errors.New("transient failure")
	})

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Handler must be called exactly maxHandlerAttempts times (3).
	if calls != 3 {
		t.Errorf("expected 3 handler attempts (retry exhaustion), got %d", calls)
	}
}

func TestInMemoryBus_Publish_SucceedsOnSecondAttempt(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	bus := messaging.NewInMemoryBus(nil)
	attempt := 0

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		attempt++
		if attempt == 1 {
			return errors.New("first attempt fails")
		}
		return nil
	})

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	if attempt != 2 {
		t.Errorf("expected handler called 2 times (fail then succeed), got %d", attempt)
	}
}

// ── RedisBus ──────────────────────────────────────────────────────────────────

func TestRedisBus_Publish_DeliversToSubscriber(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	received := make(chan events.Envelope, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, env events.Envelope) error {
		received <- env
		return nil
	})

	// Give the background consumer goroutine time to subscribe before publishing.
	time.Sleep(50 * time.Millisecond)

	want := matchFinishedEnvelope()
	if err := bus.Publish(context.Background(), want); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	select {
	case got := <-received:
		if got.Type != want.Type {
			t.Errorf(fmtWrongPayload, want.Type, got.Type)
		}
	case <-time.After(2 * time.Second):
		t.Fatal(fmtHandlerNotCalled)
	}
}

func TestRedisBus_Publish_DeliversToMultipleHandlers(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	received := make(chan struct{}, 3)
	for range 3 {
		bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
			received <- struct{}{}
			return nil
		})
	}

	time.Sleep(50 * time.Millisecond)

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	deadline := time.After(2 * time.Second)
	for range 3 {
		select {
		case <-received:
		case <-deadline:
			t.Fatal(fmtHandlerNotCalled)
		}
	}
}

func TestRedisBus_Close_IsIdempotent(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		// No-op handler: this test only needs a live subscription goroutine;
		// the handler body is intentionally empty.
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	// No assertion needed: the test verifies that calling Close more than once
	// does not panic. context.CancelFunc is safe to call multiple times and
	// all registered cancels are idempotent.
	bus.Close()
	bus.Close()
}

func TestRedisBus_Close_SubscriptionCloseError_DoesNotPanic(t *testing.T) {
	mr, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		// No-op handler: this test only needs a live subscription goroutine to
		// exist so that closeSubscription is invoked when the bus shuts down.
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	// No assertion needed: stopping miniredis before Close forces pubsub.Close()
	// to return a connection error, exercising the warning-log branch in
	// closeSubscription without panicking.
	mr.Close()
	bus.Close()
	time.Sleep(100 * time.Millisecond) // allow the goroutine to exit cleanly
}

func TestRedisBus_Publish_DoesNotCrossDeliver(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	called := make(chan struct{}, 1)
	bus.Subscribe(events.EventMatchStarted, func(_ context.Context, _ events.Envelope) error {
		called <- struct{}{}
		return nil
	})

	time.Sleep(50 * time.Millisecond)

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	select {
	case <-called:
		t.Error("handler for EventMatchStarted must not be called for EventMatchFinished")
	case <-time.After(200 * time.Millisecond):
		// correct — no cross-delivery
	}
}

func TestRedisBus_RetriesAndPushesToDLQ_OnHandlerError(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	mr, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	calls := make(chan struct{}, 10)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		calls <- struct{}{}
		return errors.New("transient failure")
	})

	time.Sleep(50 * time.Millisecond)

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Wait for all 3 attempts to complete.
	deadline := time.After(2 * time.Second)
	for range 3 {
		select {
		case <-calls:
		case <-deadline:
			t.Fatal("handler was not retried 3 times within timeout")
		}
	}

	// Allow DLQ push to land.
	time.Sleep(100 * time.Millisecond)

	// Verify the event was pushed to the DLQ list in Redis.
	dlqLen, err := client.LLen(context.Background(), "dlq:match.finished").Result()
	if err != nil {
		t.Fatalf("redis LLEN: %v", err)
	}
	if dlqLen == 0 {
		t.Error("expected at least one entry in dlq:match.finished, got 0")
	}

	// miniredis does not persist between tests; closing it cleans up automatically.
	mr.Close()
}
