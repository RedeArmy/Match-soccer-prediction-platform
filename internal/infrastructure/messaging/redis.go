package messaging

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain/events"
)

// consumerGroup is the Redis Streams consumer group name shared by all
// instances of this service. Every instance competes for messages within the
// same group, so each message is delivered to exactly one instance.
const consumerGroup = "quiniela-workers"

// streamReadBlock is the maximum time XReadGroup blocks waiting for new
// messages before returning an empty result. A finite timeout lets the
// consume loop check for context cancellation (bus shutdown) without relying
// solely on the Redis connection being closed.
// Overridable at startup via Configure (reads messaging.stream_read_block_sec from system_params).
var streamReadBlock = 5 * time.Second

// streamMaxLen caps the length of each event stream. Older acknowledged
// entries are trimmed approximately (MAXLEN ~) to keep memory bounded.
// At ~1 event per second this retains roughly 7 days of history.
// Overridable at startup via Configure (reads messaging.stream_max_len from system_params).
var streamMaxLen int64 = 600_000

// StreamWorkerCount is the number of goroutines in the worker pool spawned per
// EventType subscription. Messages for different events (e.g. two MatchFinished
// events for different matches) are processed concurrently up to this limit.
// Raising the value increases burst throughput at the cost of more concurrent
// DB/Redis connections. Must be set before Subscribe is called.
var StreamWorkerCount = 8

// dlqKey returns the Redis list key used as the dead-letter queue for the
// given event type. Failed events are appended with RPUSH so that the oldest
// entry is at the head of the list, making LRANGE 0 N show them in order.
func dlqKey(eventType events.EventType) string {
	return "dlq:" + string(eventType)
}

// streamKey returns the Redis Streams key for the given event type.
// Prefixed with "stream:" to avoid collision with legacy pub/sub channels.
func streamKey(eventType events.EventType) string {
	return "stream:" + string(eventType)
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

// RedisBus implements events.Bus using Redis Streams as the transport layer.
//
// Redis Streams provide at-least-once delivery: messages are persisted in the
// stream and assigned to a consumer group. A subscriber that is offline when
// a message arrives will receive it on restart via the pending-entry recovery
// path (consume loop first reads ID "0" to claim any unacknowledged messages
// from a previous run before switching to ">" for new ones).
//
// Each successfully processed message is acknowledged with XACK and trimmed
// from the stream lazily via MAXLEN. Messages that exhaust all handler retry
// attempts are pushed to a Redis list at dlq:<event_type> for manual replay.
//
// Call Close to stop all active consumer goroutines. The underlying Redis
// client is not owned by the bus; the caller must close it separately after
// Close returns.
type RedisBus struct {
	client       *redis.Client
	log          *zap.Logger
	baseCtx      context.Context // root context for all consumer goroutines
	consumerName string          // unique per process; prevents two replicas from stealing each other's PEL
	cancels      []context.CancelFunc
	mu           sync.RWMutex
	handlers     map[events.EventType][]func(context.Context, events.Envelope) error
}

// NewRedisBus constructs a RedisBus that publishes and subscribes using the
// provided Redis client. ctx is the application lifecycle context: all consumer
// goroutines are children of it, so a SIGTERM-triggered cancellation propagates
// to the bus without an explicit Close call. The client is not owned by the bus
// and must be closed by the caller after Close has been called.
// If log is nil a no-op logger is used so that tests do not need to provide one.
func NewRedisBus(ctx context.Context, client *redis.Client, log *zap.Logger) *RedisBus {
	if log == nil {
		log = zap.NewNop()
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}
	return &RedisBus{
		client:       client,
		log:          log,
		baseCtx:      ctx,
		consumerName: fmt.Sprintf("%s-%d", hostname, os.Getpid()),
		handlers:     make(map[events.EventType][]func(context.Context, events.Envelope) error),
	}
}

// Close cancels all active consumer goroutines. It is safe to call Close
// multiple times; subsequent calls are no-ops.
func (b *RedisBus) Close() {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, cancel := range b.cancels {
		cancel()
	}
}

// Subscribe registers handler and starts a Redis Streams consumer goroutine for
// eventType if one is not already running. The consumer group is created
// idempotently on first registration. Handlers must return an error to signal
// transient failures; the bus retries up to maxHandlerAttempts times before
// pushing the event to the dead-letter queue at dlq:<event_type>.
//
// The consumer group is created inside the consume goroutine (not before launch)
// to avoid a race where the goroutine starts reading from a group that failed to
// be created due to transient Redis unavailability. If group creation fails, the
// goroutine exits cleanly and logs an error rather than spinning in a retry loop.
func (b *RedisBus) Subscribe(eventType events.EventType, handler func(context.Context, events.Envelope) error) {
	b.mu.Lock()
	existing := b.handlers[eventType]
	b.handlers[eventType] = append(existing, handler)

	var ctx context.Context
	if len(existing) == 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(b.baseCtx)
		b.cancels = append(b.cancels, cancel)
	}
	b.mu.Unlock()

	if len(existing) == 0 {
		go b.consume(ctx, eventType)
	}
}

