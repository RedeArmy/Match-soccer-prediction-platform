package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// dbWriteTimeout bounds the wall-clock time of any mutating DB operation
// (transaction or single-row write). 10 s is sufficient for multi-query
// transactions on local/RDS Postgres while protecting against a hung
// connection stalling an HTTP handler indefinitely.
const dbWriteTimeout = 10 * time.Second

// dbReadTimeout bounds read-only queries. Single-row and small set reads
// are expected to complete in milliseconds; 5 s is a generous allowance for
// index-scanned paginated lists before treating the connection as stuck.
const dbReadTimeout = 5 * time.Second

// withTx executes fn inside a single pgx transaction. The transaction is
// committed when fn returns nil; it is rolled back on any error returned by fn.
//
// Unexpected rollback failures (errors other than pgx.ErrTxClosed, which is
// the normal response after a successful commit) are logged via the
// package-level defensive logger with a caller tag for diagnostic context.
//
// caller should be "Repository.Method" — it appears in the rollback-failure
// log entry and in no other path, so brevity is fine.
//
// All repository methods that require a transaction must use withTx rather than
// managing Begin/Rollback/Commit directly. This guarantees that every
// transaction in the package is covered by defensive rollback logging in a
// single, auditable place.
func withTx(ctx context.Context, db *pgxpool.Pool, caller string, fn func(pgx.Tx) error) error {
	tx, err := db.Begin(ctx)
	if err != nil {
		return apperrors.Internal(err)
	}
	defer func() {
		if rbErr := tx.Rollback(ctx); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			defensiveLog.Warn("transaction rollback failed",
				zap.String("caller", caller),
				zap.Error(rbErr),
			)
		}
	}()
	if err := fn(tx); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return apperrors.Internal(err)
	}
	return nil
}
