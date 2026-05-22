package repository

import (
	"context"
	"errors"
	"math/rand/v2"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// retryPolicyMu guards retryMaxAttempts, retryBaseDelay, and retryMaxDelay.
// InitRetryPolicy holds the write lock; withRetryTx holds the read lock when
// snapshotting the values at the start of each transaction.
var retryPolicyMu sync.RWMutex

var (
	retryMaxAttempts = 3
	retryBaseDelay   = 50 * time.Millisecond
	retryMaxDelay    = 1 * time.Second
)

// InitRetryPolicy overrides the transaction retry policy for transient
// database errors. Safe to call concurrently; uses retryPolicyMu.
func InitRetryPolicy(maxAttempts, baseDelayMs, maxDelayMs int) {
	retryPolicyMu.Lock()
	defer retryPolicyMu.Unlock()
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
	// Snapshot policy values under the read lock so a concurrent InitRetryPolicy
	// call cannot modify them mid-loop.
	retryPolicyMu.RLock()
	maxAttempts := retryMaxAttempts
	baseDelay := retryBaseDelay
	maxDelay := retryMaxDelay
	retryPolicyMu.RUnlock()

	for attempt := range maxAttempts {
		err := withTx(ctx, db, caller, fn)
		if err == nil || !isTransientPGError(err) {
			return err
		}
		if attempt == maxAttempts-1 {
			return err // exhausted attempts
		}
		// Equal-jitter backoff: delay = half_fixed + rand[0, half_fixed]
		full := min(baseDelay*(1<<attempt), maxDelay)
		half := full / 2
		delay := half + time.Duration(rand.Int64N(int64(half)+1)) //nolint:gosec // G404: jitter for backoff; cryptographic randomness not required
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil // unreachable; loop always returns inside
}
