package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// dlqKey returns the Redis list key used as the dead-letter queue for the
// given event type. Failed events are appended with RPUSH so that the oldest
// entry is at the head of the list, making LRANGE 0 N show them in order.
func dlqKey(eventType events.EventType) string {
	return "dlq:" + string(eventType)
}

// dlqEntry is the payload stored in the dead-letter queue for each event that
// exhausted all handler retry attempts. It carries enough context for an
// operator to identify the event, understand the failure, and replay it.
type dlqEntry struct {
	EventType      string          `json:"event_type"`
	Envelope       events.Envelope `json:"envelope"`
	Error          string          `json:"error"`
	DeadLetteredAt time.Time       `json:"dead_lettered_at"`
	Attempts       int             `json:"attempts"`
}

// RedisBus implements events.Bus using Redis pub/sub as the transport layer.
//
// Published events are serialised to JSON and sent to a Redis channel whose
// name matches the EventType string. Subscribers receive messages from Redis
// on a background goroutine and dispatch them to registered handlers.
//
// Each handler is retried up to maxHandlerAttempts times with exponential
// backoff (see retry.go). Events that exhaust all attempts are pushed to a
// Redis list at dlq:<event_type> so operators can inspect and replay them.
//
// Redis pub/sub is fire-and-forget: messages are not persisted and a
// subscriber that is offline during publication will miss events. For this
// project's requirements (real-time scoring after match completion) this is
// acceptable. If at-least-once delivery becomes a requirement, migrate to
// Redis Streams or a dedicated message broker.
//
// Call Close to stop all active subscription goroutines and release their
// Redis connections. The underlying Redis client is not owned by the bus;
// the caller must close it separately after calling Close.
type RedisBus struct {
	client   *redis.Client
	log      *zap.Logger
	cancels  []context.CancelFunc
	mu       sync.RWMutex
	handlers map[events.EventType][]func(context.Context, events.Envelope) error
}

// NewRedisBus constructs a RedisBus that publishes and subscribes using the
// provided Redis client. The client is not owned by the bus and must be closed
// by the caller after Close has been called.
// If log is nil a no-op logger is used so that tests do not need to provide one.
func NewRedisBus(client *redis.Client, log *zap.Logger) *RedisBus {
	if log == nil {
		log = zap.NewNop()
	}
	return &RedisBus{
		client:   client,
		log:      log,
		handlers: make(map[events.EventType][]func(context.Context, events.Envelope) error),
	}
}

// Close cancels all active subscription goroutines. It is safe to call
// Close multiple times; subsequent calls are no-ops.
func (b *RedisBus) Close() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, cancel := range b.cancels {
		cancel()
	}
}

// Subscribe registers handler and starts a Redis subscription goroutine for
// eventType if one is not already running. Handlers must return an error to
// signal transient failures; the bus retries up to maxHandlerAttempts times
// before pushing the event to the dead-letter queue at dlq:<event_type>.
func (b *RedisBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope) error) {
	b.mu.Lock()
	existing := b.handlers[eventType]
	b.handlers[eventType] = append(existing, handler)

	// Start the Redis consumer goroutine only on the first handler registration
	// for this event type, avoiding duplicate consumers. The context and cancel
	// are created inside the lock so Close cannot race with Subscribe.
	var ctx context.Context
	if len(existing) == 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(context.Background())
		b.cancels = append(b.cancels, cancel)
	}
	b.mu.Unlock()

	if len(existing) == 0 {
		go b.consume(ctx, eventType)
	}
}

// Publish serialises envelope to JSON and sends it to the Redis channel for
// its EventType. Returns an error if marshalling or publication fails.
func (b *RedisBus) Publish(ctx context.Context, envelope events.Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("redis bus: marshal envelope: %w", err)
	}
	if err := b.client.Publish(ctx, string(envelope.Type), data).Err(); err != nil {
		return fmt.Errorf("redis bus: publish %s: %w", envelope.Type, err)
	}
	return nil
}

// closeSubscription closes a Redis pub/sub handle and logs a warning on failure.
// Extracted from consume's defer to keep cognitive complexity low and allow the
// error path to be exercised in isolation.
func (b *RedisBus) closeSubscription(pubsub *redis.PubSub, eventType events.EventType) {
	if err := pubsub.Close(); err != nil {
		b.log.Warn("redis bus: failed to close subscription",
			zap.String("event_type", string(eventType)),
			zap.Error(err),
		)
	}
}

// consume runs a blocking Redis subscription loop for the given event type.
// It exits when ctx is cancelled (i.e. Close is called), which causes the
// Redis pub/sub channel to be drained and closed. Each message is dispatched
// to all registered handlers with retry; failures are pushed to the DLQ.
func (b *RedisBus) consume(ctx context.Context, eventType events.EventType) {
	pubsub := b.client.Subscribe(ctx, string(eventType))
	defer b.closeSubscription(pubsub, eventType)

	ch := pubsub.Channel()
	for msg := range ch {
		var envelope events.Envelope
		if err := json.Unmarshal([]byte(msg.Payload), &envelope); err != nil {
			b.log.Error("redis bus: unmarshal envelope",
				zap.String("event_type", string(eventType)),
				zap.Error(err),
			)
			continue
		}

		b.mu.RLock()
		handlers := b.handlers[eventType]
		b.mu.RUnlock()

		// Detach cancellation from the subscription context so that handlers
		// are not aborted when Close() cancels the consumer goroutine mid-message.
		// context.WithoutCancel preserves any values on ctx (e.g. trace IDs set
		// by the subscriber) while removing the cancellation signal, giving each
		// handler a clean deadline-free context that still carries observability
		// metadata — unlike context.Background() which discards all values.
		handlerCtx := context.WithoutCancel(ctx)
		for _, h := range handlers {
			h := h // capture loop variable
			if err := callWithRetry(handlerCtx, func() error { return h(handlerCtx, envelope) }); err != nil {
				b.pushDLQ(envelope, err)
			}
		}
	}
}

// pushDLQ appends a dead-letter entry to the Redis list at dlq:<event_type>.
// Operators can inspect the list with LRANGE dlq:<event_type> 0 -1 and replay
// events by re-publishing the stored envelope JSON to the appropriate channel.
func (b *RedisBus) pushDLQ(envelope events.Envelope, handlerErr error) {
	entry := dlqEntry{
		EventType:      string(envelope.Type),
		Envelope:       envelope,
		Error:          handlerErr.Error(),
		DeadLetteredAt: time.Now().UTC(),
		Attempts:       maxHandlerAttempts,
	}
	data, err := json.Marshal(entry)
	if err != nil {
		b.log.Error("redis bus: marshal dlq entry",
			zap.String("event_type", string(envelope.Type)),
			zap.Error(err),
		)
		return
	}
	// RPUSH keeps entries in arrival order; LRANGE 0 N shows oldest-first.
	// Use a fresh context so a cancelled handler context does not block the push.
	if err := b.client.RPush(context.Background(), dlqKey(envelope.Type), data).Err(); err != nil {
		b.log.Error("redis bus: push to dlq",
			zap.String("event_type", string(envelope.Type)),
			zap.Error(err),
		)
	}
	b.log.Error("event handler failed after all retries — dead-lettered",
		zap.String("event_type", string(envelope.Type)),
		zap.String("dlq_key", dlqKey(envelope.Type)),
		zap.Int("attempts", maxHandlerAttempts),
		zap.Error(handlerErr),
	)
}
