package messaging

import (
	"testing"
	"time"
)

func TestConfigure_SetsPackageVars(t *testing.T) {
	origAttempts := maxHandlerAttempts
	origStreamMax := streamMaxLen
	origBackoff := RetryBackoff
	defer func() {
		maxHandlerAttempts = origAttempts
		streamMaxLen = origStreamMax
		RetryBackoff = origBackoff
	}()

	backoff := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	Configure(7, 999_000, backoff)

	if maxHandlerAttempts != 7 {
		t.Errorf("maxHandlerAttempts: want 7, got %d", maxHandlerAttempts)
	}
	if streamMaxLen != 999_000 {
		t.Errorf("streamMaxLen: want 999000, got %d", streamMaxLen)
	}
	if len(RetryBackoff) != 2 || RetryBackoff[0] != 10*time.Millisecond {
		t.Errorf("RetryBackoff: want [10ms 20ms], got %v", RetryBackoff)
	}
}

func TestConfigure_NilBackoffKeepsExisting(t *testing.T) {
	origAttempts := maxHandlerAttempts
	origStreamMax := streamMaxLen
	origBackoff := RetryBackoff
	defer func() {
		maxHandlerAttempts = origAttempts
		streamMaxLen = origStreamMax
		RetryBackoff = origBackoff
	}()

	Configure(4, 500_000, nil)

	if maxHandlerAttempts != 4 {
		t.Errorf("maxHandlerAttempts: want 4, got %d", maxHandlerAttempts)
	}
	if RetryBackoff[0] != origBackoff[0] {
		t.Errorf("RetryBackoff should be unchanged when nil passed")
	}
}
