package repository_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rede/world-cup-quiniela/internal/repository"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// The three tests below exercise withTx (via the exported shim repository.WithTx)
// using the shared testDB pool created in TestMain. Sharing the pool avoids
// spinning up a separate Docker container and running all migrations for each
// individual test, which was the cause of the test-timeout regression these
// tests previously introduced.

func TestWithTx_BeginFails_ReturnsInternalError(t *testing.T) {
	skipIfNoDB(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling withTx; db.Begin should fail immediately

	err := repository.WithTx(ctx, testDB, "test.Begin", func(pgx.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Errorf("expected internal error wrapping, got %v", err)
	}
}

func TestWithTx_FnReturnsError_PropagatesAndRollsBack(t *testing.T) {
	skipIfNoDB(t)

	sentinelErr := apperrors.NotFound("not found")
	err := repository.WithTx(context.Background(), testDB, "test.FnError", func(pgx.Tx) error {
		return sentinelErr
	})
	if !errors.Is(err, sentinelErr) {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestWithTx_FnSucceeds_CommitsAndReturnsNil(t *testing.T) {
	skipIfNoDB(t)

	err := repository.WithTx(context.Background(), testDB, "test.FnOK", func(pgx.Tx) error { return nil })
	if err != nil {
		t.Errorf("expected nil on success, got %v", err)
	}
}

// ── SetDBTimeouts ─────────────────────────────────────────────────────────────

func TestSetDBTimeouts_PositiveValues_BothUpdated(t *testing.T) {
	origWrite, origRead := repository.DBTimeoutsSnapshot()
	t.Cleanup(func() { repository.SetDBTimeouts(origWrite, origRead) })

	repository.SetDBTimeouts(30*time.Second, 15*time.Second)

	gotWrite, gotRead := repository.DBTimeoutsSnapshot()
	if gotWrite != 30*time.Second {
		t.Errorf("dbWriteTimeout = %v; want 30s", gotWrite)
	}
	if gotRead != 15*time.Second {
		t.Errorf("dbReadTimeout = %v; want 15s", gotRead)
	}
}

func TestSetDBTimeouts_ZeroValues_NeitherUpdated(t *testing.T) {
	origWrite, origRead := repository.DBTimeoutsSnapshot()
	t.Cleanup(func() { repository.SetDBTimeouts(origWrite, origRead) })

	repository.SetDBTimeouts(0, 0)

	gotWrite, gotRead := repository.DBTimeoutsSnapshot()
	if gotWrite != origWrite {
		t.Errorf("dbWriteTimeout changed to %v; want %v (unchanged)", gotWrite, origWrite)
	}
	if gotRead != origRead {
		t.Errorf("dbReadTimeout changed to %v; want %v (unchanged)", gotRead, origRead)
	}
}

func TestSetDBTimeouts_OnlyWrite_ReadUnchanged(t *testing.T) {
	origWrite, origRead := repository.DBTimeoutsSnapshot()
	t.Cleanup(func() { repository.SetDBTimeouts(origWrite, origRead) })

	repository.SetDBTimeouts(20*time.Second, 0)

	gotWrite, gotRead := repository.DBTimeoutsSnapshot()
	if gotWrite != 20*time.Second {
		t.Errorf("dbWriteTimeout = %v; want 20s", gotWrite)
	}
	if gotRead != origRead {
		t.Errorf("dbReadTimeout changed to %v; want %v (unchanged)", gotRead, origRead)
	}
}
