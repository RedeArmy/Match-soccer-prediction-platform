package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	oteltrace "go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// repoTracer is the shared OTel tracer for the repository layer.
// All spans share this tracer so Tempo/Jaeger shows a single "repository"
// instrumentation scope that operators can filter on.
var repoTracer = otel.Tracer("repository")

// startRepoSpan opens a child span for a read-only repository operation.
// Use it on hot-path read methods where DB query time should be visible in
// distributed traces. Call span.End() via defer after checking the error.
//
//	ctx, span := startRepoSpan(ctx, "PredictionRepository.GetByMatch")
//	defer span.End()
func startRepoSpan(ctx context.Context, operation string) (context.Context, oteltrace.Span) {
	return repoTracer.Start(ctx, operation,
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(attribute.String("db.system", "postgresql")),
	)
}

// dbWriteTimeout bounds the wall-clock time of any mutating DB operation
// (transaction or single-row write). Override via SetDBTimeouts at startup
// using WCQ_DATABASE_QUERYWRITETIMEOUT; the default (10 s) is used when not
// explicitly configured.
var dbWriteTimeout = 10 * time.Second

// dbReadTimeout bounds read-only queries. Single-row and small set reads
// are expected to complete in milliseconds; 5 s is a generous allowance for
// index-scanned paginated lists before treating the connection as stuck.
// Override via SetDBTimeouts at startup using WCQ_DATABASE_QUERYREADTIMEOUT.
var dbReadTimeout = 5 * time.Second

// SetDBTimeouts replaces the package-wide query timeout values. It must be
// called once at process startup before any repository method is invoked.
// Zero values are ignored so callers can leave unset fields as zero and rely
// on the package defaults.
func SetDBTimeouts(write, read time.Duration) {
	if write > 0 {
		dbWriteTimeout = write
	}
	if read > 0 {
		dbReadTimeout = read
	}
}

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
	ctx, span := repoTracer.Start(ctx, "db.tx",
		oteltrace.WithSpanKind(oteltrace.SpanKindClient),
		oteltrace.WithAttributes(
			attribute.String("db.system", "postgresql"),
			attribute.String("db.caller", caller),
		),
	)
	defer span.End()

	tx, err := db.Begin(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "begin transaction failed")
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
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "commit failed")
		return apperrors.Internal(err)
	}
	return nil
}
