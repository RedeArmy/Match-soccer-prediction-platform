package messaging_test

import (
	"context"
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
	fmtHandlerNotCalled  = "handler was not called within timeout"
	fmtWrongPayload      = "expected payload %v, got %v"
	fmtUnexpectedErr     = "unexpected error: %v"
	fmtWrongCallCount    = "expected handler call count %d, got %d"
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
	bus := messaging.NewInMemoryBus()
	want := matchFinishedEnvelope()

	received := make(chan events.Envelope, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, env events.Envelope) {
		received <- env
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
	bus := messaging.NewInMemoryBus()
	var mu sync.Mutex
	count := 0

	for range 3 {
		bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) {
			mu.Lock()
			count++
			mu.Unlock()
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
	bus := messaging.NewInMemoryBus()
	if err := bus.Publish(context.Background(), matchFinishedEnvelope()); err != nil {
		t.Fatalf(fmtUnexpectedErr, err)
	}
}

func TestInMemoryBus_Publish_DoesNotCrossDeliver(t *testing.T) {
	bus := messaging.NewInMemoryBus()
	called := false

	bus.Subscribe(events.EventMatchStarted, func(_ context.Context, _ events.Envelope) {
		called = true
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
	bus := messaging.NewInMemoryBus()
	var wg sync.WaitGroup
	received := make(chan struct{}, 20)

	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) {
		received <- struct{}{}
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

// ── RedisBus ──────────────────────────────────────────────────────────────────

func TestRedisBus_Publish_DeliversToSubscriber(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	received := make(chan events.Envelope, 1)
	bus.Subscribe(events.EventMatchFinished, func(_ context.Context, env events.Envelope) {
		received <- env
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
		bus.Subscribe(events.EventMatchFinished, func(_ context.Context, _ events.Envelope) {
			received <- struct{}{}
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

func TestRedisBus_Publish_DoesNotCrossDeliver(t *testing.T) {
	_, client := newMiniRedis(t)
	bus := messaging.NewRedisBus(client, nil)

	called := make(chan struct{}, 1)
	bus.Subscribe(events.EventMatchStarted, func(_ context.Context, _ events.Envelope) {
		called <- struct{}{}
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
