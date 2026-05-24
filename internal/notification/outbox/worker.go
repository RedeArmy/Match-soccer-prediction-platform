package outbox

import (
	"context"
	"math/rand/v2"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

const (
	defaultBatchSize    = domain.DefaultNotifyOutboxBatchSize
	defaultPollInterval = domain.DefaultNotifyOutboxPollIntervalSec * time.Second
	defaultLockDuration = domain.DefaultNotifyOutboxLockDurationSec * time.Second
	defaultMaxAttempts  = domain.DefaultNotifyOutboxMaxAttempts
)

// Dispatcher is the delivery interface called by the Worker for each claimed
// outbox entry.  Phase 0 uses a no-op implementation; Phase 1 replaces it with
// the real fan-out dispatcher (SSE → Web Push → Email).
type Dispatcher interface {
	Dispatch(ctx context.Context, entry *notification.OutboxEntry) error
}

// outboxLagNotifier is the narrow interface consumed by Worker for alerting
// when outbox processing lags behind the ingest rate. The concrete type is
// *observability.Notifier; the interface keeps the outbox package import-free
// of the observability layer.
type outboxLagNotifier interface {
	NotifyOutboxLag(ctx context.Context, lagSeconds float64, pendingCount int64)
}

const defaultLagAlertThreshold = domain.DefaultNotifyOutboxLagAlertThresholdSec * time.Second

// Worker polls domain_outbox and dispatches entries to the configured
// Dispatcher.  Run blocks until ctx is cancelled; it is safe to run as a
// goroutine alongside the HTTP server or as a standalone worker process.
//
// Concurrency: Run itself is single-threaded per instance.  Horizontal scale
// is achieved by running multiple Worker instances against the same database;
// SELECT FOR UPDATE SKIP LOCKED in ClaimBatch guarantees each entry is
// processed by exactly one instance at a time.
type Worker struct {
	repo              Repository
	dispatcher        Dispatcher
	log               *zap.Logger
	batchSize         int
	pollInterval      time.Duration
	lockDuration      time.Duration
	maxAttempts       int
	lagNotifier       outboxLagNotifier
	lagAlertThreshold time.Duration
}

// WorkerOption is a functional option for Worker.
type WorkerOption func(*Worker)

// WithBatchSize overrides the number of outbox rows claimed per poll cycle.
func WithBatchSize(n int) WorkerOption {
	return func(w *Worker) { w.batchSize = n }
}

// WithPollInterval overrides the sleep duration between poll cycles.
func WithPollInterval(d time.Duration) WorkerOption {
	return func(w *Worker) { w.pollInterval = d }
}

// WithLockDuration overrides how long a claimed row is locked before the
// stale-lock recovery job reclaims it.
func WithLockDuration(d time.Duration) WorkerOption {
	return func(w *Worker) { w.lockDuration = d }
}

// WithMaxAttempts overrides the maximum number of dispatch attempts before an
// outbox entry is permanently marked failed.
func WithMaxAttempts(n int) WorkerOption {
	return func(w *Worker) { w.maxAttempts = n }
}

// WithOutboxNotifier wires an observability.Notifier so that the Worker fires
// an n8n webhook whenever claimed entries are older than the lag alert
// threshold. Pass nil to disable (identical to omitting this option).
func WithOutboxNotifier(n outboxLagNotifier) WorkerOption {
	return func(w *Worker) { w.lagNotifier = n }
}

// WithOutboxLagThreshold overrides the age threshold above which a pending
// entry triggers a NotifyOutboxLag alert. Defaults to 30 s.
func WithOutboxLagThreshold(d time.Duration) WorkerOption {
	return func(w *Worker) { w.lagAlertThreshold = d }
}

// NewWorker constructs a Worker.  dispatcher is called for every entry the
// worker claims; pass a NoopDispatcher for Phase 0.
func NewWorker(repo Repository, dispatcher Dispatcher, log *zap.Logger, opts ...WorkerOption) *Worker {
	w := &Worker{
		repo:              repo,
		dispatcher:        dispatcher,
		log:               log,
		batchSize:         defaultBatchSize,
		pollInterval:      defaultPollInterval,
		lockDuration:      defaultLockDuration,
		maxAttempts:       defaultMaxAttempts,
		lagAlertThreshold: defaultLagAlertThreshold,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Run starts the polling loop.  It blocks until ctx is cancelled.
// Errors from individual poll cycles are logged but do not stop the loop;
// only ctx cancellation terminates Run.
func (w *Worker) Run(ctx context.Context) {
	w.log.Info("outbox worker started",
		zap.Int("batch_size", w.batchSize),
		zap.Duration("poll_interval", w.pollInterval),
		zap.Duration("lock_duration", w.lockDuration),
	)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	// Run an immediate stale-unlock on startup to recover any rows left
	// behind by a previously crashed instance.
	w.unlockStale(ctx)

	for {
		select {
		case <-ctx.Done():
			w.log.Info("outbox worker stopped")
			return
		case <-ticker.C:
			w.unlockStale(ctx)
			if err := w.poll(ctx); err != nil {
				w.log.Error("outbox poll cycle failed", zap.Error(err))
			}
		}
	}
}

// poll claims a batch of pending outbox entries, dispatches each one, and
// updates its final status (done or scheduled for retry).
func (w *Worker) poll(ctx context.Context) error {
	entries, err := w.repo.ClaimBatch(ctx, w.batchSize, w.lockDuration)
	if err != nil {
		return err
	}
	w.checkLag(ctx, entries)
	for _, entry := range entries {
		w.process(ctx, entry)
	}
	return nil
}

// checkLag fires NotifyOutboxLag when the oldest entry in the claimed batch
// has been waiting longer than lagAlertThreshold. Using CreatedAt (original
// enqueue time) rather than ScheduledAt avoids false negatives on retry
// entries whose scheduled_at has been bumped forward by exponential backoff.
func (w *Worker) checkLag(ctx context.Context, entries []*notification.OutboxEntry) {
	if w.lagNotifier == nil || len(entries) == 0 {
		return
	}
	oldest := entries[0]
	for _, e := range entries[1:] {
		if e.CreatedAt.Before(oldest.CreatedAt) {
			oldest = e
		}
	}
	if lag := time.Since(oldest.CreatedAt); lag > w.lagAlertThreshold {
		w.lagNotifier.NotifyOutboxLag(ctx, lag.Seconds(), int64(len(entries)))
	}
}

// process dispatches a single entry and marks it done or retries it.
func (w *Worker) process(ctx context.Context, entry *notification.OutboxEntry) {
	ctx, span := otel.Tracer("outbox").Start(ctx, "outbox.process")
	span.SetAttributes(
		attribute.String("event_type", string(entry.EventType)),
		attribute.Int64("outbox_id", entry.ID),
		attribute.String("aggregate_type", entry.AggregateType),
		attribute.String("aggregate_id", entry.AggregateID),
		attribute.Int("attempt", entry.Attempts),
	)
	defer span.End()

	log := w.log.With(
		append([]zap.Field{
			zap.Int64("outbox_id", entry.ID),
			zap.String("event_type", string(entry.EventType)),
			zap.String("aggregate_type", entry.AggregateType),
			zap.String("aggregate_id", entry.AggregateID),
			zap.Int("attempt", entry.Attempts),
		}, tracing.LogFields(ctx)...)...,
	)

	if err := w.dispatcher.Dispatch(ctx, entry); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "dispatch failed")
		log.Warn("outbox dispatch failed", zap.Error(err))
		if entry.Attempts >= w.maxAttempts {
			if mfErr := w.repo.MarkFailed(ctx, entry.ID, err.Error()); mfErr != nil {
				log.Error("failed to mark outbox entry as failed", zap.Error(mfErr))
			} else {
				log.Error("outbox entry exhausted max attempts — marked failed",
					zap.Int("max_attempts", w.maxAttempts),
				)
			}
			return
		}
		// Exponential backoff with equal jitter: delay = half_fixed + rand[0, half_fixed]
		full := min(time.Duration(1<<entry.Attempts)*time.Second, 5*time.Minute)
		half := full / 2
		delay := half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // G404: jitter for backoff; cryptographic randomness not required
		scheduledAt := time.Now().Add(delay)
		if schedErr := w.repo.Schedule(ctx, entry.ID, scheduledAt); schedErr != nil {
			log.Error("failed to reschedule outbox entry", zap.Error(schedErr))
		} else {
			log.Info("outbox entry rescheduled",
				zap.Duration("backoff", delay),
				zap.Time("scheduled_at", scheduledAt),
			)
		}
		return
	}

	if err := w.repo.MarkDone(ctx, entry.ID); err != nil {
		log.Error("failed to mark outbox entry done", zap.Error(err))
		return
	}
	log.Info("outbox entry dispatched and marked done")
}

// unlockStale reclaims processing rows whose lock has expired.
func (w *Worker) unlockStale(ctx context.Context) {
	n, err := w.repo.UnlockStale(ctx)
	if err != nil {
		w.log.Warn("outbox stale-unlock failed", zap.Error(err))
		return
	}
	if n > 0 {
		w.log.Warn("unlocked stale outbox entries", zap.Int64("count", n))
	}
}

// NoopDispatcher is a Dispatcher that logs and succeeds without delivering.
// Used in Phase 0 before real delivery channels are wired.
type NoopDispatcher struct {
	log *zap.Logger
}

// NewNoopDispatcher constructs a NoopDispatcher.
func NewNoopDispatcher(log *zap.Logger) Dispatcher {
	return &NoopDispatcher{log: log}
}

// Dispatch logs the entry and returns nil so the worker marks it done.
func (d *NoopDispatcher) Dispatch(ctx context.Context, entry *notification.OutboxEntry) error {
	d.log.Info("outbox noop dispatch",
		zap.String("event_type", string(entry.EventType)),
		zap.String("aggregate_type", entry.AggregateType),
		zap.String("aggregate_id", entry.AggregateID),
	)
	return nil
}
