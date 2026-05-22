package dispatcher

import (
	"context"
	"fmt"
	"time"
)

// withRenderTimeout runs fn in a dedicated goroutine and returns its error.
// If fn does not complete within timeout, it returns a wrapped context.DeadlineExceeded
// error so the caller can treat the render as a transient failure and retry.
//
// Template rendering in Go's text/template and html/template packages is purely
// CPU-bound and does not check context cancellation internally. This helper
// provides an external wall-clock bound by racing the goroutine against a
// timeout derived from the context. The goroutine continues to run after a
// timeout (it cannot be cancelled), but the buffered channel prevents it from
// leaking: the goroutine will eventually write its result and exit regardless of
// whether the caller is still listening.
//
// Callers should treat a timeout error the same as any other render error:
// log it, reschedule the outbox entry with back-off, and let the worker retry.
func withRenderTimeout(ctx context.Context, timeout time.Duration, fn func() error) error {
	ch := make(chan error, 1) // buffered: goroutine writes once and exits cleanly
	go func() { ch <- fn() }()

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	select {
	case err := <-ch:
		return err
	case <-timeoutCtx.Done():
		return fmt.Errorf("email render exceeded %s budget: %w", timeout, timeoutCtx.Err())
	}
}
