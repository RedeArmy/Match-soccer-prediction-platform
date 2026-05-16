package repository

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/rede/world-cup-quiniela/pkg/apperrors"
)

// rowScanner is satisfied by both pgx.Row (single-row query) and pgx.Rows
// (multi-row iteration). Accepting this interface in scanFooFields lets a
// single function serve both callers without duplicating the Scan call and any
// post-scan field transformations (JSON unmarshal, nil-pointer unwrap, etc.).
type rowScanner interface {
	Scan(dest ...any) error
}

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
//
// Pagination.Limit must be positive (bounded) or -1 (unbounded via Unbounded()).
// Zero-value Pagination{} (Limit=0) triggers a panic to fail fast during development
// rather than silently executing an unbounded query that could exhaust memory in
// production. Tests that need unbounded queries must explicitly call Unbounded().
func applyPagination(q string, args []any, n int, p Pagination) (string, []any, int) {
	if p.Limit == 0 {
		panic("repository: Pagination.Limit=0 is invalid; use positive limit or Unbounded()")
	}
	if p.Limit > 0 {
		q += " LIMIT $" + itoa(n)
		args = append(args, p.Limit)
		n++
	}
	// p.Limit == unboundedLimit (-1): no LIMIT clause, query returns all rows
	if p.Offset > 0 {
		q += " OFFSET $" + itoa(n)
		args = append(args, p.Offset)
		n++
	}
	return q, args, n
}

// itoa converts a non-negative int to its decimal string representation.
// Used to build $N positional-argument placeholders.
func itoa(n int) string {
	return strconv.Itoa(n)
}

// whereBuilder accumulates SQL predicate fragments and their positional
// arguments. It eliminates the WHERE 1=1 anti-pattern: call clause() to get
// a proper WHERE clause (empty string when no predicates were added).
type whereBuilder struct {
	conds  []string
	args   []any
	argIdx int
}

// newWhereBuilder returns a builder whose positional arguments start at $1.
func newWhereBuilder() *whereBuilder {
	return &whereBuilder{argIdx: 1}
}

// add appends one predicate. expr must contain a single %d verb for the
// positional placeholder, e.g. "user_id = $%d".
func (w *whereBuilder) add(expr string, val any) {
	w.conds = append(w.conds, fmt.Sprintf(expr, w.argIdx))
	w.args = append(w.args, val)
	w.argIdx++
}

// clause returns " WHERE cond1 AND cond2 ..." or "" when no predicates exist.
func (w *whereBuilder) clause() string {
	if len(w.conds) == 0 {
		return ""
	}
	return " WHERE " + strings.Join(w.conds, " AND ")
}

// addCond appends a predicate that has no positional arguments — e.g. a NULL
// check or a literal boolean expression.
func (w *whereBuilder) addCond(expr string) {
	w.conds = append(w.conds, expr)
}

// addDual appends a predicate that references the same value in two positions.
// expr must contain exactly two %d verbs that will both receive the same
// positional argument index. Use for ILIKE patterns applied across two columns:
//
//	wb.addDual("(name ILIKE $%d OR email ILIKE $%d)", "%query%")
func (w *whereBuilder) addDual(expr string, val any) {
	w.conds = append(w.conds, fmt.Sprintf(expr, w.argIdx, w.argIdx))
	w.args = append(w.args, val)
	w.argIdx++
}

// next returns the next positional argument index for passing to applyPagination.
func (w *whereBuilder) next() int {
	return w.argIdx
}
