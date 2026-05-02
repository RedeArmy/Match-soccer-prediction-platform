package repository

import (
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// singleScanErr maps the error from a single-row QueryRow().Scan() call to the
// repository's not-found convention: pgx.ErrNoRows becomes nil (the caller then
// returns (nil, nil), signalling "not found"). Any other error is wrapped in
// apperrors.Internal.
//
// Usage - replace the two-branch sentinel check in every scanX helper:
//
//	if err := row.Scan(...); err != nil {
//	    return nil, singleScanErr(err)
//	}
func singleScanErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	return apperrors.Internal(err)
}

// collectRows iterates rows, calling scan once per row, and returns the
// accumulated slice. rows is closed before returning in all paths. Any scan
// error or rows.Err() value is wrapped in apperrors.Internal.
//
// The caller is responsible for the initial db.Query() call and its error
// check; collectRows only handles the iteration phase.
//
// Usage:
//
//	rows, err := r.db.Query(ctx, query, args...)
//	if err != nil {
//	    return nil, apperrors.Internal(err)
//	}
//	return collectRows(rows, func(r pgx.Rows) (*domain.Thing, error) {
//	    t := &domain.Thing{}
//	    return t, r.Scan(&t.ID, &t.Name)
//	})
func collectRows[T any](rows pgx.Rows, scan func(pgx.Rows) (T, error)) ([]T, error) {
	defer rows.Close()
	var result []T
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, apperrors.Internal(err)
		}
		result = append(result, v)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Internal(err)
	}
	return result, nil
}

// applyPagination appends LIMIT/OFFSET clauses to q and corresponding values to args.
// n is the next positional argument index (1-based) for the query being built.
func applyPagination(q string, args []any, n int, p Pagination) (string, []any, int) {
	if p.Limit > 0 {
		q += " LIMIT $" + itoa(n)
		args = append(args, p.Limit)
		n++
	}
	if p.Offset > 0 {
		q += " OFFSET $" + itoa(n)
		args = append(args, p.Offset)
		n++
	}
	return q, args, n
}

// itoa converts a non-negative int to its decimal string representation.
// Used to build $N positional-argument placeholders without importing fmt.
func itoa(n int) string {
	return strconv.Itoa(n)
}
