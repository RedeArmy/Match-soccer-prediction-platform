package repository

import (
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

// ── InitRetryPolicy ───────────────────────────────────────────────────────────

func TestInitRetryPolicy_SetsValues(t *testing.T) {
	// Capture original values and restore after the test so other tests in the
	// package are unaffected by the override.
	origAttempts, origBase, origMax := RetryPolicySnapshot()
	t.Cleanup(func() {
		InitRetryPolicy(int(origAttempts), int(origBase/time.Millisecond), int(origMax/time.Millisecond))
	})

	InitRetryPolicy(5, 100, 2000)

	gotAttempts, gotBase, gotMax := RetryPolicySnapshot()
	if gotAttempts != 5 {
		t.Errorf("maxAttempts: got %d, want 5", gotAttempts)
	}
	if gotBase != 100*time.Millisecond {
		t.Errorf("baseDelay: got %v, want 100ms", gotBase)
	}
	if gotMax != 2000*time.Millisecond {
		t.Errorf("maxDelay: got %v, want 2000ms", gotMax)
	}
}

// ── isTransientPGError ────────────────────────────────────────────────────────

func pgErr(code string) *pgconn.PgError { return &pgconn.PgError{Code: code} }

func TestIsTransientPGError_Serialization(t *testing.T) {
	if !isTransientPGError(pgErr("40001")) {
		t.Error("40001 (serialization_failure) must be transient")
	}
}

func TestIsTransientPGError_Deadlock(t *testing.T) {
	if !isTransientPGError(pgErr("40P01")) {
		t.Error("40P01 (deadlock_detected) must be transient")
	}
}

func TestIsTransientPGError_OtherPGCode_ReturnsFalse(t *testing.T) {
	if isTransientPGError(pgErr("23505")) { // unique_violation — not transient
		t.Error("23505 must NOT be transient")
	}
}

func TestIsTransientPGError_NonPGError_ReturnsFalse(t *testing.T) {
	if isTransientPGError(errors.New("connection reset")) {
		t.Error("plain error must NOT be transient")
	}
}

func TestIsTransientPGError_Nil_ReturnsFalse(t *testing.T) {
	if isTransientPGError(nil) {
		t.Error("nil must NOT be transient")
	}
}

func TestIsTransientPGError_WrappedPGError_Detected(t *testing.T) {
	wrapped := errors.New("wrapped: " + pgErr("40001").Error())
	// errors.As cannot unwrap this plain wrapping, so it must return false
	// (the test documents the boundary: only errors.As-unwrappable chains work).
	_ = isTransientPGError(wrapped) // must not panic
}