// Publish serialises envelope to JSON and appends it to the Redis Stream for
// its EventType. Returns an error if marshalling or the XADD command fails.
func (b *RedisBus) Publish(ctx context.Context, envelope events.Envelope) error {
	data, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("redis bus: marshal envelope: %w", err)
	}
	if err := b.client.XAdd(ctx, &redis.XAddArgs{
		Stream: streamKey(envelope.Type),
		MaxLen: streamMaxLen,
		Approx: true, // MAXLEN ~ for O(1) amortised trimming
		Values: map[string]any{"payload": string(data)},
	}).Err(); err != nil {
		return fmt.Errorf("redis bus: XADD %s: %w", envelope.Type, err)
	}
	return nil
}

// ensureConsumerGroup creates the consumer group for eventType if it does not
// already exist. "0" as the start ID means the group will receive all entries
// already in the stream; this is intentional - if a new subscriber is added
// to a stream that already has events it should process them from the beginning.
// MKSTREAM creates the stream key if it does not yet exist.
//
// Returns true if the group exists (either created now or already present),
// false if creation failed due to a non-recoverable Redis error.
func (b *RedisBus) ensureConsumerGroup(ctx context.Context, eventType events.EventType) bool {
	err := b.client.XGroupCreateMkStream(
		ctx,
		streamKey(eventType),
		consumerGroup,
		"0",
	).Err()
	// BUSYGROUP is returned when the group already exists - safe to ignore.
	if err != nil && !isBusyGroup(err) {
		b.log.Error("redis bus: failed to create consumer group",
			zap.String("stream", streamKey(eventType)),
			zap.String("group", consumerGroup),
			zap.Error(err),
		)
		return false
	}
	return true
}

// consume runs the Redis Streams consumer loop for the given event type.
//
// On startup it first ensures the consumer group exists. If group creation
// fails (e.g., Redis temporarily unavailable), the goroutine exits cleanly
// rather than entering a retry loop with no valid consumer group.
//
// Once the group is verified, it reads pending entries (ID "0") to recover
// any messages that were delivered in a previous run but not acknowledged
// before the process crashed. Once the pending list is drained it switches to
// reading new entries (ID ">"). This two-phase approach provides at-least-once
// delivery: a message is never lost unless it is explicitly acknowledged.
//
// Concurrency model — worker pool:
//
// A fixed pool of StreamWorkerCount goroutines processes messages concurrently.
// The read loop dispatches each message to the pool via a buffered work channel
// that provides backpressure: the loop blocks when all workers are busy, so
// Redis is never read faster than the handlers can process. Events for
// different matches run in parallel; the underlying handler (ScoreMatch) is
// idempotent, so the relaxed delivery order is safe.
//
// On shutdown (ctx cancelled), the read loop exits, the work channel is closed,
// and the pool drains all in-flight messages before consume returns. No
// acknowledged-pending entries are lost during graceful shutdown.
func (b *RedisBus) consume(ctx context.Context, eventType events.EventType) {
	key := streamKey(eventType)

	if !b.ensureConsumerGroup(ctx, eventType) {
		b.log.Error("redis bus: consumer group setup failed, goroutine exiting",
			zap.String("stream", key),
		)
		return
	}

	// Snapshot the pool size once so a mid-flight change to StreamWorkerCount
	// (e.g. in tests) cannot cause a mismatch between channel capacity and worker count.
	workers := StreamWorkerCount
	if workers < 1 {
		workers = 1
	}

	// workCh is the bounded queue between the read loop and the worker pool.
	// Capacity 2× workers lets the read loop stay one batch ahead without
	// blocking, while still bounding peak goroutine memory.
	workCh := make(chan redis.XMessage, workers*2)

	var poolWg sync.WaitGroup
	for range workers {
		poolWg.Add(1)
		go func() {
			defer poolWg.Done()
			for msg := range workCh {
				b.processMessage(ctx, eventType, key, msg)
			}
		}()
	}
	// Drain the pool before returning so every in-flight message is either
	// processed+ACKed or left in the Redis PEL for the next startup to recover.
	defer func() {
		close(workCh)
		poolWg.Wait()
	}()

	// dispatch sends msg to the worker pool, honouring bus shutdown so the
	// read loop is not stuck blocking on workCh when ctx is cancelled.
	dispatch := func(msg redis.XMessage) {
		select {
		case workCh <- msg:
		case <-ctx.Done():
		}
	}

	// Phase 1: recover unacknowledged messages from a previous run.
	b.recoverPending(ctx, eventType, key, dispatch)

	// Phase 2: read new messages indefinitely until the bus is closed.
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := b.readStreamMessages(ctx, key)
		if err != nil {
			if b.shouldExitOnError(err) {
				return
			}
			continue
		}

		b.processStreamMessages(msgs, dispatch)
	}
}

