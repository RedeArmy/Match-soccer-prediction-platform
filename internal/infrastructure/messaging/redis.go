package messaging

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// RedisBus implements events.Bus using Redis pub/sub as the transport layer.
//
// Published events are serialised to JSON and sent to a Redis channel whose
// name matches the EventType string. Subscribers receive messages from Redis
// on a background goroutine and dispatch them to registered handlers.
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
	handlers map[events.EventType][]func(context.Context, events.Envelope)
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
		handlers: make(map[events.EventType][]func(context.Context, events.Envelope)),
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
// eventType if one is not already running. A cancellable context is created
// per unique event type; calling Close cancels all of them.
func (b *RedisBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope)) {
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
// Redis pub/sub channel to be drained and closed.
// Messages are unmarshalled and dispatched to all registered handlers.
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

		ctx := context.Background()
		for _, h := range handlers {
			h(ctx, envelope)
		}
	}
}
