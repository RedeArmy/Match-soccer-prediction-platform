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
const maxHandlerAttempts = 3

// callWithRetry invokes fn up to maxHandlerAttempts times. Between attempts it
// sleeps for the duration given by RetryBackoff, honouring ctx cancellation so
// that bus shutdown is not blocked by an in-progress sleep. Returns nil on the
// first success, or the last non-nil error after all attempts are exhausted.
func callWithRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxHandlerAttempts; attempt++ {
		if err = fn(); err == nil {
			return nil
		}
		if attempt < len(RetryBackoff) {
			select {
			case <-ctx.Done():
				// Bus is shutting down; stop retrying immediately.
				return err
			case <-time.After(RetryBackoff[attempt]):
			}
		}
	}
	return err
}
