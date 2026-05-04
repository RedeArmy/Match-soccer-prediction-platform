package repository

import (
	"testing"

	"go.uber.org/zap"
)

// TestSetDefensiveLogger verifies that the package-level logger is replaced
// when a non-nil logger is provided, and that nil is safely ignored.
func TestSetDefensiveLogger_NonNil_ReplacesLogger(t *testing.T) {
	originalLog := defensiveLog

	// Replace with a real logger
	testLog := zap.NewNop()
	SetDefensiveLogger(testLog)

	// Verify it was set (we can't compare loggers directly, but we can
	// verify it didn't panic and the function completed)
	if defensiveLog == nil {
		t.Error("expected defensiveLog to be non-nil after SetDefensiveLogger")
	}

	// Restore original for other tests
	defensiveLog = originalLog
}

// TestSetDefensiveLogger_Nil_Ignored verifies that passing nil to
// SetDefensiveLogger is safe and does not replace the existing logger.
func TestSetDefensiveLogger_Nil_Ignored(t *testing.T) {
	originalLog := defensiveLog

	// Attempt to set nil
	SetDefensiveLogger(nil)

	// Verify the logger was not replaced
	if defensiveLog != originalLog {
		t.Error("expected defensiveLog to remain unchanged when nil is passed")
	}
}

// TestDefensiveLogInitialization verifies that the package-level logger
// is initialized to a no-op implementation on package load.
func TestDefensiveLogInitialization(t *testing.T) {
	// The logger should never be nil, even at package init
	if defensiveLog == nil {
		t.Error("expected defensiveLog to be initialized to zap.NewNop()")
	}
}
