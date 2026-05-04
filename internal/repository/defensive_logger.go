package repository

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"go.uber.org/zap"
)

// defensiveLog is a package-level logger used exclusively for defensive logging
// in deferred cleanup paths where errors cannot be returned to the caller.
//
// Currently used for logging unexpected transaction rollback failures (errors
// other than pgx.ErrTxClosed). A package-level logger is intentional: repository
// constructors do not accept loggers (repositories are pure data-access and do
// not own cross-cutting concerns), but deferred rollback cleanup must log
// connection failures that would otherwise be silently lost.
//
// The logger is initialized to a no-op implementation. Production applications
// should call SetDefensiveLogger during startup to wire a real logger.
var defensiveLog = zap.NewNop()

// SetDefensiveLogger replaces the package-level defensive logger with the
// provided instance. This must be called during application startup (after
// logger initialization, before any repository methods are invoked) to ensure
// deferred rollback failures are observable.
//
// Not calling this function is safe: the default no-op logger silently discards
// all log statements, which is acceptable given that deferred rollback failures
// are rare and the original commit/query error will still be returned to the caller.
func SetDefensiveLogger(log *zap.Logger) {
	if log != nil {
		defensiveLog = log
	}
}

// logRollbackFailure logs unexpected transaction rollback failures to the
// defensive logger. It filters out pgx.ErrTxClosed (the expected error after
// successful commit) and only logs genuine infrastructure failures.
//
// This function is extracted from inline defer blocks to make the defensive
// logging logic testable via unit tests, since simulating rollback failures
// in integration tests requires flaky infrastructure failure simulation.
//
// Called from deferred cleanup in repository methods that use transactions.
func logRollbackFailure(err error, repository, method string) {
	if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
		defensiveLog.Warn("transaction rollback failed",
			zap.String("repository", repository),
			zap.String("method", method),
			zap.Error(err),
		)
	}
}
