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
	errTransientFail    = "fail"
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

	// Publish a different event type - the MatchStarted handler must not fire.
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

func TestInMemoryBus_Publish_ContextCancelled_StopsRetrying(t *testing.T) {
	// Use a long backoff so the cancelled context fires before the sleep ends.
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{500 * time.Millisecond, time.Second}
	defer func() { messaging.RetryBackoff = orig }()

	bus := messaging.NewInMemoryBus(nil)
	calls := 0

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		calls++
		return errors.New(errTransientFail)
	})

	// A pre-cancelled context causes callWithRetry to exit during the first
	// inter-attempt sleep, so the handler is called exactly once.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_ = bus.Publish(ctx, matchFinishedEnvelope())

	if calls != 1 {
		t.Errorf("expected 1 call before context cancelled, got %d", calls)
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
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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
		// correct - no cross-delivery
	}
}

func TestRedisBus_ContextCancelled_StopsRetrying(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{500 * time.Millisecond, time.Second}
	defer func() { messaging.RetryBackoff = orig }()

	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

	calls := make(chan struct{}, 10)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		calls <- struct{}{}
		return errors.New(errTransientFail)
	})
	time.Sleep(50 * time.Millisecond)

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Close cancels the subscription context, which interrupts the retry sleep.
	// Wait for the first handler call then close immediately.
	select {
	case <-calls:
	case <-time.After(2 * time.Second):
		t.Fatal("handler never called")
	}
	bus.Close()

	// Allow the goroutine to process the cancellation.
	time.Sleep(100 * time.Millisecond)

	// At most one additional attempt may have completed before Close fired.
	if len(calls) > 1 {
		t.Errorf("expected at most 1 extra call after Close, got %d", len(calls))
	}
}

func TestRedisBus_DLQPushError_DoesNotPanic(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	mr, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

	// Count handler calls so we know when all retries are exhausted.
	var mu sync.Mutex
	callCount := 0
	ready := make(chan struct{}) // closed after the 3rd attempt

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		mu.Lock()
		callCount++
		done := callCount == 3
		mu.Unlock()
		if done {
			close(ready)
		}
		return errors.New(errTransientFail)
	})
	time.Sleep(50 * time.Millisecond)

	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}

	// Wait until all 3 attempts have run, then kill Redis so the DLQ RPush fails.
	select {
	case <-ready:
		mr.Close() // stop Redis before pushDLQ can write
	case <-time.After(2 * time.Second):
		t.Fatal("handler was not called 3 times within timeout")
	}

	// Give the goroutine time to attempt the failing RPush - must not panic.
	time.Sleep(150 * time.Millisecond)
}

// TestRedisBus_ProcessMessage_MissingPayload_DoesNotPanic verifies that a stream
// message without a "payload" field is acknowledged and skipped without panicking.
func TestRedisBus_ProcessMessage_MissingPayload_DoesNotPanic(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

	handlerCalled := make(chan struct{}, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		handlerCalled <- struct{}{}
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	// Inject a raw message into the stream that has no "payload" field.
	client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "stream:match.finished",
		Values: map[string]any{"not_payload": "ignored"},
	})

	// The handler must NOT be called - the message is dropped at the missing-payload check.
	select {
	case <-handlerCalled:
		t.Error("handler must not be called for a message with no payload field")
	case <-time.After(300 * time.Millisecond):
		// correct - message was silently dropped
	}
}

// TestRedisBus_ProcessMessage_InvalidJSONPayload_DoesNotPanic verifies that a
// stream message whose payload is not valid JSON is acknowledged and skipped.
func TestRedisBus_ProcessMessage_InvalidJSONPayload_DoesNotPanic(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

	handlerCalled := make(chan struct{}, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		handlerCalled <- struct{}{}
		return nil
	})
	time.Sleep(50 * time.Millisecond)

	client.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "stream:match.finished",
		Values: map[string]any{"payload": "not valid json{{"},
	})

	select {
	case <-handlerCalled:
		t.Error("handler must not be called for a message with invalid JSON payload")
	case <-time.After(300 * time.Millisecond):
		// correct - message was silently dropped
	}
}

// TestRedisBus_ConcurrentMessages_ProcessedConcurrently verifies that the
// worker pool allows multiple messages to be processed at the same time.
// The test uses a gate channel to hold handlers in-flight simultaneously
// and confirms that at least N handlers run concurrently (where N > 1).
func TestRedisBus_ConcurrentMessages_ProcessedConcurrently(t *testing.T) {
	// Reduce pool size to 4 so we can saturate it with exactly 4 messages
	// without relying on the default of 8. Restored after the test.
	orig := messaging.StreamWorkerCount
	messaging.StreamWorkerCount = 4
	defer func() { messaging.StreamWorkerCount = orig }()

	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

	const numMessages = 4
	gate := make(chan struct{}) // closed to release all in-flight handlers at once
	done := make(chan struct{}, numMessages)

	var (
		mu          sync.Mutex
		maxInFlight int
		inFlight    int
	)

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) error {
		mu.Lock()
		inFlight++
		if inFlight > maxInFlight {
			maxInFlight = inFlight
		}
		mu.Unlock()

		<-gate // hold until all messages have been dispatched to the pool

		mu.Lock()
		inFlight--
		mu.Unlock()

		done <- struct{}{}
		return nil
	})
	time.Sleep(50 * time.Millisecond) // allow the consume goroutine to start

	for range numMessages {
		if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}

	// Wait until all handlers are blocked inside <-gate (maxInFlight reaches numMessages).
	waitDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(waitDeadline) {
		mu.Lock()
		n := maxInFlight
		mu.Unlock()
		if n == numMessages {
			break
		}
		time.Sleep(time.Millisecond)
	}
	mu.Lock()
	reached := maxInFlight
	mu.Unlock()
	if reached < numMessages {
		t.Fatalf("expected %d concurrent handlers before gate release, got %d — pool may be sequential", numMessages, reached)
	}

	close(gate) // release all in-flight handlers

	collectDeadline := time.After(2 * time.Second)
	for range numMessages {
		select {
		case <-done:
		case <-collectDeadline:
			t.Fatal("not all handlers completed after gate was released")
		}
	}
}

func TestRedisBus_RetriesAndPushesToDLQ_OnHandlerError(t *testing.T) {
	orig := messaging.RetryBackoff
	messaging.RetryBackoff = []time.Duration{time.Millisecond, 2 * time.Millisecond}
	defer func() { messaging.RetryBackoff = orig }()

	mr, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(context.Background(), client, nil)

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

	// Poll until the DLQ push lands (pushDLQ is synchronous in the consumer
	// goroutine but may not have been scheduled yet under -race).
	const dlqKeyName = "dlq:match.finished"
	var dlqLen int64
	pollDeadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(pollDeadline) {
		n, err := client.LLen(context.Background(), dlqKeyName).Result()
		if err != nil {
			t.Fatalf("redis LLEN: %v", err)
		}
		if n > 0 {
			dlqLen = n
			break
		}
		time.Sleep(time.Millisecond)
	}
	if dlqLen == 0 {
		t.Error("expected at least one entry in dlq:match.finished, got 0")
	}

	// miniredis does not persist between tests; closing it cleans up automatically.
	mr.Close()
}
