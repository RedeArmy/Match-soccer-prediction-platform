package messaging

import (
	"testing"
	"time"
)

func TestConfigure_SetsPackageVars(t *testing.T) {
	origAttempts := maxHandlerAttempts
	origStreamMax := streamMaxLen
	origWorkers := StreamWorkerCount
	origReadBlock := streamReadBlock
	origBackoff := RetryBackoff
	defer func() {
		maxHandlerAttempts = origAttempts
		streamMaxLen = origStreamMax
		StreamWorkerCount = origWorkers
		streamReadBlock = origReadBlock
		RetryBackoff = origBackoff
	}()

	backoff := []time.Duration{10 * time.Millisecond, 20 * time.Millisecond}
	Configure(7, 999_000, 4, 10, backoff)

	if maxHandlerAttempts != 7 {
		t.Errorf("maxHandlerAttempts: want 7, got %d", maxHandlerAttempts)
	}
	if streamMaxLen != 999_000 {
		t.Errorf("streamMaxLen: want 999000, got %d", streamMaxLen)
	}
	if StreamWorkerCount != 4 {
		t.Errorf("StreamWorkerCount: want 4, got %d", StreamWorkerCount)
	}
	if streamReadBlock != 10*time.Second {
		t.Errorf("streamReadBlock: want 10s, got %v", streamReadBlock)
	}
	if len(RetryBackoff) != 2 || RetryBackoff[0] != 10*time.Millisecond {
		t.Errorf("RetryBackoff: want [10ms 20ms], got %v", RetryBackoff)
	}
}

func TestConfigure_NilBackoffKeepsExisting(t *testing.T) {
	origAttempts := maxHandlerAttempts
	origStreamMax := streamMaxLen
	origWorkers := StreamWorkerCount
	origReadBlock := streamReadBlock
	origBackoff := RetryBackoff
	defer func() {
		maxHandlerAttempts = origAttempts
		streamMaxLen = origStreamMax
		StreamWorkerCount = origWorkers
		streamReadBlock = origReadBlock
		RetryBackoff = origBackoff
	}()

	Configure(4, 500_000, 0, 0, nil) // zero values for workers/block keep existing

	if maxHandlerAttempts != 4 {
		t.Errorf("maxHandlerAttempts: want 4, got %d", maxHandlerAttempts)
	}
	if StreamWorkerCount != origWorkers {
		t.Errorf("StreamWorkerCount should be unchanged when 0 passed; got %d", StreamWorkerCount)
	}
	if streamReadBlock != origReadBlock {
		t.Errorf("streamReadBlock should be unchanged when 0 passed; got %v", streamReadBlock)
	}
	if RetryBackoff[0] != origBackoff[0] {
		t.Errorf("RetryBackoff should be unchanged when nil passed")
	}
}

func TestConfigure_ZeroWorkersIgnored(t *testing.T) {
	origWorkers := StreamWorkerCount
	defer func() { StreamWorkerCount = origWorkers }()

	Configure(3, 600_000, 0, 0, nil)

	if StreamWorkerCount != origWorkers {
		t.Errorf("StreamWorkerCount must not change when streamWorkers=0; got %d", StreamWorkerCount)
	}
}
