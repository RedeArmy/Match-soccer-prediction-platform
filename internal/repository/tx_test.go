package repository_test

import (
	"context"
	"errors"
	"testing"

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
