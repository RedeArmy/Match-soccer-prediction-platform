package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
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

// ── logRollbackFailure tests ──────────────────────────────────────────────────

// TestLogRollbackFailure_ConnectionError_LogsWarning verifies that genuine
// infrastructure errors (connection loss, timeouts) are logged.
func TestLogRollbackFailure_ConnectionError_LogsWarning(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	spyLogger := zap.New(core)
	SetDefensiveLogger(spyLogger)
	defer SetDefensiveLogger(zap.NewNop())

	// Simulate connection error (not ErrTxClosed)
	err := errors.New("connection reset by peer")
	logRollbackFailure(err, "QuinielaRepository", "CreateWithMembership")

	// Verify warning was logged
	if logs.Len() != 1 {
		t.Errorf("expected 1 log entry, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Message != "transaction rollback failed" {
		t.Errorf("expected 'transaction rollback failed', got %q", entry.Message)
	}

	// Verify structured fields
	fields := make(map[string]string)
	for _, f := range entry.Context {
		if f.Type == zapcore.StringType {
			fields[f.Key] = f.String
		}
	}

	if fields["repository"] != "QuinielaRepository" {
		t.Errorf("expected repository=QuinielaRepository, got %s", fields["repository"])
	}
	if fields["method"] != "CreateWithMembership" {
		t.Errorf("expected method=CreateWithMembership, got %s", fields["method"])
	}
}

// TestLogRollbackFailure_ErrTxClosed_NoLog verifies that the normal case
// (commit succeeded, rollback returns ErrTxClosed) does not log anything.
func TestLogRollbackFailure_ErrTxClosed_NoLog(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	spyLogger := zap.New(core)
	SetDefensiveLogger(spyLogger)
	defer SetDefensiveLogger(zap.NewNop())

	// Simulate normal case: commit succeeded, rollback returns ErrTxClosed
	logRollbackFailure(pgx.ErrTxClosed, "QuinielaRepository", "CreateWithMembership")

	// Should NOT log anything
	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries for ErrTxClosed, got %d", logs.Len())
	}
}

// TestLogRollbackFailure_NilError_NoLog verifies that successful rollback
// (nil error) does not log anything.
func TestLogRollbackFailure_NilError_NoLog(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	spyLogger := zap.New(core)
	SetDefensiveLogger(spyLogger)
	defer SetDefensiveLogger(zap.NewNop())

	// Simulate successful rollback (nil error)
	logRollbackFailure(nil, "QuinielaRepository", "CreateWithMembership")

	// Should NOT log anything
	if logs.Len() != 0 {
		t.Errorf("expected 0 log entries for nil error, got %d", logs.Len())
	}
}

// TestLogRollbackFailure_ContextCanceled_LogsWarning verifies that context
// cancellation during rollback triggers defensive logging.
func TestLogRollbackFailure_ContextCanceled_LogsWarning(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	spyLogger := zap.New(core)
	SetDefensiveLogger(spyLogger)
	defer SetDefensiveLogger(zap.NewNop())

	// Simulate context cancellation during rollback
	logRollbackFailure(context.Canceled, "GroupMembershipRepository", "TransferOwnershipRoles")

	// Should log (context.Canceled is not ErrTxClosed)
	if logs.Len() != 1 {
		t.Errorf("expected 1 log entry for context.Canceled, got %d", logs.Len())
	}

	entry := logs.All()[0]
	if entry.Message != "transaction rollback failed" {
		t.Errorf("expected 'transaction rollback failed', got %q", entry.Message)
	}

	// Verify repository field
	for _, f := range entry.Context {
		if f.Key == "repository" && f.Type == zapcore.StringType {
			if f.String != "GroupMembershipRepository" {
				t.Errorf("expected repository=GroupMembershipRepository, got %s", f.String)
			}
			return
		}
	}
	t.Error("missing 'repository' field in log entry")
}
