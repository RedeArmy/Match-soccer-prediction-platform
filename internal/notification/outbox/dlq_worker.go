package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/internal/domain"
	"github.com/rede/world-cup-quiniela/internal/notification"
	"github.com/rede/world-cup-quiniela/pkg/tracing"
)

const (
	dlqDefaultBatchSize      = domain.DefaultNotifyDLQReplayBatchSize
	dlqDefaultPollInterval   = domain.DefaultNotifyDLQReplayPollIntervalSec * time.Second
	dlqDefaultMaxAttempts    = domain.DefaultNotifyDLQReplayMaxAttempts
	dlqDefaultAlertThreshold = domain.DefaultNotifyDLQReplayAlertThreshold
)

// dlqOverflowNotifier is the narrow interface consumed by DLQWorker for
// operational alerting when the unresolved DLQ depth exceeds the threshold.
// The concrete type is *observability.Notifier; the interface breaks the
// import cycle and keeps the outbox package free of infrastructure.
type dlqOverflowNotifier interface {
	NotifyDLQOverflow(ctx context.Context, depth, threshold int64)
}

// dlqRepository is the persistence contract consumed by DLQWorker.
// The production implementation is *repository.postgresNotificationDLQRepository.
type dlqRepository interface {
	ClaimBatch(ctx context.Context, limit, maxAttempts int) ([]*domain.NotificationDLQEntry, error)
	MarkResolved(ctx context.Context, id int64) error
	RecordFailure(ctx context.Context, id int64, errDetail string) error
	CountUnresolved(ctx context.Context) (int64, error)
}

// dlqWriter is the outbox-write contract for DLQWorker.
// It matches the Write method of *outbox.PoolWriter.
type dlqWriter interface {
	Write(ctx context.Context, eventType notification.EventType, aggregateType, aggregateID string, payload any) error
}

// DLQWorker polls the notification_dlq table and re-inserts eligible entries
// into domain_outbox so they re-enter the normal dispatch pipeline.
//
// Entries are eligible when:
//   - resolved_at IS NULL
//   - attempts < maxAttempts
//   - exponential back-off has elapsed (2^attempts seconds since last_retry_at)
//
// On successful replay the entry is marked resolved; on failure the attempt
// counter is incremented and the entry waits for the next back-off window.
type DLQWorker struct {
	repo           dlqRepository
	writer         dlqWriter
	log            *zap.Logger
	pollInterval   time.Duration
	batchSize      int
	maxAttempts    int
	alertThreshold int64
	// notifier fires an n8n webhook when the unresolved DLQ count exceeds
	// alertThreshold. nil disables the operational alert (logs-only mode).
	notifier dlqOverflowNotifier
}

// DLQWorkerOption is a functional option for DLQWorker.
type DLQWorkerOption func(*DLQWorker)

// WithDLQBatchSize overrides the number of DLQ entries claimed per poll cycle.
func WithDLQBatchSize(n int) DLQWorkerOption {
	return func(w *DLQWorker) { w.batchSize = n }
}

// WithDLQPollInterval overrides the sleep duration between poll cycles.
func WithDLQPollInterval(d time.Duration) DLQWorkerOption {
	return func(w *DLQWorker) { w.pollInterval = d }
}

// WithDLQMaxAttempts overrides the maximum number of replay attempts before an
// entry is permanently abandoned.
func WithDLQMaxAttempts(n int) DLQWorkerOption {
	return func(w *DLQWorker) { w.maxAttempts = n }
}

// WithDLQAlertThreshold overrides the unresolved-entry count above which the
// worker emits an Error-level log line on each poll cycle.
func WithDLQAlertThreshold(n int64) DLQWorkerOption {
	return func(w *DLQWorker) { w.alertThreshold = n }
}

// WithDLQNotifier wires an observability.Notifier so that DLQWorker fires an
// n8n webhook whenever the unresolved DLQ count exceeds the alert threshold.
// The call is fire-and-forget and never blocks the poll loop.
// Pass nil to disable (identical to omitting this option).
func WithDLQNotifier(n dlqOverflowNotifier) DLQWorkerOption {
	return func(w *DLQWorker) { w.notifier = n }
}

// NewDLQWorker constructs a DLQWorker.
func NewDLQWorker(repo dlqRepository, writer dlqWriter, log *zap.Logger, opts ...DLQWorkerOption) *DLQWorker {
	w := &DLQWorker{
		repo:           repo,
		writer:         writer,
		log:            log,
		pollInterval:   dlqDefaultPollInterval,
		batchSize:      dlqDefaultBatchSize,
		maxAttempts:    dlqDefaultMaxAttempts,
		alertThreshold: dlqDefaultAlertThreshold,
	}
	for _, o := range opts {
		o(w)
	}
	return w
}

// Run starts the DLQ replay polling loop.  It blocks until ctx is cancelled.
// Errors from individual poll cycles are logged but do not stop the loop.
func (w *DLQWorker) Run(ctx context.Context) {
	w.log.Info("dlq replay worker started",
		zap.Int("batch_size", w.batchSize),
		zap.Duration("poll_interval", w.pollInterval),
		zap.Int("max_attempts", w.maxAttempts),
	)
	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			w.log.Info("dlq replay worker stopped")
			return
		case <-ticker.C:
			w.poll(ctx)
		}
	}
}

// poll counts unresolved entries (alerting if above threshold), then claims
// and processes a batch.
func (w *DLQWorker) poll(ctx context.Context) {
	total, err := w.repo.CountUnresolved(ctx)
	if err != nil {
		w.log.Warn("dlq replay: count unresolved failed", zap.Error(err))
	} else if total > w.alertThreshold {
		w.log.Error("dlq replay: unresolved entry count exceeds alert threshold",
			zap.Int64("unresolved", total),
			zap.Int64("threshold", w.alertThreshold),
		)
		if w.notifier != nil {
			w.notifier.NotifyDLQOverflow(ctx, total, w.alertThreshold)
		}
	}

	entries, err := w.repo.ClaimBatch(ctx, w.batchSize, w.maxAttempts)
	if err != nil {
		w.log.Error("dlq replay: claim batch failed", zap.Error(err))
		return
	}
	for _, entry := range entries {
		w.process(ctx, entry)
	}
}

// process replays a single DLQ entry by writing it back to domain_outbox.
// On success it marks the entry resolved; on failure it records the error
// and increments the attempt counter so the exponential back-off applies.
func (w *DLQWorker) process(ctx context.Context, entry *domain.NotificationDLQEntry) {
	log := w.log.With(
		append([]zap.Field{
			zap.Int64("dlq_id", entry.ID),
			zap.String("event_type", entry.EventType),
			zap.String("channel", entry.Channel),
			zap.Int("attempts", entry.Attempts),
		}, tracing.LogFields(ctx)...)...,
	)

	if err := w.writer.Write(
		ctx,
		notification.EventType(entry.EventType),
		"dlq_replay",
		fmt.Sprintf("dlq-%d", entry.ID),
		json.RawMessage(entry.Payload),
	); err != nil {
		log.Warn("dlq replay: write to outbox failed", zap.Error(err))
		if rfErr := w.repo.RecordFailure(ctx, entry.ID, err.Error()); rfErr != nil {
			log.Error("dlq replay: record failure failed", zap.Error(rfErr))
		}
		return
	}

	if err := w.repo.MarkResolved(ctx, entry.ID); err != nil {
		log.Error("dlq replay: mark resolved failed", zap.Error(err))
		return
	}
	log.Info("dlq replay: entry replayed and resolved")
}
