package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RetryPolicySnapshot returns the current retry policy values for assertion in
// tests. Only valid for use in tests; not part of the public API.
func RetryPolicySnapshot() (maxAttempts int, baseDelay, maxDelay time.Duration) {
	return retryMaxAttempts, retryBaseDelay, retryMaxDelay
}

// WithTx exposes the internal withTx helper for white-box testing from the
// external test package (package repository_test). Not part of the public API.
func WithTx(ctx context.Context, db *pgxpool.Pool, caller string, fn func(pgx.Tx) error) error {
	return withTx(ctx, db, caller, fn)
}

// DBTimeoutsSnapshot returns the current dbWriteTimeout and dbReadTimeout values
// for assertion in tests. Only valid for use in tests; not part of the public API.
func DBTimeoutsSnapshot() (write, read time.Duration) {
	return dbWriteTimeout, dbReadTimeout
}
