package messaging

import (
	"context"
	"time"
)

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
// retry and stream configuration. Must be called before any subscribers are
// registered (i.e. before bus.Subscribe is first invoked) to avoid a data race
// with running consumer goroutines.
//
// maxRetries: total handler attempts (messaging.max_retries).
// streamMax:  Redis Stream MAXLEN cap (messaging.stream_max_len).
// backoff:    per-attempt sleep durations; nil keeps the current value.
func Configure(maxRetries int, streamMax int64, backoff []time.Duration) {
	maxHandlerAttempts = maxRetries
	streamMaxLen = streamMax
	if len(backoff) > 0 {
		RetryBackoff = backoff
	}
}

// callWithRetry invokes fn up to maxHandlerAttempts times. Between attempts it
// sleeps for the duration given by RetryBackoff, honouring ctx cancellation so
// that bus shutdown is not blocked by an in-progress sleep. Returns nil on the
// first success, or the last non-nil error after all attempts are exhausted.
func callWithRetry(ctx context.Context, fn func() error) error {
	// Snapshot RetryBackoff and maxHandlerAttempts once before any handler call.
	// This ensures the reads are ordered before the first fn() invocation and
	// avoids a data race with tests that restore RetryBackoff via defer.
	backoff := RetryBackoff
	maxAttempts := maxHandlerAttempts
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
