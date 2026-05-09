package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/rede/world-cup-quiniela/internal/infrastructure/database"
	"github.com/rede/world-cup-quiniela/internal/testutil"
	"github.com/rede/world-cup-quiniela/migrations"
	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

func TestWithTx_BeginFails_ReturnsInternalError(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = withTx(ctx, pool, "test.Begin", func(pgx.Tx) error { return nil })
	if err == nil {
		t.Fatal("expected error when context is cancelled, got nil")
	}
	if !errors.Is(err, apperrors.ErrInternal) {
		t.Errorf("expected internal error wrapping, got %v", err)
	}
}

func TestWithTx_FnReturnsError_PropagatesAndRollsBack(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	sentinelErr := apperrors.NotFound("not found")
	err = withTx(context.Background(), pool, "test.FnError", func(pgx.Tx) error {
		return sentinelErr
	})
	if err != sentinelErr {
		t.Errorf("expected sentinel error, got %v", err)
	}
}

func TestWithTx_FnSucceeds_CommitsAndReturnsNil(t *testing.T) {
	dsn := testutil.SetupPostgres(t)
	if err := database.Migrate(dsn, migrations.FS); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	pool, err := database.NewPool(context.Background(), database.Config{
		DSN:             dsn,
		MaxOpenConns:    3,
		MaxIdleConns:    1,
		ConnMaxLifetime: time.Minute,
	})
	if err != nil {
		t.Fatalf("new pool: %v", err)
	}
	defer pool.Close()

	err = withTx(context.Background(), pool, "test.FnOK", func(pgx.Tx) error { return nil })
	if err != nil {
		t.Errorf("expected nil on success, got %v", err)
	}
}