// readStreamMessages attempts to read messages from the Redis stream.
// Returns nil error for non-fatal conditions (timeout, transient errors).
func (b *RedisBus) readStreamMessages(ctx context.Context, key string) ([]redis.XStream, error) {
	msgs, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    consumerGroup,
		Consumer: b.consumerName,
		Streams:  []string{key, ">"},
		Block:    streamReadBlock,
		Count:    10,
	}).Result()
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		if errors.Is(err, redis.Nil) {
			// Timeout with no new messages - return nil to continue loop.
			return nil, nil
		}
		b.log.Error("redis bus: XReadGroup error",
			zap.String("stream", key),
			zap.Error(err),
		)
		return nil, nil
	}
	return msgs, nil
}

// shouldExitOnError determines if consume() should exit based on error type.
func (b *RedisBus) shouldExitOnError(err error) bool {
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

// processStreamMessages dispatches all messages in the stream result to the
// worker pool via dispatch. It does not process messages directly; the pool
// goroutines call processMessage to maintain bounded concurrency.
func (b *RedisBus) processStreamMessages(msgs []redis.XStream, dispatch func(redis.XMessage)) {
	for i := range msgs {
		for _, msg := range msgs[i].Messages {
			dispatch(msg)
		}
	}
}

// recoverPending reads and reprocesses any pending entries (delivered but not
// yet acknowledged) left over from a previous consumer run. It drains the
// pending list in batches until no more entries remain, dispatching each
// message to the worker pool via dispatch. startID advances after each
// dispatch so a restart mid-recovery does not skip entries: unACKed messages
// remain in the Redis PEL and are recovered again on the next startup.
func (b *RedisBus) recoverPending(ctx context.Context, eventType events.EventType, key string, dispatch func(redis.XMessage)) {
	startID := "0"
	for {
		msgs, err := b.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    consumerGroup,
			Consumer: b.consumerName,
			Streams:  []string{key, startID},
			Count:    10,
		}).Result()
		if err != nil || len(msgs) == 0 || len(msgs[0].Messages) == 0 {
			return
		}
		for _, msg := range msgs[0].Messages {
			dispatch(msg)
			startID = msg.ID
		}
	}
}

// processMessage dispatches a single stream message to all registered handlers,
// acknowledges the message unconditionally, and sends it to the DLQ if all
// handler attempts were exhausted.
//
// XACK is called regardless of handler outcome because retry logic is already
// handled by callWithRetry. If all retries fail the event is preserved in the
// DLQ for manual replay; keeping it in the PEL would require a separate
// redelivery process and adds operational complexity without a clear benefit
// given the DLQ already provides recovery capability.
func (b *RedisBus) processMessage(ctx context.Context, eventType events.EventType, key string, msg redis.XMessage) {
	raw, ok := msg.Values["payload"].(string)
	if !ok {
		b.log.Error("redis bus: stream message missing 'payload' field",
			zap.String("stream", key),
			zap.String("id", msg.ID),
		)
		b.ack(ctx, key, msg.ID)
		return
	}

	var envelope events.Envelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		b.log.Error("redis bus: unmarshal envelope",
			zap.String("stream", key),
			zap.String("id", msg.ID),
			zap.Error(err),
		)
		b.ack(ctx, key, msg.ID)
		return
	}

	b.mu.RLock()
	handlers := b.handlers[eventType]
	b.mu.RUnlock()

	// handlerCtx strips cancellation so bus shutdown does not abort a handler
	// mid-execution. The original ctx is passed to callWithRetry so that the
	// inter-attempt sleep is interrupted on shutdown.
	handlerCtx := context.WithoutCancel(ctx)
	for _, h := range handlers {
		h := h
		if err := callWithRetry(ctx, func() error { return h(handlerCtx, envelope) }); err != nil {
			b.pushDLQ(ctx, envelope, err)
		}
	}

	b.ack(ctx, key, msg.ID)
}

// ack sends XACK for the given message ID and logs a warning on failure.
// WithoutCancel ensures XACK completes even during graceful shutdown when
// the consume goroutine's context is already cancelled.
func (b *RedisBus) ack(ctx context.Context, key, msgID string) {
	if err := b.client.XAck(context.WithoutCancel(ctx), key, consumerGroup, msgID).Err(); err != nil {
		b.log.Warn("redis bus: XACK failed",
			zap.String("stream", key),
			zap.String("id", msgID),
			zap.Error(err),
		)
	}
}

// pushDLQ appends a dead-letter entry to the Redis list at dlq:<event_type>.
func (b *RedisBus) pushDLQ(ctx context.Context, envelope events.Envelope, handlerErr error) {
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
	if err := b.client.RPush(context.WithoutCancel(ctx), dlqKey(envelope.Type), data).Err(); err != nil {
		b.log.Error("redis bus: push to dlq",
			zap.String("event_type", string(envelope.Type)),
			zap.Error(err),
		)
	}
	b.log.Error("event handler failed after all retries - dead-lettered",
		zap.String("event_type", string(envelope.Type)),
		zap.String("dlq_key", dlqKey(envelope.Type)),
		zap.Int("attempts", maxHandlerAttempts),
		zap.Error(handlerErr),
	)
}

// isBusyGroup reports whether err is the Redis BUSYGROUP error returned when
// a consumer group with the requested name already exists.
func isBusyGroup(err error) bool {
	return err != nil && strings.HasPrefix(err.Error(), "BUSYGROUP")
}
