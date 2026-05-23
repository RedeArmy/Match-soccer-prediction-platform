package dispatcher

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWithRenderTimeout_CompletesWithinBudget(t *testing.T) {
	t.Parallel()
	err := withRenderTimeout(context.Background(), 500*time.Millisecond, func() error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestWithRenderTimeout_PropagatesFnError(t *testing.T) {
	t.Parallel()
	want := errors.New("template parse error")
	err := withRenderTimeout(context.Background(), 500*time.Millisecond, func() error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped %v, got %v", want, err)
	}
}

func TestWithRenderTimeout_ExceedsBudget(t *testing.T) {
	t.Parallel()
	// fn blocks longer than the timeout; we expect a deadline-exceeded error.
	err := withRenderTimeout(context.Background(), 20*time.Millisecond, func() error {
		time.Sleep(200 * time.Millisecond)
		return nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
}

func TestWithRenderTimeout_CancelledParentContext(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled before the call
	err := withRenderTimeout(ctx, 500*time.Millisecond, func() error {
		time.Sleep(100 * time.Millisecond) // fn would complete normally
		return nil
	})
	// A pre-cancelled parent context means the timeout fires immediately.
	if err == nil {
		t.Fatal("expected error from cancelled parent context, got nil")
	}
}
