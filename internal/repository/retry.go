package repository

import (
	"context"
	"errors"
	"math/rand"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	retryMaxAttempts = 3
	retryBaseDelay   = 50 * time.Millisecond
	retryMaxDelay    = 1 * time.Second
)

// InitRetryPolicy overrides the transaction retry policy for transient
// database errors. Call once at process startup before any requests are
// served; calling it concurrently with active requests is not safe.
func InitRetryPolicy(maxAttempts, baseDelayMs, maxDelayMs int) {
	retryMaxAttempts = maxAttempts
	retryBaseDelay = time.Duration(baseDelayMs) * time.Millisecond
	retryMaxDelay = time.Duration(maxDelayMs) * time.Millisecond
}

// isTransientPGError returns true for PostgreSQL error codes that are safe to
// retry because the operation was aborted by the engine without any lasting
// side-effect:
//
//	40001  serialization_failure  — MVCC conflict; transaction was rolled back.
//	40P01  deadlock_detected      — Deadlock; transaction was rolled back.
//
// Business-logic errors (constraint violations, not-found sentinel values,
// insufficient balance) are NOT transient and must not trigger retries.
func isTransientPGError(err error) bool {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "40001" || pgErr.Code == "40P01"
	}
	return false
}

// withRetryTx executes fn inside a transaction with up to retryMaxAttempts
// attempts. Retries only occur for transient serialization failures and
// deadlocks (see isTransientPGError). All other errors — including application
// sentinel errors — are returned immediately without retry.
//
// Backoff uses equal-jitter exponential delay: each interval is split in half,
// one half fixed and one half randomly distributed, so concurrent callers
// spread their retries rather than thundering back simultaneously.
//
//	attempt 1 → delay: 25–50 ms
//	attempt 2 → delay: 50–100 ms
func withRetryTx(ctx context.Context, db *pgxpool.Pool, caller string, fn func(pgx.Tx) error) error {
	for attempt := range retryMaxAttempts {
		err := withTx(ctx, db, caller, fn)
		if err == nil || !isTransientPGError(err) {
			return err
		}
		if attempt == retryMaxAttempts-1 {
			return err // exhausted attempts
		}
		// Equal-jitter backoff: delay = half_fixed + rand[0, half_fixed]
		full := min(retryBaseDelay*(1<<attempt), retryMaxDelay)
		half := full / 2
		delay := half + time.Duration(rand.Int63n(int64(half)+1))
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil // unreachable; loop always returns inside
}
