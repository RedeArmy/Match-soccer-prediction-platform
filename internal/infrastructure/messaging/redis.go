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
	ctx      context.Context
	cancel   context.CancelFunc
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
	ctx, cancel := context.WithCancel(context.Background())
	return &RedisBus{
		client:   client,
		log:      log,
		ctx:      ctx,
		cancel:   cancel,
		handlers: make(map[events.EventType][]func(context.Context, events.Envelope)),
	}
}

// Close cancels the bus's internal context, which causes all active
// subscription goroutines to exit once the Redis pub/sub channel is closed.
// It is safe to call Close multiple times.
func (b *RedisBus) Close() {
	b.cancel()
}

// Subscribe registers handler and starts a Redis subscription goroutine for
// eventType if one is not already running. The goroutine exits when Close is
// called, which cancels the bus's internal context and causes the underlying
// Redis subscription channel to be closed.
func (b *RedisBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope)) {
	b.mu.Lock()
	existing := b.handlers[eventType]
	b.handlers[eventType] = append(existing, handler)
	b.mu.Unlock()

	// Start the Redis consumer goroutine only on the first handler registration
	// for this event type, avoiding duplicate consumers.
	if len(existing) == 0 {
		go b.consume(eventType)
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

// consume runs a blocking Redis subscription loop for the given event type.
// It exits when the bus's internal context is cancelled (i.e. Close is called),
// which causes the Redis pub/sub channel to be drained and closed.
// Messages are unmarshalled and dispatched to all registered handlers.
func (b *RedisBus) consume(eventType events.EventType) {
	pubsub := b.client.Subscribe(b.ctx, string(eventType))
	defer func() {
		if err := pubsub.Close(); err != nil {
			b.log.Warn("redis bus: failed to close subscription",
				zap.String("event_type", string(eventType)),
				zap.Error(err),
			)
		}
	}()

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
