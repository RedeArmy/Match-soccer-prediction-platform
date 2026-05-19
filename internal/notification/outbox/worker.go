package outbox

import (
	"context"
	"math/rand/v2"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/notification"
)

const (
	defaultBatchSize    = 50
	defaultPollInterval = 2 * time.Second
	defaultLockDuration = 5 * time.Minute
	defaultMaxAttempts  = 5
)

// Dispatcher is the delivery interface called by the Worker for each claimed
// outbox entry.  Phase 0 uses a no-op implementation; Phase 1 replaces it with
// the real fan-out dispatcher (SSE → Web Push → Email).
type Dispatcher interface {
	Dispatch(ctx context.Context, entry *notification.OutboxEntry) error
}

// Worker polls domain_outbox and dispatches entries to the configured
// Dispatcher.  Run blocks until ctx is cancelled; it is safe to run as a
// goroutine alongside the HTTP server or as a standalone worker process.
//
// Concurrency: Run itself is single-threaded per instance.  Horizontal scale
// is achieved by running multiple Worker instances against the same database;
// SELECT FOR UPDATE SKIP LOCKED in ClaimBatch guarantees each entry is
// processed by exactly one instance at a time.
type Worker struct {
	repo         Repository
	dispatcher   Dispatcher
	log          *zap.Logger
	batchSize    int
	pollInterval time.Duration
	lockDuration time.Duration
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

// NewWorker constructs a Worker.  dispatcher is called for every entry the
// worker claims; pass a NoopDispatcher for Phase 0.
func NewWorker(repo Repository, dispatcher Dispatcher, log *zap.Logger, opts ...WorkerOption) *Worker {
	w := &Worker{
		repo:         repo,
		dispatcher:   dispatcher,
		log:          log,
		batchSize:    defaultBatchSize,
		pollInterval: defaultPollInterval,
		lockDuration: defaultLockDuration,
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
	for _, entry := range entries {
		w.process(ctx, entry)
	}
	return nil
}

// process dispatches a single entry and marks it done or retries it.
func (w *Worker) process(ctx context.Context, entry *notification.OutboxEntry) {
	log := w.log.With(
		zap.Int64("outbox_id", entry.ID),
		zap.String("event_type", string(entry.EventType)),
		zap.String("aggregate_type", entry.AggregateType),
		zap.String("aggregate_id", entry.AggregateID),
		zap.Int("attempt", entry.Attempts),
	)

	if err := w.dispatcher.Dispatch(ctx, entry); err != nil {
		log.Warn("outbox dispatch failed", zap.Error(err))
		if entry.Attempts >= defaultMaxAttempts {
			if mfErr := w.repo.MarkFailed(ctx, entry.ID, err.Error()); mfErr != nil {
				log.Error("failed to mark outbox entry as failed", zap.Error(mfErr))
			} else {
				log.Error("outbox entry exhausted max attempts — marked failed",
					zap.Int("max_attempts", defaultMaxAttempts),
				)
			}
			return
		}
		// Exponential backoff with equal jitter: delay = half_fixed + rand[0, half_fixed]
		full := min(time.Duration(1<<entry.Attempts)*time.Second, 5*time.Minute)
		half := full / 2
		delay := half + time.Duration(rand.Int64N(int64(half)+1))
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
