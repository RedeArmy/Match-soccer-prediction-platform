package messaging

import (
	"context"
	"sync"
	"time"
)

// configMu guards all package-level configuration globals (RetryBackoff,
// maxHandlerAttempts, streamMaxLen, StreamWorkerCount, streamReadBlock).
// Configure holds the write lock; callWithRetry and any other reader that
// snapshots these values holds the read lock.
var configMu sync.RWMutex

// RetryBackoff defines the sleep durations between consecutive handler attempts.
// The slice has maxHandlerAttempts-1 entries: there is no sleep before the first
// attempt; RetryBackoff[0] is used before the second, RetryBackoff[1] before the
// third, and so on. Tests may override this variable to avoid real sleeps.
var RetryBackoff = []time.Duration{time.Second, 2 * time.Second}

// maxHandlerAttempts is the total number of times a failing event handler is
// called before the event is declared undeliverable and routed to the DLQ.
// Overridable at startup via Configure (reads messaging.max_retries from system_params).
var maxHandlerAttempts = 3

// Configure applies system_params values to the messaging package-level
// retry and stream configuration. Safe to call concurrently; uses configMu.
//
// maxRetries:         total handler attempts (messaging.max_retries).
// streamMax:          Redis Stream MAXLEN cap (messaging.stream_max_len).
// streamWorkers:      goroutines per EventType pool (messaging.stream_worker_count); ≤0 keeps current value.
// streamReadBlockSec: XREADGROUP block timeout in seconds (messaging.stream_read_block_sec); ≤0 keeps current value.
// backoff:            per-attempt sleep durations; nil keeps the current value.
func Configure(maxRetries int, streamMax int64, streamWorkers int, streamReadBlockSec int, backoff []time.Duration) {
	configMu.Lock()
	defer configMu.Unlock()
	maxHandlerAttempts = maxRetries
	streamMaxLen = streamMax
	if streamWorkers > 0 {
		StreamWorkerCount = streamWorkers
	}
	if streamReadBlockSec > 0 {
		streamReadBlock = time.Duration(streamReadBlockSec) * time.Second
	}
	if len(backoff) > 0 {
		RetryBackoff = backoff
	}
}

// callWithRetry invokes fn up to maxHandlerAttempts times. Between attempts it
// sleeps for the duration given by RetryBackoff, honouring ctx cancellation so
// that bus shutdown is not blocked by an in-progress sleep. Returns nil on the
// first success, or the last non-nil error after all attempts are exhausted.
func callWithRetry(ctx context.Context, fn func() error) error {
	// Snapshot under the read lock so that a concurrent Configure call cannot
	// modify the globals mid-read. The snapshot is held for the lifetime of this
	// call; changes made by a subsequent Configure take effect on the next call.
	configMu.RLock()
	backoff := RetryBackoff
	maxAttempts := maxHandlerAttempts
	configMu.RUnlock()
	var err error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if attempt < len(backoff) {
			select {
			case <-ctx.Done():
				// Bus is shutting down; stop retrying immediately.
				return err
			case <-time.After(backoff[attempt]):
			}
		}
	}
	return err
}
